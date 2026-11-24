package rulechanges

import (
	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

// The framework's own rule packages. A rule package is one whose
// declarations are rules: a file that uses one of its names is a rule file.
// Products list theirs in lawgate.json "rule_packages", so no product name
// is built into the classifier.
const (
	provePackage   = "github.com/pantacor/lawgate/prove"
	runnerPackage  = "github.com/pantacor/lawgate/runner"
	resolvePackage = "github.com/pantacor/lawgate/resolve"
	machineType    = "Machine"
	machinePackage = gosource.MachinePackage
)

var machineRuleNames = []string{machineType, "Law", "Edge", "Run", "CheckLaws"}

// A machine command's entries (what runs, which laws, which source) and the
// resolve vocabulary (what a call site means) decide what the gate checks,
// wherever a product puts them.
var (
	runnerRuleNames  = []string{"Entry", "Main"}
	resolveRuleNames = []string{"Vocabulary"}
)

type packageRules struct {
	every bool
	names map[string]bool
}

type vocabulary map[string]*packageRules

func frameworkVocabulary() vocabulary {
	words := make(vocabulary)
	words.add(inventory.RulePackage{Path: provePackage})
	words.add(inventory.RulePackage{Path: machinePackage, Names: machineRuleNames})
	words.add(inventory.RulePackage{Path: runnerPackage, Names: runnerRuleNames})
	words.add(inventory.RulePackage{Path: resolvePackage, Names: resolveRuleNames})
	return words
}

func (words vocabulary) add(rule inventory.RulePackage) {
	entry := words[rule.Path]
	if entry == nil {
		entry = &packageRules{names: make(map[string]bool)}
		words[rule.Path] = entry
	}
	entry.every = entry.every || len(rule.Names) == 0
	for _, name := range rule.Names {
		entry.names[name] = true
	}
}

func (words vocabulary) isRule(importPath, name string) bool {
	entry := words[importPath]
	if entry == nil {
		return false
	}
	return entry.every || entry.names[name]
}

func (words vocabulary) addAll(rules []inventory.RulePackage) {
	for _, rule := range rules {
		words.add(rule)
	}
}
