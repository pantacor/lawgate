package rulechanges

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

type goContext struct {
	module    string
	workspace string
}

type localPackage struct {
	module    string
	directory string
	// vendored packages live in module's vendor/ directory, which may be a
	// workspace root rather than a module.
	vendored bool
}

// goContext is the build Go uses in directory: the nearest go.work above
// it, when that workspace uses the directory's module. Go does not look
// further up, and a module its nearest workspace omits builds on its own
// (with GOWORK=off; without it, the go command refuses).
func (sources *goSources) goContext(directory string) goContext {
	module, _ := sources.moduleDirectory(directory)
	context := goContext{module: module}
	workspace, found := sources.nearestWorkspace(directory)
	uses := sources.workspaces[workspace].uses
	member := module == "" || slices.Contains(uses, module)
	if found && member {
		context.workspace = workspace
	}
	return context
}

func (sources *goSources) nearestWorkspace(directory string) (string, bool) {
	for parent := directory; ; parent = path.Dir(parent) {
		if _, ok := sources.workspaces[parent]; ok {
			return parent, true
		}
		if parent == "." {
			return "", false
		}
	}
}

func (sources *goSources) localImports(context goContext, location string) []localPackage {
	var packages []localPackage
	for _, directory := range sources.mainModules(context) {
		manifest := sources.modules[directory]
		candidate, matches := importFromModule(location, manifest.module, directory)
		actual, _ := sources.moduleDirectory(candidate.directory)
		if matches && actual == directory {
			packages = append(packages, candidate)
		}
	}
	for _, replace := range sources.replacements(context) {
		candidate, matches := importFromModule(location, replace.module, replace.directory)
		built := !sources.unusedReplacement(context, replace) && !sources.crossesModule(candidate)
		if matches && built {
			packages = append(packages, candidate)
		}
	}
	packages = append(packages, sources.vendoredImports(context, location)...)
	slices.SortFunc(packages, func(a, b localPackage) int {
		if a.directory == b.directory {
			return strings.Compare(a.module, b.module)
		}
		return strings.Compare(a.directory, b.directory)
	})
	return slices.Compact(packages)
}

func (sources *goSources) mainModules(context goContext) []string {
	if workspace, ok := sources.workspaces[context.workspace]; ok {
		return workspace.uses
	}
	if context.module != "" {
		return []string{context.module}
	}
	return nil
}

func (sources *goSources) replacements(context goContext) []moduleReplace {
	workspace := sources.workspaces[context.workspace]
	replacements := slices.Clone(workspace.replacements)
	for _, directory := range sources.mainModules(context) {
		for _, replace := range sources.modules[directory].replacements {
			replacements = includeReplacement(replacements, workspace.replacements, replace)
		}
	}
	return replacements
}

func includeReplacement(replacements, overrides []moduleReplace, replace moduleReplace) []moduleReplace {
	if slices.ContainsFunc(overrides, replace.overrides) {
		return replacements
	}
	return append(replacements, replace)
}

func (replace moduleReplace) overrides(workspace moduleReplace) bool {
	sameModule := workspace.module == replace.module
	sameVersion := workspace.version == "" || workspace.version == replace.version
	return sameModule && sameVersion
}

func importFromModule(location, module, directory string) (localPackage, bool) {
	if module == "" || directory == "" {
		return localPackage{}, false
	}
	suffix, prefixed := strings.CutPrefix(location, module+"/")
	if prefixed {
		return localPackage{module: directory, directory: path.Join(directory, suffix)}, true
	}
	return localPackage{module: directory, directory: directory}, location == module
}

func (reach *ruleReach) enterLocalImport(importer, location string, context goContext) {
	for _, candidate := range reach.sources.localImports(context, location) {
		if err := reach.sources.checkLocalPackage(candidate); err != nil {
			reach.result.protect(importer, fmt.Errorf("local import %q: %w", location, err))
			continue
		}
		reach.enterPackageIn(candidate.directory, context)
	}
}

// checkLocalPackage reports a local package Go would build but that cannot
// be read. It reads the package's deferred files first (a vendored or
// "_"-prefixed package an import reaches).
func (sources *goSources) checkLocalPackage(candidate localPackage) error {
	if err := sources.checkLocalModule(candidate); err != nil {
		return err
	}
	sources.loadDeferred(candidate.directory)
	for _, file := range sources.directories[candidate.directory] {
		if !strings.HasSuffix(file.name, "_test.go") {
			return nil
		}
	}
	return fmt.Errorf("cannot read Go package in %q", candidate.directory)
}

func (sources *goSources) checkLocalModule(candidate localPackage) error {
	if !fs.ValidPath(candidate.module) || !fs.ValidPath(candidate.directory) {
		return fmt.Errorf("replace target %q is outside the repository: lawgate cannot protect it", candidate.module)
	}
	if candidate.vendored {
		return nil
	}
	module, _ := sources.moduleDirectory(candidate.directory)
	if module == "" || module != candidate.module {
		return fmt.Errorf("cannot read module %q for directory %q", candidate.module, candidate.directory)
	}
	return nil
}

// Go does not find a package inside another module through its parent module.
func (sources *goSources) crossesModule(candidate localPackage) bool {
	actual, _ := sources.moduleDirectory(candidate.directory)
	_, exists := sources.modules[candidate.module]
	return exists && actual != candidate.module
}
