package rulechanges

import (
	"errors"
	"go/version"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// deferredSources are the Go files of directories that package discovery
// skips: vendor/ and directories whose names start with "_" or ".". Go
// still builds such a package when something imports it explicitly, so its
// files are read when an import reaches it, and only then: unreached vendored
// code is never parsed.
type deferredSources struct {
	tree   sourceTree
	result *classification
	files  map[string][]string
	known  map[string]bool
}

func newDeferredSources(tree sourceTree, result *classification) *deferredSources {
	known := make(map[string]bool, len(tree.paths))
	for _, name := range tree.paths {
		known[name] = true
	}
	return &deferredSources{tree: tree, result: result, files: make(map[string][]string), known: known}
}

// hold records a skipped Go file for later. Go never builds testdata or
// node_modules, so their files are not held.
func (deferred *deferredSources) hold(name string) {
	elements := strings.Split(path.Dir(name), "/")
	never := slices.Contains(elements, "testdata") || slices.Contains(elements, "node_modules")
	if never || !strings.HasSuffix(name, ".go") {
		return
	}
	directory := path.Dir(name)
	deferred.files[directory] = append(deferred.files[directory], name)
}

// loadDeferred reads the held files of directory into sources. A file that
// cannot be read is protected and reported, like any other rule input.
func (sources *goSources) loadDeferred(directory string) {
	deferred := sources.deferred
	names := deferred.files[directory]
	delete(deferred.files, directory)
	for _, name := range names {
		if err := sources.readFile(deferred.tree, name); err != nil {
			deferred.result.protect(name, err)
		}
	}
}

// holdsPackage reports whether directory has Go files, read or held.
func (sources *goSources) holdsPackage(directory string) bool {
	return len(sources.directories[directory]) > 0 || len(sources.deferred.files[directory]) > 0
}

// vendorRoots are the directories whose vendor/ the build of context reads:
// the workspace's in workspace mode, else the module's, when it has
// vendor/modules.txt. Go ignores a member module's vendor/ in workspace
// mode, so a stale one is never followed. Go then compiles every package
// outside the main modules from vendor/, local replacements included,
// without checking it against go.sum.
func (sources *goSources) vendorRoots(context goContext) []string {
	root := context.vendorRoot()
	if root == "" || !sources.deferred.known[vendorManifest(root)] {
		return nil
	}
	return []string{root}
}

// vendorRoot is the one directory whose vendor/ Go may read for context.
func (context goContext) vendorRoot() string {
	if context.workspace != "" {
		return context.workspace
	}
	return context.module
}

func vendorManifest(root string) string {
	return path.Join(root, "vendor", "modules.txt")
}

// linkedVendor reports whether root's vendor/ is reached through a
// symbolic link. refuseLinkedVendor reports the link once; resolving
// imports through it would only repeat that cause for every package.
func (sources *goSources) linkedVendor(root string) bool {
	_, linked := sources.deferred.tree.linkOn(path.Join(root, "vendor"))
	return linked
}

// vendoredImports are the vendored packages an import resolves to.
func (sources *goSources) vendoredImports(context goContext, location string) []localPackage {
	var packages []localPackage
	for _, root := range sources.vendorRoots(context) {
		if sources.linkedVendor(root) {
			continue
		}
		directory := path.Join(root, "vendor", location)
		if sources.holdsPackage(directory) {
			packages = append(packages, localPackage{module: root, directory: directory, vendored: true})
		}
	}
	return packages
}

// unusedReplacement is a replace target outside the repository in a build
// that vendors: Go compiles the vendored copy, which ownership protects.
func (sources *goSources) unusedReplacement(context goContext, replace moduleReplace) bool {
	return !fs.ValidPath(replace.directory) && sources.vendorsByDefault(context)
}

// vendorsByDefault mirrors the go command's default -mod=vendor: the
// selected vendor/ has modules.txt, the selecting go.work or go.mod says
// go 1.14 or later, and modules.txt was written for the same mode. Anything
// else builds the replacement itself, so its refusal must stand.
func (sources *goSources) vendorsByDefault(context goContext) bool {
	roots := sources.vendorRoots(context)
	if len(roots) == 0 {
		return false
	}
	workspaceMode := context.workspace != ""
	manifest := sources.modules[roots[0]]
	if workspaceMode {
		manifest = sources.workspaces[roots[0]]
	}
	recent := manifest.goVersion != "" && version.Compare("go"+manifest.goVersion, "go1.14") >= 0
	forWorkspace, readable := sources.vendoredForWorkspace(roots[0])
	sameMode := readable && forWorkspace == workspaceMode
	return recent && sameMode
}

// vendoredForWorkspace reports whether root's modules.txt starts with the
// "## workspace" annotation `go work vendor` writes, and whether it could
// be read: an unreadable one never waives a refusal.
func (sources *goSources) vendoredForWorkspace(root string) (bool, bool) {
	data, err := sources.deferred.tree.read(vendorManifest(root))
	if err != nil {
		return false, false
	}
	line, _, _ := strings.Cut(string(data), "\n")
	annotations, annotated := strings.CutPrefix(line, "## ")
	if !annotated {
		return false, true
	}
	for _, entry := range strings.Split(annotations, ";") {
		if strings.TrimSpace(entry) == "workspace" {
			return true, true
		}
	}
	return false, true
}

var errLinkedVendor = errors.New("vendored input is a symbolic link: lawgate refuses symlinked vendor directories")

// refuseLinkedVendor reports a symbolic link at or under a module's or
// workspace's vendor/. Git lists a linked directory as one path, so the
// vendored code Go compiles through it would never reach ownership.
func (sources *goSources) refuseLinkedVendor(tree sourceTree, result *classification) {
	for _, name := range tree.paths {
		if tree.links[name] && sources.underVendor(name) {
			result.protect(name, errLinkedVendor)
		}
	}
}

// underVendor reports whether name is a module's or workspace's vendor/
// directory or lies inside one.
func (sources *goSources) underVendor(name string) bool {
	elements := strings.Split(name, "/")
	for index, element := range elements {
		root := path.Join(append([]string{"."}, elements[:index]...)...)
		_, module := sources.modules[root]
		_, workspace := sources.workspaces[root]
		goRoot := module || workspace
		if element == "vendor" && goRoot {
			return true
		}
	}
	return false
}
