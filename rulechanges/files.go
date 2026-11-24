// Package rulechanges discovers code-owner rules and audits their Git history.
package rulechanges

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

type sourceTree struct {
	root  string
	paths []string
	read  func(string) ([]byte, error)
	// links are the paths that are symbolic links, so a rule input reached
	// through one is refused rather than protected at the wrong path.
	links map[string]bool
}

// RuleFileList is what RuleFiles finds: the rule files, and notes, facts
// the owner should see that do not fail the gate (an approved outside
// include directory, the system headers a cgo package reads), each once.
type RuleFileList struct {
	Files []string
	Notes []string
}

// RuleFiles lists the worktree's rule files. A file it cannot read or parse
// is listed (protected) and reported in the error; the rest is still listed.
func RuleFiles(root string) (RuleFileList, error) {
	root, err := gosource.CanonicalRoot(root)
	if err != nil {
		return RuleFileList{}, err
	}
	paths, links, err := trackedAndUntracked(root)
	if err != nil {
		return RuleFileList{}, err
	}
	tree := sourceTree{root: root, paths: paths, links: links, read: func(name string) ([]byte, error) {
		return readWorktreeFile(root, name)
	}}
	result := classify(tree)
	return RuleFileList{Files: sortedPaths(result.rules), Notes: result.notes}, result.err()
}

