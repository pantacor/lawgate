package rulechanges

import (
	"fmt"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

type moduleCommand struct {
	module string
	args   []string
}

func (inputs *ruleInputs) markCommands(paths []string, result *classification) map[packageContext]bool {
	seeds := make(map[packageContext]bool)
	for _, command := range inputs.commands {
		if err := inputs.markCommand(command, paths, result, seeds); err != nil {
			result.protect(inventory.Filename, fmt.Errorf("command in module %q: %w", command.module, err))
		}
	}
	return seeds
}

func (inputs *ruleInputs) markCommand(command moduleCommand, paths []string, result *classification, seeds map[packageContext]bool) error {
	parsed, err := inventory.ParseGoCommand(command.module, command.args)
	if err != nil {
		return err
	}
	inputs.configured = append(inputs.configured, command.markFiles(paths, result.rules, parsed.Operands)...)
	if parsed.Subcommand == "" {
		return nil
	}
	tree := inputs.sources.deferred.tree
	directory, err := selectedRepoPath(tree, parsed.Dir)
	if err != nil {
		return fmt.Errorf("-C: %w", err)
	}
	command.module = directory
	inputs.configured = append(inputs.configured, command.markFiles(paths, result.rules, parsed.Operands)...)
	scan := commandScan{sources: inputs.sources, result: result, seeds: seeds}
	explicitSelection := parsed.Workspace != "" || parsed.ModFile != "" || parsed.Mod != ""
	if explicitSelection {
		return scan.markSelectedBuild(tree, parsed, directory)
	}
	scan.build = scan.sources.goContext(directory)
	return scan.markOperands(tree, directory, parsed.Operands)
}

// Selected manifests give this invocation its own view of the modules.
// Later commands and rules discovered independently still use their own view.
func (scan commandScan) markSelectedBuild(tree sourceTree, parsed inventory.GoCommand, directory string) error {
	sources := *scan.sources
	sources.modules = maps.Clone(sources.modules)
	sources.workspaces = maps.Clone(sources.workspaces)
	if parsed.Mod == "mod" || parsed.Mod == "readonly" {
		sources.deferred = withoutVendor(sources.deferred)
	}
	scan.sources = &sources
	scan.seeds = make(map[packageContext]bool)
	scan.build = sources.goContext(directory)
	if err := scan.selectWorkspace(tree, parsed.Workspace); err != nil {
		return err
	}
	if err := scan.selectModFile(tree, parsed.ModFile); err != nil {
		return err
	}
	// A selected workspace's vendor/ is one the repository-wide check
	// never sees, so its symbolic links are refused here.
	scan.sources.refuseLinkedVendor(tree, scan.result)
	if err := scan.markOperands(tree, directory, parsed.Operands); err != nil {
		return err
	}
	reach := newRuleReach(scan.sources, scan.result)
	for key := range scan.seeds {
		reach.enterPackageIn(key.directory, key.build)
	}
	reach.protectPackageInputs(tree.paths)
	return nil
}

func withoutVendor(deferred *deferredSources) *deferredSources {
	selected := *deferred
	selected.known = maps.Clone(deferred.known)
	for name := range selected.known {
		if name == "vendor/modules.txt" || strings.HasSuffix(name, "/vendor/modules.txt") {
			delete(selected.known, name)
		}
	}
	return &selected
}

func (scan *commandScan) selectWorkspace(tree sourceTree, filename string) error {
	switch filename {
	case "":
		return nil
	case "off":
		scan.build.workspace = ""
		return nil
	}
	name, err := selectedRepoPath(tree, filename)
	if err != nil {
		return fmt.Errorf("GOWORK: %w", err)
	}
	workspace, err := scan.selectedManifest(tree, name, path.Dir(name))
	if err != nil {
		return fmt.Errorf("GOWORK: %w", err)
	}
	if err := scan.protectSum(tree, name+".sum"); err != nil {
		return fmt.Errorf("GOWORK: %w", err)
	}
	scan.build.workspace = path.Dir(name)
	scan.sources.workspaces[scan.build.workspace] = workspace
	return nil
}

func (scan commandScan) selectModFile(tree sourceTree, filename string) error {
	if filename == "" {
		return nil
	}
	if scan.build.workspace != "" || scan.build.module == "" {
		return fmt.Errorf("-modfile requires a module with workspace mode disabled")
	}
	name, err := selectedRepoPath(tree, filename)
	if err != nil {
		return fmt.Errorf("-modfile: %w", err)
	}
	manifest, err := scan.selectedManifest(tree, name, scan.build.module)
	if err != nil {
		return fmt.Errorf("-modfile: %w", err)
	}
	if manifest.module == "" {
		return fmt.Errorf("-modfile %q is missing its module directive", name)
	}
	if err := scan.protectSum(tree, strings.TrimSuffix(name, ".mod")+".sum"); err != nil {
		return fmt.Errorf("-modfile: %w", err)
	}
	scan.sources.modules[scan.build.module] = manifest
	return nil
}

func (scan commandScan) selectedManifest(tree sourceTree, name, base string) (goManifest, error) {
	scan.result.rules[name] = true
	if !slices.Contains(tree.paths, name) {
		return goManifest{}, fmt.Errorf("manifest %q is missing from the repository tree", name)
	}
	if gosource.Sensitive(name) {
		return goManifest{}, fmt.Errorf("cannot inspect Go manifest %q", name)
	}
	data, err := tree.read(name)
	if err != nil {
		return goManifest{}, err
	}
	manifest, err := parseManifest(path.Join(base, path.Base(name)), data)
	manifest.relativize(tree.root)
	return manifest, err
}

func (scan commandScan) protectSum(tree sourceTree, name string) error {
	if !slices.Contains(tree.paths, name) {
		return nil
	}
	scan.result.rules[name] = true
	_, err := selectedRepoPath(tree, name)
	return err
}

func selectedRepoPath(tree sourceTree, input string) (string, error) {
	name := filepath.ToSlash(repositoryDirectory(tree.root, input))
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("%q is outside the repository: lawgate cannot protect it", input)
	}
	if link, linked := tree.linkOn(name); linked {
		return "", fmt.Errorf("Go input %q resolves through symlink %q", name, link)
	}
	return name, nil
}

