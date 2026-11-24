package rulechanges

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path"
	"strconv"
	"strings"
)

// errUnparsable marks a Go file the classifier could not read; the file is
// protected anyway, so callers may treat it as a note rather than an abort.
var errUnparsable = errors.New("cannot parse Go source")

type symbol struct {
	Directory string
	Package   string
	Name      string
}

type goFile struct {
	name    string
	syntax  *ast.File
	imports map[string]string
	// unaliased are the import paths named by their package clause, which
	// markRules binds once every package's name is known.
	unaliased []string
	// importPaths are every path the file imports, blank and dot imports
	// included: each one the repository holds is reached (enterPackage).
	importPaths []string
	embeds      embedSet
}

func (file *goFile) symbol(name string) symbol {
	return symbol{path.Dir(file.name), file.syntax.Name.Name, name}
}

type declaration struct {
	file *goFile
	node ast.Node
}

type goSources struct {
	files       []*goFile
	directories map[string][]*goFile
	modules     map[string]goManifest
	workspaces  map[string]goManifest
	symbols     map[symbol][]declaration
	methods     map[symbol][]declaration
	constants   map[string][]declaration
	deferred    *deferredSources
}

func newGoSources(deferred *deferredSources) *goSources {
	return &goSources{
		deferred:    deferred,
		directories: make(map[string][]*goFile),
		modules:     make(map[string]goManifest), workspaces: make(map[string]goManifest),
		symbols: make(map[symbol][]declaration),
		methods: make(map[symbol][]declaration), constants: make(map[string][]declaration),
	}
}

func (sources *goSources) readFile(tree sourceTree, name string) error {
	content, err := tree.read(name)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	syntax, err := parser.ParseFile(fset, name, content, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("%w: %v", errUnparsable, err)
	}
	file := &goFile{name: name, syntax: syntax, imports: imports(syntax)}
	file.unaliased = unaliasedImports(syntax)
	file.importPaths = importPaths(syntax)
	file.embeds, err = readEmbeds(name, content, directiveLines(fset, syntax))
	sources.add(file)
	return err
}

func (sources *goSources) add(file *goFile) {
	sources.files = append(sources.files, file)
	directory := path.Dir(file.name)
	sources.directories[directory] = append(sources.directories[directory], file)
	for _, node := range file.syntax.Decls {
		sources.indexDeclaration(declaration{file, node})
	}
}

func importPaths(syntax *ast.File) []string {
	var paths []string
	for _, spec := range syntax.Imports {
		location, err := strconv.Unquote(spec.Path.Value)
		if err == nil {
			paths = append(paths, location)
		}
	}
	return paths
}

func imports(syntax *ast.File) map[string]string {
	result := make(map[string]string)
	for _, spec := range syntax.Imports {
		location, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(location)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		result[name] = location
	}
	return result
}

func (sources *goSources) indexDeclaration(entry declaration) {
	switch node := entry.node.(type) {
	case *ast.FuncDecl:
		sources.indexFunction(node, entry)
	case *ast.GenDecl:
		for _, spec := range node.Specs {
			sources.indexSpec(node.Tok, spec, entry)
		}
	}
}

// indexFunction files a method under its receiver type, so reaching the type
// reaches every method a caller could invoke on it.
func (sources *goSources) indexFunction(node *ast.FuncDecl, entry declaration) {
	if node.Recv == nil {
		sources.indexSymbol(node.Name.Name, entry)
		return
	}
	receiver := receiverType(node.Recv)
	if receiver == "" {
		return
	}
	key := entry.file.symbol(receiver)
	sources.methods[key] = append(sources.methods[key], entry)
}

func receiverType(fields *ast.FieldList) string {
	if len(fields.List) == 0 {
		return ""
	}
	expression := fields.List[0].Type
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.IndexExpr:
			expression = typed.X
		case *ast.IndexListExpr:
			expression = typed.X
		case *ast.Ident:
			return typed.Name
		default:
			return ""
		}
	}
}

func (sources *goSources) indexSpec(kind token.Token, spec ast.Spec, entry declaration) {
	switch typed := spec.(type) {
	case *ast.TypeSpec:
		sources.indexSymbol(typed.Name.Name, entry)
	case *ast.ValueSpec:
		for _, name := range typed.Names {
			sources.indexValue(kind, name.Name, entry)
		}
	}
}

func (sources *goSources) indexValue(kind token.Token, name string, entry declaration) {
	sources.indexSymbol(name, entry)
	if kind == token.CONST {
		sources.constants[name] = append(sources.constants[name], entry)
	}
}

func (sources *goSources) indexSymbol(name string, entry declaration) {
	key := entry.file.symbol(name)
	sources.symbols[key] = append(sources.symbols[key], entry)
}

// markRules protects every declaration that declares a rule or enforces a
// law, everything those declarations reach, transitively, and the whole of
// every package reached (see enterPackage). Go files already protected, such
// as a machine command's package, are seeds too.
func (sources *goSources) markRules(paths []string, result *classification, enforcers *enforcers, words vocabulary, commands map[packageContext]bool) {
	sources.bindPackageNames()
	reach := newRuleReach(sources, result)
	enforcing := sources.enforcingTests(enforcers)
	seeds := maps.Clone(result.rules)
	for key := range commands {
		reach.enterPackageIn(key.directory, key.build)
	}
	for _, file := range sources.files {
		reach.seedFile(file, enforcing, words, seeds)
	}
	for importPath := range words {
		reach.seedDefinitions(importPath, words)
	}
	reach.expand()
	reach.protectPackageInputs(paths)
}

func (reach *ruleReach) seedFile(file *goFile, enforcing map[testKey]bool, words vocabulary, seeds map[string]bool) {
	if seeds[file.name] {
		reach.enterPackage(path.Dir(file.name))
		return
	}
	for _, node := range file.syntax.Decls {
		entry := declaration{file, node}
		if isEnforcer(entry, enforcing) || declaresRule(entry, words) {
			reach.enterPackage(path.Dir(file.name))
			return
		}
	}
}

// seedDefinitions protects the rule names a rule package defines when that
// package lives in this repository (lawgate's own machine, a product's copy).
func (reach *ruleReach) seedDefinitions(importPath string, words vocabulary) {
	for key, targets := range reach.sources.symbols {
		defined := reach.sources.importPath(key.Directory) == importPath && ast.IsExported(key.Name)
		if defined && words.isRule(importPath, key.Name) {
			reach.enqueueAll(targets)
		}
	}
}

func isEnforcer(entry declaration, enforcing map[testKey]bool) bool {
	fn, ok := entry.node.(*ast.FuncDecl)
	if !ok || fn.Recv != nil {
		return false
	}
	return enforcing[testKey{entry.file.name, fn.Name.Name}]
}

func declaresRule(entry declaration, words vocabulary) bool {
	var found bool
	ast.Inspect(entry.node, func(node ast.Node) bool {
		found = found || isRuleNode(node, entry.file, words)
		return !found
	})
	return found
}

func isRuleNode(node ast.Node, file *goFile, words vocabulary) bool {
	switch item := node.(type) {
	case *ast.SelectorExpr:
		name, ok := item.X.(*ast.Ident)
		return ok && words.isRule(file.imports[name.Name], item.Sel.Name)
	case *ast.Ident:
		return words.isRule(file.imports["."], item.Name)
	case *ast.Comment:
		return strings.TrimSpace(item.Text) == "// lawgate:kernel"
	}
	return false
}
