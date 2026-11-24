package rulechanges

import (
	"fmt"
	"path"
	"regexp"
)

// nodeGraph follows an enforcing Node test through its relative imports, so
// a helper the test calls is as protected as the test itself.
type nodeGraph struct {
	tree    sourceTree
	result  *classification
	known   map[string]bool
	visited map[string]bool
	// declared are the files each enforcer's test(...) call appears in.
	declared map[string][]string
}

func newNodeGraph(tree sourceTree, result *classification) *nodeGraph {
	known := make(map[string]bool, len(tree.paths))
	for _, name := range tree.paths {
		known[name] = true
	}
	return &nodeGraph{tree: tree, result: result, known: known, visited: make(map[string]bool), declared: make(map[string][]string)}
}

func (graph *nodeGraph) follow(name string, data []byte) {
	graph.result.rules[name] = true
	if graph.visited[name] {
		return
	}
	graph.visited[name] = true
	if !nodeSource(name) {
		return
	}
	for _, target := range graph.imports(name, data) {
		graph.followFile(target)
	}
}

func (graph *nodeGraph) followFile(name string) {
	if graph.visited[name] {
		return
	}
	data, err := graph.tree.read(name)
	if err != nil {
		graph.result.protect(name, err)
		return
	}
	graph.follow(name, data)
}

// nodeImport matches require("./x"), import("./x"), import "./x" and
// from "./x", and the directory forms ".", "..", "./" and "../": only
// relative specifiers name files of this repository. The first group is
// the syntax, which decides the resolver.
var nodeImport = regexp.MustCompile(`(require\s*\(\s*|import\s*\(\s*|from\s+|import\s+)['"](\.\.?(?:/[^'"\r\n]*)?)['"]`)

// imports are the repository files name's relative imports load. One that
// loads no repository file is reported on name, which stays protected.
func (graph *nodeGraph) imports(name string, data []byte) []string {
	var targets []string
	for _, match := range nodeImport.FindAllSubmatch(data, -1) {
		specifier := string(match[2])
		lookup := graph.lookupFor(name, string(match[1]))
		resolved, err := lookup.resolve(path.Dir(name), specifier)
		if err != nil {
			graph.result.protect(name, fmt.Errorf("relative import %q: %w", specifier, err))
		}
		targets = append(targets, resolved...)
	}
	return targets
}