func (scan commandScan) markOperands(tree sourceTree, directory string, operands []string) error {
	for _, operand := range operands {
		if err := scan.markOperand(tree, directory, operand); err != nil {
			return err
		}
	}
	return nil
}

func (scan commandScan) markOperand(tree sourceTree, directory, operand string) error {
	file := strings.HasSuffix(operand, ".go")
	local := strings.HasPrefix(operand, ".") || filepath.IsAbs(operand)
	if file || local {
		return scan.markLocalPath(tree, directory, operand)
	}
	scan.markGoOperand(operand)
	return nil
}

func (scan commandScan) markLocalPath(tree sourceTree, directory, operand string) error {
	name := filepath.FromSlash(operand)
	if !filepath.IsAbs(name) {
		name = filepath.Join(directory, name)
	}
	name, err := selectedRepoPath(tree, name)
	if err != nil {
		return err
	}
	if strings.HasSuffix(operand, ".go") {
		if !slices.Contains(tree.paths, name) {
			return fmt.Errorf("Go source operand %q is missing from the repository tree", name)
		}
		scan.seedDirectory(path.Dir(name))
		return nil
	}
	scan.markGoPackage(name)
	return nil
}

// markFiles protects the repository files a command's arguments name, and
// returns every argument that could name one, so a path reached through a
// symbolic link is refused even when Git lists only the link.
func (command moduleCommand) markFiles(paths []string, rules map[string]bool, operands []string) []string {
	var candidates []string
	for _, argument := range command.args {
		name := path.Join(command.module, argument)
		if slices.Contains(operands, argument) || !candidateCommandPath(name, argument) {
			continue
		}
		candidates = append(candidates, name)
		if slices.Contains(paths, name) {
			rules[name] = true
		}
	}
	return candidates
}

func candidateCommandPath(name, argument string) bool {
	relative := !path.IsAbs(argument) && !strings.HasPrefix(argument, "-")
	valid := argument != "" && fs.ValidPath(name)
	return relative && valid
}

type commandScan struct {
	sources *goSources
	result  *classification
	seeds   map[packageContext]bool
	build   goContext
}

func (scan commandScan) markGoOperand(operand string) {
	if operand == "all" {
		for _, main := range scan.sources.mainModules(scan.build) {
			scan.markGoPackage(path.Join(main, "..."))
		}
		return
	}
	location, _ := strings.CutSuffix(operand, "/...")
	for _, candidate := range scan.sources.localImports(scan.build, location) {
		scan.markLocalOperand(candidate, operand)
	}
}

func (scan commandScan) markLocalOperand(candidate localPackage, operand string) {
	recursive := strings.HasSuffix(operand, "/...")
	err := scan.sources.checkLocalModule(candidate)
	if err == nil && !recursive {
		err = scan.sources.checkLocalPackage(candidate)
	}
	if err != nil {
		scan.result.protect(inventory.Filename, fmt.Errorf("Go command package %q: %w", operand, err))
		return
	}
	packages := candidate.directory
	if recursive {
		packages = path.Join(packages, "...")
	}
	scan.markGoPackage(packages)
}

func (scan commandScan) seedDirectory(directory string) {
	scan.sources.loadDeferred(directory)
	if len(scan.sources.directories[directory]) == 0 {
		scan.readExplicitSource(directory)
	}
	scan.seeds[packageContext{directory: directory, build: scan.build}] = true
}

// Explicit operands can name packages that ordinary recursive scans skip.
func (scan commandScan) readExplicitSource(directory string) {
	tree := scan.sources.deferred.tree
	for _, name := range tree.paths {
		source := path.Dir(name) == directory && strings.HasSuffix(name, ".go")
		if !source {
			continue
		}
		if err := scan.sources.readFile(tree, name); err != nil {
			scan.result.protect(name, err)
		}
	}
}

func (scan commandScan) markGoPackage(pattern string) {
	base, recursive := strings.CutSuffix(pattern, "/...")
	if pattern == "..." {
		base, recursive = ".", true
	}
	if !recursive {
		scan.seedDirectory(base)
		return
	}
	module, _ := scan.sources.moduleDirectory(base)
	for directory := range scan.sources.directories {
		_, inside := within(directory, base)
		if !inside {
			continue
		}
		if actual, _ := scan.sources.moduleDirectory(directory); actual == module {
			scan.seeds[packageContext{directory: directory, build: scan.build}] = true
		}
	}
}
