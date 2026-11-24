package rulechanges

import (
	"go/ast"
	"go/token"
	"path"
	"regexp"
	"strconv"
)

type ruleReach struct {
	sources  *goSources
	rules    map[string]bool
	result   *classification
	contexts map[packageContext]bool
	seen     map[ast.Node]bool
	queue    []declaration
	packages map[string]bool
}

func newRuleReach(sources *goSources, result *classification) *ruleReach {
	return &ruleReach{
		sources: sources, rules: result.rules, result: result,
		contexts: make(map[packageContext]bool),
		seen:     make(map[ast.Node]bool), packages: make(map[string]bool),
	}
}

func (reach *ruleReach) enqueue(entry declaration) {
	if reach.seen[entry.node] {
		return
	}
	reach.seen[entry.node] = true
	reach.rules[entry.file.name] = true
	reach.queue = append(reach.queue, entry)
	reach.includeMethods(entry)
	directory := path.Dir(entry.file.name)
	if !reach.packages[directory] {
		reach.enterPackage(directory)
	}
}

func (reach *ruleReach) enqueueAll(entries []declaration) {
	for _, entry := range entries {
		reach.enqueue(entry)
	}
}

// includeMethods follows a reached type to its methods: without type
// information, any of them may be what a selector on a value of it calls.
func (reach *ruleReach) includeMethods(entry declaration) {
	general, ok := entry.node.(*ast.GenDecl)
	if !ok || general.Tok != token.TYPE {
		return
	}
	for _, spec := range general.Specs {
		name := spec.(*ast.TypeSpec).Name.Name
		reach.enqueueAll(reach.sources.methods[entry.file.symbol(name)])
	}
}

func (reach *ruleReach) expand() {
	for len(reach.queue) > 0 {
		entry := reach.queue[0]
		reach.queue = reach.queue[1:]
		reach.inspect(entry.file, entry.node)
	}
}

func (reach *ruleReach) inspect(file *goFile, root ast.Node) {
	ast.Inspect(root, func(node ast.Node) bool {
		return reach.visit(file, node)
	})
}

func (reach *ruleReach) visit(file *goFile, node ast.Node) bool {
	switch item := node.(type) {
	case *ast.SelectorExpr:
		reach.includeSelector(file, item)
		return false // Selector names are not unqualified references.
	case *ast.KeyValueExpr:
		reach.includeGuardConstants(item)
	case *ast.Ident:
		reach.includeLocal(file, item)
	}
	return true
}

func (reach *ruleReach) includeLocal(file *goFile, name *ast.Ident) {
	object := name.Obj
	if object != nil && file.syntax.Scope.Lookup(name.Name) != object {
		return
	}
	reach.enqueueAll(reach.sources.symbols[file.symbol(name.Name)])
}

// includeSelector follows pkg.Name into the repository's own packages. For a
// chain such as pkg.Value.Method() or newT().Method(), it follows the
// receiver expression, whose type then brings in its methods.
func (reach *ruleReach) includeSelector(file *goFile, item *ast.SelectorExpr) {
	name, ok := item.X.(*ast.Ident)
	if !ok {
		reach.inspect(file, item.X)
		return
	}
	// Imported packages are already traversed whole by enterLocalImport.
	if file.imports[name.Name] == "" {
		reach.includeLocal(file, name)
	}
}

var guardWord = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)

func (reach *ruleReach) includeGuardConstants(item *ast.KeyValueExpr) {
	field, ok := item.Key.(*ast.Ident)
	if !ok {
		return
	}
	if field.Name != "Guard" && field.Name != "Limit" {
		return
	}
	literal, ok := item.Value.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return
	}
	guard, err := strconv.Unquote(literal.Value)
	if err != nil {
		return
	}
	for _, word := range guardWord.FindAllString(guard, -1) {
		reach.enqueueAll(reach.sources.constants[word])
	}
}
