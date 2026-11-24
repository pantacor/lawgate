package rulechanges

import (
	"bytes"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

// nodeTestCall matches a test(...) or it(...) call up to its title's
// opening quote. It runs over nodeCode, so comments and literals never
// match, and a member call (pattern.test(...)) is rejected after.
var nodeTestCall = regexp.MustCompile("\\b(?:test|it)(?:\\.(?:skip|todo|only))?\\s*\\(\\s*([\x22'\x60])")

// markNodeRules follows Node law tests, and the Node files already
// protected (a configured command, a script enforcer, a gate file), through
// their relative imports. A protected file is followed wherever it lives,
// "_" and "." directories included: only the search for unnamed law tests
// skips the directories Go's discovery skips. An enforcer is declared only
// by a file the runner loads: a default test file, or one a configured
// command names. A Node module's named enforcer that no test(...) or
// it(...) call declares is then bound to every test file of the module,
// and reported.
func markNodeRules(tree sourceTree, result *classification, enforcers *enforcers, configured []string) {
	scan := nodeRuleScan{graph: newNodeGraph(tree, result), seeds: maps.Clone(result.rules), loaded: loadedSet(configured)}
	for _, name := range tree.paths {
		if nodeSource(name) && !gosource.Sensitive(name) {
			scan.visit(name, enforcers)
		}
	}
	for _, module := range enforcers.nodeModules {
		scan.graph.bindUnmatched(module, enforcers.moduleNames(module))
	}
}

// nodeRuleScan is one pass of markNodeRules over the Node sources: seeds
// are the files protected before it, loaded the files a configured
// command names.
type nodeRuleScan struct {
	graph  *nodeGraph
	seeds  map[string]bool
	loaded map[string]bool
}

func loadedSet(configured []string) map[string]bool {
	loaded := make(map[string]bool, len(configured))
	for _, name := range configured {
		loaded[name] = true
	}
	return loaded
}

// visit follows a protected Node source and binds the enforcers name
// declares, when it may declare any.
func (scan nodeRuleScan) visit(name string, enforcers *enforcers) {
	if scan.seeds[name] {
		scan.graph.followFile(name)
	}
	if scan.declaresEnforcers(name) {
		scan.graph.markIfEnforcer(name, enforcers.namesFor(name))
	}
}

// declaresEnforcers reports whether name can declare an enforcer: a file
// the runner loads, in a directory searched for law tests or protected.
func (scan nodeRuleScan) declaresEnforcers(name string) bool {
	searched := scan.seeds[name] || !gosource.Skipped(name)
	runnable := nodeTestFile(name) || scan.loaded[name]
	return searched && runnable
}

func (graph *nodeGraph) markIfEnforcer(name string, names map[string]bool) {
	if len(names) == 0 {
		return
	}
	data, err := graph.tree.read(name)
	if err != nil {
		graph.result.protect(name, err)
		return
	}
	if graph.declare(name, names, data) {
		graph.follow(name, data)
	}
}

// declare records which of names file declares, and reports whether any.
func (graph *nodeGraph) declare(file string, names map[string]bool, data []byte) bool {
	declared := nodeDeclaredTests(names, data)
	for _, name := range declared {
		graph.declared[name] = append(graph.declared[name], file)
	}
	return len(declared) > 0
}

// bindUnmatched fails closed for each enforcer of module that no file of
// the module declares (a test registered under an alias of test or it):
// every test file Node's runner could load is protected with its imports,
// and the enforcer is reported so the owner binds it explicitly.
func (graph *nodeGraph) bindUnmatched(module inventory.Module, names []string) {
	var unmatched []string
	for _, name := range names {
		if !graph.declaredIn(name, module.Path) {
			unmatched = append(unmatched, name)
		}
	}
	if len(unmatched) == 0 {
		return
	}
	for _, file := range graph.tree.paths {
		if inModule(file, module.Path) && nodeTestFile(file) {
			graph.followFile(file)
		}
	}
	graph.result.protect(module.Laws, fmt.Errorf("Node enforcers %q match no test(...) or it(...) call in module %q: every test file of the module is protected; declare each with test or it so ownership can bind it", unmatched, module.Path))
}

func (graph *nodeGraph) declaredIn(name, module string) bool {
	for _, file := range graph.declared[name] {
		if inModule(file, module) {
			return true
		}
	}
	return false
}

// moduleNames are the test enforcers a module's own configuration names:
// its laws, its shared laws and its required tests.
func (enforcers *enforcers) moduleNames(module inventory.Module) []string {
	names := make(map[string]bool)
	documents := append([]string{module.Laws}, module.SharedLaws...)
	for _, document := range documents {
		recordRequiredNames(names, enforcers.tests[document])
	}
	recordRequiredNames(names, module.RequiredTests)
	return slices.Sorted(maps.Keys(names))
}

// nodeTestFile is a file `node --test` loads by default: *.test.*,
// *-test.*, *_test.*, test-*.*, test.* and anything under a test/
// directory, outside node_modules.
func nodeTestFile(name string) bool {
	elements := strings.Split(path.Dir(name), "/")
	if !nodeSource(name) || slices.Contains(elements, "node_modules") {
		return false
	}
	stem := strings.TrimSuffix(path.Base(name), path.Ext(name))
	suffixed := strings.HasSuffix(stem, ".test") || strings.HasSuffix(stem, "-test") || strings.HasSuffix(stem, "_test")
	prefixed := stem == "test" || strings.HasPrefix(stem, "test-")
	named := suffixed || prefixed
	return named || slices.Contains(elements, "test")
}

// nodeDeclaredTests are the names a test(...) or it(...) call of data
// declares, as a title or a title prefix followed by its closing quote.
// The title is read from data: nodeCode blanks it.
func nodeDeclaredTests(names map[string]bool, data []byte) []string {
	code := nodeCode(data)
	var declared []string
	for _, match := range nodeTestCall.FindAllSubmatchIndex(code, -1) {
		if memberCall(code, match[0]) {
			continue
		}
		quote := string(data[match[2]:match[3]])
		declared = append(declared, titledBy(names, quote, restOfLine(data, match[3]))...)
	}
	return declared
}

// memberCall reports whether the call starting at start is a property of
// something else (pattern.test, t.test, $test), not the runner's test.
func memberCall(code []byte, start int) bool {
	if start > 0 && code[start-1] == '$' {
		return true
	}
	before := bytes.TrimRight(code[:start], " \t\r\n")
	return bytes.HasSuffix(before, []byte("."))
}

func restOfLine(data []byte, start int) string {
	line := data[start:]
	if end := bytes.IndexAny(line, "\r\n"); end >= 0 {
		line = line[:end]
	}
	return string(line)
}

// titledBy are the names a call's title starts with, up to its quote.
func titledBy(names map[string]bool, quote, title string) []string {
	var titled []string
	for name := range names {
		if strings.HasPrefix(title, name+quote) {
			titled = append(titled, name)
		}
	}
	return titled
}
