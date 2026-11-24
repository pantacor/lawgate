// Package gocode finds, with go/parser, the places in Go source that lawgate
// review ties to laws: the line range of every function and method, and the
// composite literals that declare a machine's sites. It reads syntax only, so
// it works on any module without building it.
package gocode

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Range is a range of lines of a file, relative to a root.
type Range struct {
	File  string `json:"file"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Contains reports whether line is in the range.
func (r Range) Contains(line int) bool {
	return r.Start <= line && line <= r.End
}

// Overlaps reports whether the range shares a line with from..to of file.
func (r Range) Overlaps(file string, from, to int) bool {
	shares := r.Start <= to && r.End >= from
	return r.File == file && shares
}

// Functions maps every function and method declared in the non-test Go files
// of dir (relative to root) to its lines, as "Func" and "Type.Method".
func Functions(root, dir string) map[string]Range {
	return functionsIn(root, dir, isSourceFile)
}

// TestFunctions is Functions over the _test.go files of dir.
func TestFunctions(root, dir string) map[string]Range {
	return functionsIn(root, dir, isTestFile)
}

func functionsIn(root, dir string, wanted func(name string) bool) map[string]Range {
	ranges := map[string]Range{}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return ranges
	}
	for _, entry := range entries {
		if entry.IsDir() || !wanted(entry.Name()) {
			continue
		}
		addFileFunctions(ranges, root, path.Join(dir, entry.Name()))
	}
	return ranges
}

func isSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func isTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.go")
}

// addFileFunctions adds the functions of one file; a file that does not
// parse adds none (the build reports it, not the review).
func addFileFunctions(ranges map[string]Range, root, file string) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, filepath.Join(root, filepath.FromSlash(file)), nil, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	for _, decl := range parsed.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		lines := Range{File: file, Start: fset.Position(function.Pos()).Line, End: fset.Position(function.End()).Line}
		ranges[FunctionName(function)] = lines
	}
}

// FunctionName is "Func", or "Type.Method" for a method.
func FunctionName(function *ast.FuncDecl) string {
	receiver := receiverName(function)
	if receiver == "" {
		return function.Name.Name
	}
	return receiver + "." + function.Name.Name
}

// receiverName is a method's type name without pointer or type parameters.
func receiverName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return ""
	}
	expr := function.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	switch typed := expr.(type) {
	case *ast.IndexExpr:
		expr = typed.X
	case *ast.IndexListExpr:
		expr = typed.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// Enclosing finds the innermost function of file that holds line.
func Enclosing(ranges map[string]Range, file string, line int) (Range, bool) {
	var best Range
	found := false
	for _, candidate := range ranges {
		holds := candidate.File == file && candidate.Contains(line)
		if !holds {
			continue
		}
		if !found || candidate.End-candidate.Start < best.End-best.Start {
			best, found = candidate, true
		}
	}
	return best, found
}

// FunctionAt returns the function of file (relative to root) whose
// declaration starts on line.
func FunctionAt(root, file string, line int) (Range, bool) {
	ranges := map[string]Range{}
	addFileFunctions(ranges, root, file)
	for _, candidate := range ranges {
		if candidate.Start == line {
			return candidate, true
		}
	}
	return Range{}, false
}
