package rulechanges

import (
	"fmt"
	"go/ast"
	"path"
	"strconv"
	"strings"
)

// The closure is package-granular, within the owner's "rule files plus their
// callees" decision of 2026-11-20. A symbol
// walk cannot see what runs or is read without being referenced: init
// functions, TestMain, testdata, embedded files, the module manifest. So a
// package the closure reaches is protected whole: every Go file of its
// directory (tests and external tests included), its testdata tree, the
// files it embeds, and its module's go.mod and go.sum. Every repository
// package a reached package imports, blank imports included, is reached in
// turn: ownership does not depend on spotting each way one package can affect
// another.

// A package can have different imports when reached from different builds.
type packageContext struct {
	directory string
	build     goContext
}

func (reach *ruleReach) enterPackage(directory string) {
	reach.enterPackageIn(directory, reach.sources.goContext(directory))
}

func (reach *ruleReach) enterPackageIn(directory string, context goContext) {
	key := packageContext{directory: directory, build: context}
	if reach.contexts[key] {
		return
	}
	reach.contexts[key] = true
	reach.packages[directory] = true
	for _, file := range reach.sources.directories[directory] {
		reach.rules[file.name] = true
		reach.enqueueFile(file)
		for _, location := range file.importPaths {
			reach.enterLocalImport(file.name, location, context)
		}
	}
}

func (reach *ruleReach) enqueueFile(file *goFile) {
	for _, node := range file.syntax.Decls {
		reach.enqueue(declaration{file, node})
	}
}

// protectPackageInputs protects the files a reached package's build and
// tests read besides its Go code.
func (reach *ruleReach) protectPackageInputs(paths []string) {
	manifests := reach.moduleManifests()
	for _, name := range paths {
		if manifests[name] || reach.packageData(name) {
			reach.rules[name] = true
		}
	}
	reach.protectNativeInputs(paths)
}

func (reach *ruleReach) moduleManifests() map[string]bool {
	manifests := make(map[string]bool)
	for directory := range reach.packages {
		module, ok := reach.sources.moduleDirectory(directory)
		if !ok {
			continue
		}
		manifests[path.Join(module, "go.mod")] = true
		manifests[path.Join(module, "go.sum")] = true
	}
	for key := range reach.contexts {
		reach.workspaceManifests(manifests, key.build.workspace)
		reach.vendorManifests(manifests, key.build)
	}
	return manifests
}

func (reach *ruleReach) vendorManifests(manifests map[string]bool, build goContext) {
	for _, root := range reach.sources.vendorRoots(build) {
		manifests[vendorManifest(root)] = true
	}
}

func (reach *ruleReach) workspaceManifests(manifests map[string]bool, directory string) {
	workspace, ok := reach.sources.workspaces[directory]
	if !ok {
		return
	}
	filename := path.Join(directory, "go.work")
	if manifests[filename] {
		return
	}
	manifests[filename] = true
	manifests[path.Join(directory, "go.work.sum")] = true
	for _, module := range workspace.uses {
		if _, ok := reach.sources.modules[module]; !ok {
			reach.result.protect(filename, fmt.Errorf("cannot read workspace module %q", module))
			continue
		}
		manifests[path.Join(module, "go.mod")] = true
		manifests[path.Join(module, "go.sum")] = true
	}
}

func (reach *ruleReach) packageData(name string) bool {
	for directory := range reach.packages {
		relative, inside := relativeTo(name, directory)
		if !inside {
			continue
		}
		read := fixture(relative) || buildFile(relative)
		if read || reach.embedded(directory, relative) {
			return true
		}
	}
	return false
}

func fixture(relative string) bool {
	return strings.HasPrefix(relative, "testdata/")
}

// buildFile is a non-Go file of the package directory itself that the go
// command compiles or links into the package: assembly, cgo sources and
// headers, SWIG definitions, prebuilt objects.
func buildFile(relative string) bool {
	if strings.Contains(relative, "/") {
		return false
	}
	switch path.Ext(relative) {
	case ".s", ".S", ".sx", ".c", ".cc", ".cpp", ".cxx", ".m", ".h", ".hh", ".hpp", ".hxx",
		".f", ".F", ".for", ".f90", ".swig", ".swigcxx", ".syso":
		return true
	}
	return false
}

func (reach *ruleReach) embedded(directory, relative string) bool {
	for _, file := range reach.sources.directories[directory] {
		if file.embeds.includes(relative) {
			return true
		}
	}
	return false
}

// relativeTo is name relative to directory, when name is inside it.
func relativeTo(name, directory string) (string, bool) {
	if directory == "." {
		return name, true
	}
	return strings.CutPrefix(name, directory+"/")
}

// within is directory relative to module, when module holds it.
func within(directory, module string) (string, bool) {
	if directory == module {
		return ".", true
	}
	return relativeTo(directory, module)
}

// moduleDirectory is the directory of the innermost module holding directory.
func (sources *goSources) moduleDirectory(directory string) (string, bool) {
	best, found := "", false
	for module := range sources.modules {
		_, inside := within(directory, module)
		innermost := !found || depth(module) > depth(best)
		if inside && innermost {
			best, found = module, true
		}
	}
	return best, found
}

// depth counts a directory's path elements, the root "." having none, so a
// one-letter module ("a") is deeper than the root module it nests in.
func depth(directory string) int {
	if directory == "." {
		return 0
	}
	return strings.Count(directory, "/") + 1
}

// importPath is the import path of the package in directory, or "" when no
// module of the repository holds it.
func (sources *goSources) importPath(directory string) string {
	module, ok := sources.moduleDirectory(directory)
	if !ok {
		return ""
	}
	relative, _ := within(directory, module)
	return path.Join(sources.modules[module].module, relative)
}

// unaliasedImports are the import paths a file names by the imported
// package's own name.
func unaliasedImports(syntax *ast.File) []string {
	var paths []string
	for _, spec := range syntax.Imports {
		location, err := strconv.Unquote(spec.Path.Value)
		if err == nil && spec.Name == nil {
			paths = append(paths, location)
		}
	}
	return paths
}

// bindPackageNames lets a file refer to a repository package by its package
// clause name, which may differ from its directory's (example/ holding
// package worker, a /v2 module).
func (sources *goSources) bindPackageNames() {
	names := sources.packageNames()
	for _, file := range sources.files {
		sources.bindImports(file, names)
	}
}

func (sources *goSources) bindImports(file *goFile, names map[string]string) {
	context := sources.goContext(path.Dir(file.name))
	for _, location := range file.unaliased {
		bindImport(file, names, location, sources.localImports(context, location))
	}
}

func bindImport(file *goFile, names map[string]string, location string, packages []localPackage) {
	for _, candidate := range packages {
		if name := names[candidate.directory]; name != "" {
			file.imports[name] = location
		}
	}
}

// packageNames maps each directory to the name of its non-test package.
func (sources *goSources) packageNames() map[string]string {
	names := make(map[string]string)
	for directory, files := range sources.directories {
		if name := packageName(files); name != "" {
			names[directory] = name
		}
	}
	return names
}

// packageName is the package clause of a directory's files, external test
// packages aside.
func packageName(files []*goFile) string {
	for _, file := range files {
		name := file.syntax.Name.Name
		if !strings.HasSuffix(name, "_test") {
			return name
		}
	}
	return ""
}
