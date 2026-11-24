package lawslock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/mutate"
)

type function struct {
	file string
	decl *ast.FuncDecl
}

func (fn function) name() string {
	if fn.decl.Recv == nil {
		return fn.decl.Name.Name
	}
	receiver := fn.decl.Recv.List[0].Type
	for {
		switch expr := receiver.(type) {
		case *ast.StarExpr:
			receiver = expr.X
		case *ast.IndexExpr:
			receiver = expr.X
		case *ast.IndexListExpr:
			receiver = expr.X
		case *ast.Ident:
			return expr.Name + "." + fn.decl.Name.Name
		default:
			return fn.decl.Name.Name
		}
	}
}

func parseSource(name string, read Reader) (*ast.File, error) {
	data, err := read(name)
	if err != nil {
		return nil, err
	}
	return parser.ParseFile(token.NewFileSet(), name, data, parser.ParseComments)
}

func fingerprint(fset *token.FileSet, decl *ast.FuncDecl) (string, error) {
	var source bytes.Buffer
	if err := printer.Fprint(&source, fset, decl); err != nil {
		return "", err
	}
	hash := sha256.Sum256(source.Bytes())
	return hex.EncodeToString(hash[:]), nil
}

func hashSubject(root string, subject mutate.Subject, pkg gosource.Package, read Reader) (string, error) {
	name := strings.TrimPrefix(subject.String(), pkg.ImportPath+".")
	for _, source := range pkg.Sources() {
		hash, err := sourceSubject(root, source, name, read)
		if err != nil || hash != "" {
			return hash, err
		}
	}
	return "", fmt.Errorf("subject %s has no function in the active Go build", subject)
}

func sourceSubject(root, source, name string, read Reader) (string, error) {
	relative, err := relativeFile(root, source)
	if err != nil {
		return "", err
	}
	data, err := read(relative)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relative, data, parser.SkipObjectResolution)
	if err != nil {
		return "", err
	}
	if decl := findSubject(file, name); decl != nil {
		// Match mutate.Target.Fingerprint, which ignores comments.
		return fingerprint(fset, decl)
	}
	return "", nil
}

func findSubject(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && namedSubject(fn, name) {
			return fn
		}
	}
	return nil
}

func namedSubject(fn *ast.FuncDecl, name string) bool {
	subject := function{decl: fn}
	return fn.Body != nil && subject.name() == name
}

type testScope struct {
	dir string
	pkg string
}

type proofSources struct {
	scopes map[string]testScope
	byName map[testScope]map[string][]function
}

// Proof hashes the named tests and reachable functions from their test packages.
func Proof(entry Entry, read Reader) (string, error) {
	sources := proofSources{scopes: make(map[string]testScope), byName: make(map[testScope]map[string][]function)}
	for _, name := range entry.TestFiles {
		if err := sources.addFile(name, read); err != nil {
			return "", err
		}
	}
	var hashes []string
	for _, test := range entry.Tests {
		parts, err := sources.testProof(test)
		if err != nil {
			return "", err
		}
		hashes = append(hashes, parts...)
	}
	slices.Sort(hashes)
	hash := sha256.Sum256([]byte(strings.Join(slices.Compact(hashes), "\n")))
	return hex.EncodeToString(hash[:]), nil
}

func (sources proofSources) addFile(name string, read Reader) error {
	if !localPath(name) || !strings.HasSuffix(name, "_test.go") {
		return fmt.Errorf("invalid proof source path %q", name)
	}
	file, err := parseSource(name, read)
	if err != nil {
		return err
	}
	scope := testScope{dir: path.Dir(name), pkg: file.Name.Name}
	sources.scopes[name] = scope
	byName := sources.byName[scope]
	if byName == nil {
		byName = make(map[string][]function)
		sources.byName[scope] = byName
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			byName[fn.Name.Name] = append(byName[fn.Name.Name], function{file: name, decl: fn})
		}
	}
	return nil
}

func (sources proofSources) testProof(test Test) ([]string, error) {
	byName := sources.byName[sources.scopes[test.File]]
	for _, fn := range byName[test.Name] {
		if fn.decl.Recv == nil && fn.file == test.File {
			return hashClosure(fn, byName)
		}
	}
	return nil, fmt.Errorf("locked test %s is missing from %s", test.QualifiedName(), test.File)
}

func hashClosure(root function, byName map[string][]function) ([]string, error) {
	seen := make(map[*ast.FuncDecl]bool)
	queue := []function{root}
	var hashes []string
	for index := 0; index < len(queue); index++ {
		fn := queue[index]
		if seen[fn.decl] {
			continue
		}
		seen[fn.decl] = true
		hash, err := fingerprint(token.NewFileSet(), fn.decl)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, fn.file+":"+fn.name()+":"+hash)
		queue = append(queue, proofReferences(fn.decl.Body, byName)...)
	}
	return hashes, nil
}

func proofReferences(body *ast.BlockStmt, byName map[string][]function) []function {
	if body == nil {
		return nil
	}
	var references []function
	for node := range ast.Preorder(body) {
		references = appendNodeReferences(references, node, byName)
	}
	return references
}

func appendNodeReferences(refs []function, node ast.Node, byName map[string][]function) []function {
	switch expr := node.(type) {
	case *ast.SelectorExpr:
		return appendReferences(refs, byName[expr.Sel.Name], methodReference)
	case *ast.Ident:
		if expr.Obj == nil || expr.Obj.Kind == ast.Fun {
			return appendReferences(refs, byName[expr.Name], functionReference)
		}
	}
	return refs
}

type referenceKind int

const (
	methodReference referenceKind = iota
	functionReference
)

func appendReferences(refs, candidates []function, kind referenceKind) []function {
	for _, fn := range candidates {
		if (fn.decl.Recv != nil) == (kind == methodReference) {
			refs = append(refs, fn)
		}
	}
	return refs
}
