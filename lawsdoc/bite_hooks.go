package lawsdoc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/pantacor/lawgate/gosource"
)

// GoBiteHooks finds literal calls to bite.Off in module-owned production
// files selected in the enforcing test package's tagged import closure.
// bitePackage is the import path of the bite package the calls must use.
func GoBiteHooks(root, bitePackage string, test TestFunc, packages []gosource.Package) (map[string]bool, error) {
	files, err := gosource.Files(root)
	if err != nil {
		return nil, err
	}
	sources := reachableBiteSources(test, packages)
	hooks := make(map[string]bool)
	for _, name := range sources {
		ignored := !files[name] || strings.HasSuffix(name, "_test.go")
		if ignored {
			continue
		}
		if err := scanBiteFile(name, bitePackage, hooks); err != nil {
			return nil, err
		}
	}
	return hooks, nil
}

func reachableBiteSources(test TestFunc, packages []gosource.Package) []string {
	byImport := make(map[string]gosource.Package, len(packages))
	var queue []string
	for _, pkg := range packages {
		byImport[pkg.ImportPath] = pkg
		if pkg.ForTest == test.Package && slices.Contains(pkg.Sources(), test.File) {
			queue = append(queue, pkg.ImportPath)
		}
	}
	var sources []string
	for len(queue) > 0 {
		pkg, unseen := byImport[queue[0]]
		queue = queue[1:]
		if !unseen || pkg.Standard {
			continue
		}
		delete(byImport, pkg.ImportPath)
		sources = append(sources, pkg.Sources()...)
		queue = append(queue, biteImports(pkg)...)
	}
	return sources
}

func biteImports(pkg gosource.Package) []string {
	imports := make([]string, 0, len(pkg.Imports))
	for _, path := range pkg.Imports {
		if mapped := pkg.ImportMap[path]; mapped != "" {
			path = mapped
		}
		imports = append(imports, path)
	}
	return imports
}

func scanBiteFile(name, bitePackage string, hooks map[string]bool) error {
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	alias := biteImportAlias(file, bitePackage)
	if alias == "" || alias == "_" {
		return nil
	}
	for _, decl := range file.Decls {
		if declaresName(decl, calledName(alias)) {
			continue // The declaration shadows the import: its calls are not hooks.
		}
		collectBiteCalls(decl, alias, hooks)
	}
	return nil
}

func collectBiteCalls(decl ast.Decl, alias string, hooks map[string]bool) {
	ast.Inspect(decl, func(node ast.Node) bool {
		if law := biteCallLaw(node, alias); law != "" {
			hooks[law] = true
		}
		return true
	})
}

func biteImportAlias(file *ast.File, bitePackage string) string {
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path != bitePackage {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return "bite"
	}
	return ""
}

func biteCallLaw(node ast.Node, alias string) string {
	call, ok := node.(*ast.CallExpr)
	candidate := ok && len(call.Args) == 1 && isBiteOff(ast.Unparen(call.Fun), alias)
	if !candidate {
		return ""
	}
	literal, ok := ast.Unparen(call.Args[0]).(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return ""
	}
	law, _ := strconv.Unquote(literal.Value)
	return law
}

func isBiteOff(function ast.Expr, alias string) bool {
	if alias == "." {
		return isName(function, "Off")
	}
	selector, ok := function.(*ast.SelectorExpr)
	matches := ok && selector.Sel.Name == "Off" && isName(selector.X, alias)
	return matches
}

func isName(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}

// calledName is the identifier a hook call starts with: the import's name,
// or Off itself under a dot import.
func calledName(alias string) string {
	if alias == "." {
		return "Off"
	}
	return alias
}

// declaresName reports whether anything inside the declaration (a parameter,
// a variable, a type) takes the name, which would hide the import there.
func declaresName(decl ast.Decl, name string) bool {
	found := false
	ast.Inspect(decl, func(node ast.Node) bool {
		found = found || slices.ContainsFunc(declaredNames(node), func(id *ast.Ident) bool {
			return id.Name == name
		})
		return !found
	})
	return found
}

func declaredNames(node ast.Node) []*ast.Ident {
	switch node := node.(type) {
	case *ast.Field:
		return node.Names
	case *ast.ValueSpec:
		return node.Names
	case *ast.TypeSpec:
		return []*ast.Ident{node.Name}
	case *ast.AssignStmt:
		return definedNames(node.Tok, node.Lhs...)
	case *ast.RangeStmt:
		return definedNames(node.Tok, node.Key, node.Value)
	}
	return nil
}

func definedNames(tok token.Token, targets ...ast.Expr) []*ast.Ident {
	if tok != token.DEFINE {
		return nil
	}
	var names []*ast.Ident
	for _, target := range targets {
		if id, ok := target.(*ast.Ident); ok {
			names = append(names, id)
		}
	}
	return names
}
