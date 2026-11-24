package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/pantacor/lawgate/gosource"
)

// GoModules returns module paths relative to root, including "." when present.
func GoModules(root string) ([]string, error) {
	return gosource.Modules(root)
}

// GoPackages selects the active module packages using the Go build options.
func GoPackages(root string, command []string, timeouts ...time.Duration) ([]gosource.Package, error) {
	dir, err := gosource.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	parsed, err := ParseGoCommand(dir, command)
	if err != nil {
		return nil, err
	}
	targets := parsed.Operands
	if len(targets) == 0 {
		targets, err = discoverDirectories(parsed.Dir)
		if err != nil || len(targets) == 0 {
			return nil, err
		}
	}
	return listPackages(parsed, "-find", targets, timeoutOrDefault(timeouts))
}

// GoTestPackages selects the test variants and their transitive imports from
// the same build context as the command that runs them.
func GoTestPackages(root string, argv []string, timeout time.Duration) ([]gosource.Package, error) {
	dir, err := gosource.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	command, err := ParseGoCommand(dir, argv)
	if err != nil {
		return nil, err
	}
	if command.Subcommand != "test" {
		return nil, fmt.Errorf("package closure requires a direct Go test command")
	}
	return listPackages(command, "-deps", command.Operands, timeout)
}

func discoverDirectories(dir string) ([]string, error) {
	files, err := gosource.Files(dir)
	if err != nil {
		return nil, err
	}
	return packageDirectories(dir, files), nil
}

func listPackages(command GoCommand, selection string, targets []string, timeout time.Duration) ([]gosource.Package, error) {
	args := command.listArgs(selection, targets)
	data, stderr, err := command.runList(args, timeout)
	if err != nil {
		return nil, fmt.Errorf("go list sources: %w: %.4096s", err, stderr)
	}
	return gosource.ReadPackages(bytes.NewReader(data), nil)
}

func packageDirectories(root string, files map[string]bool) []string {
	directories := make(map[string]bool)
	for file := range files {
		name, _ := filepath.Rel(root, filepath.Dir(file))
		directories[filepath.ToSlash(name)] = true
	}
	paths := make([]string, 0, len(directories))
	for name := range directories {
		if name != "." {
			name = "./" + name
		}
		paths = append(paths, name)
	}
	slices.Sort(paths)
	return paths
}

// DeclaredMachineNames uses the default Go build to discover machine names.
func DeclaredMachineNames(root, module string, rules []RulePackage) ([]string, error) {
	return (Module{Path: module}).MachineNames(root, rules)
}

// MachineNames reports statically named machines and diagnoses unresolved ones.
func (module Module) MachineNames(root string, rules []RulePackage) ([]string, error) {
	declarations, problems := module.MachineDeclarations(root, rules)
	var names []string
	for _, declaration := range declarations {
		if declaration.Name == "" {
			problem := fmt.Errorf("%s: machine name cannot be resolved statically", declaration.Position)
			problems = errors.Join(problems, problem)
			continue
		}
		names = append(names, declaration.Name)
	}
	slices.Sort(names)
	return slices.Compact(names), problems
}

// MachineDeclarations returns named and unresolved declarations in the
// module's active machine build.
func (module Module) MachineDeclarations(root string, rules []RulePackage) ([]gosource.Declaration, error) {
	root, err := gosource.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, module.Path)
	command, err := ParseGoCommand(dir, module.machineSourceCommand())
	if err != nil {
		return nil, err
	}
	targets := command.Operands
	if len(targets) == 0 {
		targets, err = discoverDirectories(command.Dir)
		if err != nil || len(targets) == 0 {
			return nil, err
		}
	}
	packages, err := listPackages(command, "-deps", targets, module.Timeout())
	if err != nil {
		return nil, err
	}
	return gosource.MachineDeclarations(dir, packages, MachinePackages(rules))
}

// MachinePackages supplies the adopter types recognized alongside lawgate.
func MachinePackages(rules []RulePackage) map[string]bool {
	packages := make(map[string]bool)
	for _, rule := range rules {
		if len(rule.Names) == 0 || slices.Contains(rule.Names, "Machine") {
			packages[rule.Path] = true
		}
	}
	return packages
}
