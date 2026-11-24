package lawsdoc

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pantacor/lawgate/inventory"
)

// TestFunc is a test function the build would compile.
type TestFunc struct {
	Name    string
	Package string // Go import path, including for external tests
	File    string // path of its file
	Line    int
	// text is the test's doc comment and the string literals of its body:
	// where a test names the laws it enforces.
	text string
}

// NamesLaw reports whether the test names law in its doc comment or in a
// string literal of its body, as a whole kebab-case word.
func (f TestFunc) NamesLaw(law string) bool {
	return containsWord(f.text, law)
}

// Tests are the test functions of a module, by name. One name may exist in
// several packages.
type Tests map[string][]TestFunc

// NewTestFunc builds a TestFunc from text, for test inventories that do not
// come from Go (another language's test runner).
func NewTestFunc(name, file string, line int, text string) TestFunc {
	return TestFunc{Name: name, File: file, Line: line, text: text}
}

// Add records a test.
func (t Tests) Add(test TestFunc) {
	t[test.Name] = append(t[test.Name], test)
}

// QualifiedName is the test identity the Go event stream reports.
func (f TestFunc) QualifiedName() string {
	if f.Package == "" {
		return f.Name
	}
	return f.Package + "/" + f.Name
}

// EnforcingTest finds the unique compiled test with the given reference.
func (t Tests) EnforcingTest(law, reference string) (TestFunc, error) {
	test, err := t.Resolve(reference)
	if err != nil {
		return TestFunc{}, err
	}
	if !test.NamesLaw(law) {
		return TestFunc{}, fmt.Errorf("is enforced by test %q, which does not name the law %s in its doc comment or body", reference, law)
	}
	return test, nil
}

// Resolve accepts the import path plus test name, or an unambiguous bare name.
func (t Tests) Resolve(reference string) (TestFunc, error) {
	var matches []TestFunc
	separator := strings.LastIndexAny(reference, "/.")
	name := reference[separator+1:]
	for _, test := range t[name] {
		qualified := reference == test.QualifiedName() || reference == test.Package+"."+test.Name
		if separator < 0 || qualified {
			matches = append(matches, test)
		}
	}
	switch len(matches) {
	case 0:
		return TestFunc{}, fmt.Errorf("is enforced by test %q, which is not a test function the build compiles in this module", reference)
	case 1:
		return matches[0], nil
	}
	names := make([]string, 0, len(matches))
	for _, test := range matches {
		names = append(names, test.QualifiedName())
	}
	return TestFunc{}, fmt.Errorf("is enforced by ambiguous test %q; qualify it as one of: %s", reference, strings.Join(names, ", "))
}

// ResolveTestRef resolves a test in the active build of the Go module at root.
func ResolveTestRef(root, ref string) (file string, ok bool) {
	tests, err := DiscoverTests(root)
	if err != nil {
		return "", false
	}
	test, err := tests.Resolve(ref)
	return test.File, err == nil
}

// DiscoverTests asks the active Go build which test files it selects.
func DiscoverTests(root string, buildFlags ...string) (Tests, error) {
	command := append([]string{"go", "test"}, buildFlags...)
	return DiscoverCommandTests(root, command)
}

// DiscoverCommandTests selects the test files of a Go test command, including
// the environment of a direct invocation through env. An optional timeout
// bounds the build query, as for every other module command.
func DiscoverCommandTests(root string, command []string, timeout ...time.Duration) (Tests, error) {
	packages, err := inventory.GoPackages(root, command, timeout...)
	if err != nil {
		return nil, err
	}
	tests := Tests{}
	for _, pkg := range packages {
		if err := addPackageTests(tests, pkg.ImportPath, pkg.Tests()); err != nil {
			return nil, err
		}
	}
	return tests, nil
}

func addPackageTests(tests Tests, pkg string, files []string) error {
	for _, name := range files {
		if err := addFileTests(tests, pkg, name); err != nil {
			return err
		}
	}
	return nil
}

func addFileTests(tests Tests, pkg, path string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && isTestFunc(fn) {
			tests.Add(TestFunc{Name: fn.Name.Name, Package: pkg, File: path, Line: fset.Position(fn.Pos()).Line, text: testText(fn)})
		}
	}
	return nil
}

// isTestFunc follows the go tool: TestXxx (Xxx not starting with a lower
// case letter), no receiver, one parameter.
func isTestFunc(fn *ast.FuncDecl) bool {
	rest, ok := strings.CutPrefix(fn.Name.Name, "Test")
	plainOneParam := fn.Recv == nil && fn.Type.Params.NumFields() == 1
	if !ok || !plainOneParam {
		return false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(first)
}

func isLowerASCII(c byte) bool { return c >= 'a' && c <= 'z' }

// testText is the test's doc comment and its body's string literals.
func testText(fn *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString(fn.Doc.Text())
	if fn.Body == nil {
		return b.String()
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if text, err := strconv.Unquote(lit.Value); err == nil {
			b.WriteString("\n" + text)
		}
		return true
	})
	return b.String()
}

// containsWord reports whether word occurs in text with no kebab-case
// character on either side.
func containsWord(text, word string) bool {
	for start := 0; ; {
		index := strings.Index(text[start:], word)
		if index < 0 {
			return false
		}
		at := start + index
		if isWordAt(text, at, len(word)) {
			return true
		}
		start = at + 1
	}
}

func isWordAt(text string, at, length int) bool {
	before := at == 0 || !isKebab(text[at-1])
	end := at + length
	after := end == len(text) || !isKebab(text[end])
	return before && after
}

func isKebab(c byte) bool {
	return isLowerASCII(c) || strings.IndexByte("0123456789-", c) >= 0
}
