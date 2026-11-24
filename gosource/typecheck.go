package gosource

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
)

type sourceBuild struct {
	fset     *token.FileSet
	std      types.Importer
	packages map[string]Package
	checked  map[string]*types.Package
	wanted   map[string]bool
	loading  map[string]bool
	syntax   map[string]*typedSource
}

func newSourceBuild(packages []Package) *sourceBuild {
	fset := token.NewFileSet()
	build := &sourceBuild{
		fset: fset, std: importer.ForCompiler(fset, "source", nil),
		packages: make(map[string]Package), checked: make(map[string]*types.Package),
		loading: make(map[string]bool), syntax: make(map[string]*typedSource),
	}
	for _, pkg := range packages {
		build.packages[pkg.ImportPath] = pkg
	}
	return build
}

type typedSource struct {
	files []*ast.File
	info  *types.Info
}

func (build *sourceBuild) load(pkg Package) error {
	if build.checked[pkg.ImportPath] != nil {
		return nil
	}
	if build.loading[pkg.ImportPath] {
		return fmt.Errorf("import cycle involving %s", pkg.ImportPath)
	}
	build.loading[pkg.ImportPath] = true
	defer delete(build.loading, pkg.ImportPath)
	var syntax []*ast.File
	for _, file := range pkg.Sources() {
		if pkg.ForTest == "" && isTestSource(file) {
			continue
		}
		parsed, err := parseBuildSource(build.fset, file)
		if err != nil {
			return err
		}
		syntax = append(syntax, parsed)
	}
	return build.typeCheck(pkg, syntax)
}

func (build *sourceBuild) typeCheck(pkg Package, syntax []*ast.File) error {
	var info *types.Info
	if build.wanted[pkg.ImportPath] {
		info = &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
	}
	imports := packageImporter{build: build, paths: pkg.ImportMap}
	options := types.Config{Importer: imports, FakeImportC: true, IgnoreFuncBodies: info == nil}
	checked, err := options.Check(pkg.ImportPath, build.fset, syntax, info)
	if err != nil {
		return err
	}
	build.checked[pkg.ImportPath] = checked
	if info != nil {
		build.syntax[pkg.ImportPath] = &typedSource{files: syntax, info: info}
	}
	return nil
}

func parseBuildSource(fset *token.FileSet, filename string) (*ast.File, error) {
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return nil, err
	}
	if resolved != filename {
		return nil, fmt.Errorf("%s was not scanned because it resolves through a symbolic link", filename)
	}
	return parser.ParseFile(fset, filename, nil, parser.SkipObjectResolution)
}

type packageImporter struct {
	build *sourceBuild
	paths map[string]string
}

func (source packageImporter) Import(path string) (*types.Package, error) {
	if resolved := source.paths[path]; resolved != "" {
		path = resolved
	}
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	build := source.build
	if checked := build.checked[path]; checked != nil {
		return checked, nil
	}
	pkg, found := build.packages[path]
	if !found || len(pkg.Sources()) == 0 {
		return nil, fmt.Errorf("Go build did not resolve source for %q", path)
	}
	if pkg.Standard {
		return build.std.Import(path)
	}
	err := build.load(pkg)
	return build.checked[path], err
}
