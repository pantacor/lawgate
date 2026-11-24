package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/readability"
	"github.com/pantacor/lawgate/rulechanges"
)

func checkRepository(root string, config inventory.Config) (moduleResult, []string) {
	result := moduleResult{Path: "repository", Complete: true}
	var notes []string
	if config.Owner != "" {
		notes = checkOwners(root, config, &result)
	}
	if config.Readability == nil {
		return result, notes
	}
	return result, append(notes, checkReadability(root, config, &result)...)
}

// checkOwners records CODEOWNERS coverage and returns the ownership notes
// (approved outside include directories, system headers) for the summary.
func checkOwners(root string, config inventory.Config, result *moduleResult) []string {
	const name = "CODEOWNERS coverage"
	listed, err := rulechanges.RuleFiles(root)
	if err != nil {
		result.recordError(name, err)
		return listed.Notes
	}
	if err := rulechanges.CheckOwners(root, listed.Files, config.Owner, config.Platform); err != nil {
		recordProblems(result, name, []string{err.Error()})
		return listed.Notes
	}
	recordProblems(result, name, nil)
	return listed.Notes
}

func checkReadability(root string, config inventory.Config, result *moduleResult) []string {
	const name = "readability"
	baselinePath, err := baselineDestination(root, config.Readability.Baseline)
	if err != nil {
		result.recordError(name, err)
		return nil
	}
	baseline, err := readability.LoadBaseline(baselinePath)
	if err != nil {
		result.recordError(name, err)
		return nil
	}
	findings, err := readabilityFindings(root, config)
	if err != nil {
		result.recordError(name, err)
		return nil
	}
	recordProblems(result, name, readability.Ratchet(findings, baseline))
	return readability.Shrinkable(findings, baseline)
}

func readabilityFindings(root string, config inventory.Config) ([]readability.Finding, error) {
	var findings []readability.Finding
	for _, module := range config.Modules {
		if module.Language != "go" {
			continue
		}
		items, err := moduleFindings(root, module.Path, *config.Readability)
		if err != nil {
			return nil, err
		}
		findings = append(findings, items...)
	}
	return findings, nil
}

func moduleFindings(root, module string, config inventory.Readability) ([]readability.Finding, error) {
	if err := requireFile(root, filepath.Join(module, "go.mod")); err != nil {
		return nil, err
	}
	findings, err := readability.CheckDir(filepath.Join(root, module), func(name string) bool {
		repoPath := path.Join(module, name)
		included := len(config.Include) == 0 || matchesAny(config.Include, repoPath)
		return included && !matchesAny(config.Exclude, repoPath)
	})
	for index := range findings {
		findings[index].Path = path.Join(module, findings[index].Path)
	}
	return findings, err
}

func matchesAny(patterns []string, name string) bool {
	segments := strings.Split(name, "/")
	return slices.ContainsFunc(patterns, func(pattern string) bool {
		return matchSegments(strings.Split(pattern, "/"), segments)
	})
}

func matchSegments(pattern, name []string) bool {
	switch {
	case len(pattern) == 0:
		return len(name) == 0
	case pattern[0] == "**":
		for index := 0; index <= len(name); index++ {
			if matchSegments(pattern[1:], name[index:]) {
				return true
			}
		}
		return false
	case len(name) == 0:
		return false
	}
	matched, _ := path.Match(pattern[0], name[0])
	return matched && matchSegments(pattern[1:], name[1:])
}

func baselineCommand(arguments []string) error {
	flags := flag.NewFlagSet("readability-baseline", flag.ContinueOnError)
	rootFlag := flags.String("root", ".", "repository root")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("readability-baseline accepts flags only")
	}
	root, err := gosource.CanonicalRoot(*rootFlag)
	if err != nil {
		return err
	}
	return updateBaseline(root)
}

func updateBaseline(root string) error {
	config, err := inventory.Load(root)
	if err != nil {
		return err
	}
	if config.Readability == nil {
		return fmt.Errorf("readability is missing from %s", inventory.Filename)
	}
	filename, err := baselineDestination(root, config.Readability.Baseline)
	if err != nil {
		return err
	}
	findings, err := readabilityFindings(root, config)
	if err != nil {
		return err
	}
	return readability.WriteBaseline(filename, readability.CountByFile(findings))
}

func baselineDestination(root, name string) (string, error) {
	if err := refuseLinkedPath(root, name); err != nil {
		return "", fmt.Errorf("readability baseline %q: %w", name, err)
	}
	filename := filepath.Join(root, name)
	if _, err := os.Lstat(filename); !os.IsNotExist(err) {
		return filename, requireFile(root, name)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filename))
	if err != nil {
		return "", err
	}
	return filename, requireInside(root, name, parent)
}

func checkMachineInventory(root string, config inventory.Config, results []moduleResult, modules []string, err error) []string {
	var problems []string
	if err != nil {
		problems = append(problems, err.Error())
	}
	listed := make(map[string]bool)
	for index, module := range config.Modules {
		if module.Language != "go" {
			continue
		}
		listed[module.Path] = true
		if results[index].Ready {
			checkListedMachines(root, module, config.RulePackages, &results[index])
		}
	}
	for _, path := range modules {
		if listed[path] {
			continue
		}
		if err := checkUnlistedMachines(root, path, config.RulePackages); err != nil {
			problems = append(problems, err.Error())
		}
	}
	return problems
}

func checkListedMachines(root string, module inventory.Module, rules []inventory.RulePackage, result *moduleResult) {
	names, err := module.MachineNames(root, rules)
	if err != nil {
		result.recordError("machine declarations", err)
		result.Ready = false
		return
	}
	result.Declared = names
	if !*module.Machines && len(names) != 0 {
		err := fmt.Errorf("Go module %q declares machines and requires machines: true", module.Path)
		result.recordError("machine declarations", err)
	}
}

func checkUnlistedMachines(root, path string, rules []inventory.RulePackage) error {
	names, err := inventory.DeclaredMachineNames(root, path, rules)
	if err != nil {
		return fmt.Errorf("module %q: scanning for machine declarations: %w", path, err)
	}
	if len(names) != 0 {
		return fmt.Errorf("Go module %q declares machines but is not listed in %s", path, inventory.Filename)
	}
	return nil
}
