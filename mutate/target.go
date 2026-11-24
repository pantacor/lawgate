package mutate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
)

// Target is a subject found in its source file.
type Target struct {
	Subject Subject
	File    string
	Source  []byte
	fset    *token.FileSet
	decl    *ast.FuncDecl
}

// Find looks for the subject among a package's production files.
func Find(files []string, subject Subject) (Target, error) {
	for _, name := range files {
		target, found, err := findInFile(name, subject)
		if err != nil || found {
			return target, err
		}
	}
	return Target{}, fmt.Errorf("subject %s is not declared in the package's non-test Go files", subject)
}

func findInFile(name string, subject Subject) (Target, bool, error) {
	source, err := os.ReadFile(name)
	if err != nil {
		return Target{}, false, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, source, parser.SkipObjectResolution)
	if err != nil {
		return Target{}, false, err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && declares(fn, subject) {
			target := Target{Subject: subject, File: name, Source: source, fset: fset, decl: fn}
			return target, true, nil
		}
	}
	return Target{}, false, nil
}

func declares(fn *ast.FuncDecl, subject Subject) bool {
	hasBody := fn.Body != nil && fn.Name.Name == subject.Func
	return hasBody && receiverName(fn) == subject.Receiver
}

// receiverName is the method's type name without pointer or type parameters.
func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	switch typed := expr.(type) {
	case *ast.IndexExpr:
		expr = typed.X
	case *ast.IndexListExpr:
		expr = typed.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// Line is the line of the subject's declaration.
func (target Target) Line() int {
	return target.fset.Position(target.decl.Pos()).Line
}

// Fingerprint is the SHA-256 of the subject's declaration printed by
// go/printer: comments and layout do not change it, code does.
func (target Target) Fingerprint() (string, error) {
	plain := *target.decl
	plain.Doc = nil
	var text bytes.Buffer
	if err := printer.Fprint(&text, target.fset, &plain); err != nil {
		return "", err
	}
	sum := sha256.Sum256(text.Bytes())
	return hex.EncodeToString(sum[:]), nil
}