func trackedAndUntracked(root string) ([]string, map[string]bool, error) {
	list, err := gitCommand(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, nil, err
	}
	names := splitNUL(list)
	if !slices.Contains(names, inventory.Filename) {
		names = append(names, inventory.Filename)
	}
	var paths []string
	links := make(map[string]bool)
	for _, name := range names {
		info, err := os.Lstat(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		links[name] = info.Mode()&fs.ModeSymlink != 0
		if !info.IsDir() {
			paths = append(paths, name)
		}
	}
	return paths, links, nil
}

func readWorktreeFile(root, name string) ([]byte, error) {
	location, err := filepath.Abs(filepath.Join(root, name))
	if err != nil {
		return nil, err
	}
	actual, err := filepath.EvalSymlinks(location)
	if err != nil {
		return nil, err
	}
	if actual != location {
		return nil, fmt.Errorf("rule input %s is a symlink", name)
	}
	return os.ReadFile(location)
}

// classification is the set of rule files and the per-file problems met
// while finding them. A file with a problem is protected, never dropped.
type classification struct {
	rules    map[string]bool
	problems []error
	// reported are the paths with a problem and each problem's text, so
	// one cause met from several places is reported once.
	reported map[string]bool
	// notes are facts the owner should see that are not problems.
	notes []string
	// outsideIncludes are the outside directories lawgate.json approves.
	outsideIncludes []string
}

func newClassification() *classification {
	return &classification{rules: make(map[string]bool), reported: make(map[string]bool)}
}

func (result *classification) protect(name string, err error) {
	result.rules[name] = true
	problem := fmt.Errorf("%s: %w", name, err)
	if result.reported[problem.Error()] {
		return
	}
	result.reported[name] = true
	result.reported[problem.Error()] = true
	result.problems = append(result.problems, problem)
}

func (result *classification) err() error {
	return errors.Join(result.problems...)
}

// fatal is every problem except unparsable Go, which a history audit can
// carry on past because the file is already protected.
func (result *classification) fatal() error {
	var fatal []error
	for _, problem := range result.problems {
		if !errors.Is(problem, errUnparsable) {
			fatal = append(fatal, problem)
		}
	}
	return errors.Join(fatal...)
}

type ruleInputs struct {
	sources   *goSources
	enforcers *enforcers
	words     vocabulary
	commands  []moduleCommand
	// configured are the paths lawgate.json names for the gate to run or
	// read (gate files, command arguments), checked for symbolic links.
	configured []string
	// outsideIncludes are lawgate.json's approved outside directories.
	outsideIncludes []string
}

func classify(tree sourceTree) *classification {
	return classifyWith(tree, frameworkVocabulary())
}

func classifyWith(tree sourceTree, words vocabulary) *classification {
	result := newClassification()
	sources := newGoSources(newDeferredSources(tree, result))
	inputs := &ruleInputs{sources: sources, enforcers: newEnforcers(), words: words}
	for _, name := range tree.paths {
		result.classifyPath(tree, name, inputs)
	}
	result.outsideIncludes = inputs.outsideIncludes
	commandSeeds := inputs.markCommands(tree.paths, result)
	inputs.sources.markRules(tree.paths, result, inputs.enforcers, words, commandSeeds)
	markNodeRules(tree, result, inputs.enforcers, inputs.configured)
	inputs.enforcers.refuseLinkedScripts(tree, result)
	inputs.refuseLinkedConfigured(tree, result)
	inputs.sources.refuseLinkedVendor(tree, result)
	refuseLinkedRules(tree, result)
	return result
}

// refuseLinkedRules reports every rule file reached through a symbolic
// link that no earlier check reported: the readability baseline, a law
// document, CODEOWNERS or lawgate.json among them. Protecting the link
// would leave the file the gate actually reads unowned. A link already
// reported (a symlinked vendor/) is not reported again for each file
// beneath it.
func refuseLinkedRules(tree sourceTree, result *classification) {
	for _, name := range sortedPaths(result.rules) {
		link, linked := tree.linkOn(name)
		known := result.reported[name] || result.reported[link]
		if linked && !known {
			result.protect(name, fmt.Errorf("rule input is reached through the symbolic link %s: lawgate refuses symlinked rule inputs", link))
		}
	}
}

// refuseLinkedConfigured reports each configured input reached through a
// symbolic link: ownership would protect the link and leave the file the
// gate actually runs or reads unowned.
func (inputs *ruleInputs) refuseLinkedConfigured(tree sourceTree, result *classification) {
	for _, name := range inputs.configured {
		if link, linked := tree.linkOn(name); linked {
			result.protect(name, fmt.Errorf("configured input is reached through the symbolic link %s: lawgate refuses symlinked inputs", link))
		}
	}
}

// linkOn is the first path from the root down to name that is a symbolic
// link: name itself or a directory above it.
func (tree sourceTree) linkOn(name string) (string, bool) {
	prefix := ""
	for _, element := range strings.Split(name, "/") {
		prefix = path.Join(prefix, element)
		if tree.links[prefix] {
			return prefix, true
		}
	}
	return "", false
}

func (result *classification) classifyPath(tree sourceTree, name string, inputs *ruleInputs) {
	if platformRuleFile(name) || unreadSource(name) {
		result.rules[name] = true
	}
	if gosource.Skipped(name) {
		inputs.sources.deferred.hold(name)
		return
	}
	if fixedRuleFile(name) {
		result.rules[name] = true
	}
	if err := inputs.load(tree, name, result.rules); err != nil {
		result.protect(name, err)
	}
}

func (inputs *ruleInputs) load(tree sourceTree, name string, rules map[string]bool) error {
	switch {
	case gosource.Sensitive(name):
		return nil
	case path.Base(name) == "LAWS.md":
		content, err := tree.read(name)
		if err == nil {
			inputs.enforcers.add(name, content, rules)
		}
		return err
	case name == inventory.Filename:
		return inputs.loadConfig(tree, rules)
	case path.Base(name) == "go.mod" || path.Base(name) == "go.work":
		return inputs.sources.readManifest(tree, name)
	case strings.HasSuffix(name, ".go"):
		return inputs.sources.readFile(tree, name)
	default:
		return nil
	}
}

func (inputs *ruleInputs) loadConfig(tree sourceTree, rules map[string]bool) error {
	data, err := tree.read(inventory.Filename)
	if err != nil {
		return err
	}
	config, err := inventory.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	inputs.words.addAll(config.RulePackages)
	problems := []error{inputs.approveOutsideIncludes(tree, config)}
	if config.Readability != nil {
		rules[config.Readability.Baseline] = true
	}
	for _, name := range config.GateFiles {
		rules[name] = true
	}
	inputs.configured = append(inputs.configured, config.GateFiles...)
	for _, module := range config.Modules {
		problems = append(problems, inputs.recordModule(tree, rules, module))
	}
	return errors.Join(problems...)
}

// approveOutsideIncludes takes lawgate.json's approved outside include
// directories, unless one holds the repository: then none is approved,
// so every outside include is still reported.
func (inputs *ruleInputs) approveOutsideIncludes(tree sourceTree, config inventory.Config) error {
	if tree.root != "" {
		if err := config.CheckOutsideIncludes(tree.root); err != nil {
			return err
		}
	}
	inputs.outsideIncludes = config.OutsideIncludes
	return nil
}

// recordModule protects a module's gate files and loads every law document
// it names, whatever the document's name or directory, scoped to the module.
func (inputs *ruleInputs) recordModule(tree sourceTree, rules map[string]bool, module inventory.Module) error {
	documents := append([]string{module.Laws}, module.SharedLaws...)
	var problems []error
	for _, document := range documents {
		inputs.enforcers.modules[document] = append(inputs.enforcers.modules[document], module.Path)
		problems = append(problems, inputs.loadLawDocument(tree, rules, document))
	}
	inputs.enforcers.required[module.Path] = module.RequiredTests
	for _, command := range module.Commands() {
		inputs.commands = append(inputs.commands, moduleCommand{module.Path, command})
	}
	if module.Language == "node" {
		inputs.enforcers.nodeModules = append(inputs.enforcers.nodeModules, module)
	}
	manifest := path.Join(module.Path, "package.json")
	if module.Language == "node" && slices.Contains(tree.paths, manifest) {
		rules[manifest] = true
	}
	return errors.Join(problems...)
}

// loadLawDocument protects a configured law document and loads its
// enforcers. A missing one is still protected: the check reports it.
func (inputs *ruleInputs) loadLawDocument(tree sourceTree, rules map[string]bool, document string) error {
	rules[document] = true
	content, err := tree.read(document)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err == nil {
		inputs.enforcers.add(document, content, rules)
	}
	return err
}

// platformRuleFile is a file the code-review platform itself reads: CI
// configuration and the active CODEOWNERS. It is protected even inside the
// hidden directories the classifier otherwise skips.
func platformRuleFile(name string) bool {
	switch name {
	case ".gitlab-ci.yml", ".gitlab-ci.yaml", ".github/CODEOWNERS", ".gitlab/CODEOWNERS":
		return true
	}
	workflow := path.Dir(name) == ".github/workflows"
	configuration := strings.HasPrefix(name, ".github/actions/") || strings.HasPrefix(name, ".gitlab/")
	return workflow || configuration
}

func fixedRuleFile(name string) bool {
	switch path.Base(name) {
	case "LAWS.md", "CODEOWNERS":
		return true
	}
	return name == "Makefile" || name == inventory.Filename
}

// unreadSource is a Node source the classifier must not read because its
// path looks like a credential: protected unread rather than missed. Go
// source and manifests are read whatever their names (gosource.Sensitive).
func unreadSource(name string) bool {
	return nodeSource(name) && gosource.Sensitive(name)
}

func sortedPaths(paths map[string]bool) []string {
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
