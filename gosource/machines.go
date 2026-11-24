package gosource

import (
	"errors"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
)

// MachinePackage is the import path of lawgate's built-in rule package.
const MachinePackage = "github.com/pantacor/lawgate/machine"

// Declaration is a machine origin. An empty Name is an unresolved origin.
type Declaration struct {
	Name     string
	Position token.Position
}

// MachineDeclarations finds named and unresolved machine origins in the
// selected packages of a module. Dependencies supply type information.
func MachineDeclarations(root string, packages []Package, rules map[string]bool) ([]Declaration, error) {
	root, err := CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	build := newSourceBuild(packages)
	inspected := inspectingPackages(root, packages, rules)
	build.wanted = inspected
	var problems error
	for _, pkg := range packages {
		if inspected[pkg.ImportPath] {
			problems = errors.Join(problems, build.load(pkg))
		}
	}
	if problems != nil {
		return nil, problems
	}
	facts := newMachineFacts()
	machines := build.machineTypes(rules)
	for _, pkg := range packages {
		if typed := build.syntax[pkg.ImportPath]; typed != nil {
			facts.collect(typed, machines)
		}
	}
	return facts.declarations(build.fset), nil
}

func machinePackage(path string, rules map[string]bool) bool {
	return path == MachinePackage || rules[path]
}

func inspectingPackages(root string, packages []Package, rules map[string]bool) map[string]bool {
	reached := ruleImporters(packages, rules)
	inspect := make(map[string]bool)
	for _, pkg := range packages {
		reachesRules := reached[pkg.ImportPath] && !machinePackage(pkg.ImportPath, rules)
		selected := reachesRules || len(pkg.InvalidGoFiles) > 0
		if selected && packageInModule(root, pkg) {
			inspect[pkg.ImportPath] = true
		}
	}
	return inspect
}

func packageInModule(root string, pkg Package) bool {
	return slices.ContainsFunc(pkg.Sources(), func(file string) bool {
		return pathWithin(root, file)
	})
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(relative)
}

func ruleImporters(packages []Package, rules map[string]bool) map[string]bool {
	dependents := packageDependents(packages)
	reached := make(map[string]bool)
	var queue []string
	for path := range dependents {
		if machinePackage(path, rules) {
			queue = append(queue, path)
		}
	}
	for len(queue) > 0 {
		path := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if !reached[path] {
			reached[path] = true
			queue = append(queue, dependents[path]...)
		}
	}
	return reached
}

func packageDependents(packages []Package) map[string][]string {
	dependents := make(map[string][]string)
	for _, pkg := range packages {
		for _, imported := range pkg.Imports {
			path := canonicalImport(pkg, imported)
			dependents[path] = append(dependents[path], pkg.ImportPath)
		}
	}
	return dependents
}

func canonicalImport(pkg Package, path string) string {
	if resolved := pkg.ImportMap[path]; resolved != "" {
		return resolved
	}
	return path
}

func (build *sourceBuild) machineTypes(rules map[string]bool) []types.Type {
	var machines []types.Type
	for path, pkg := range build.checked {
		if !machinePackage(path, rules) {
			continue
		}
		if name, ok := pkg.Scope().Lookup("Machine").(*types.TypeName); ok {
			machines = append(machines, types.Unalias(name.Type()))
		}
	}
	return machines
}

func isMachine(t types.Type, machines []types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if pointer, ok := t.Underlying().(*types.Pointer); ok {
		t = types.Unalias(pointer.Elem())
	}
	for _, machine := range machines {
		if t == machine || t.Underlying() == machine.Underlying() {
			return true
		}
	}
	return false
}

func isTestSource(name string) bool {
	return strings.HasSuffix(name, "_test.go")
}
