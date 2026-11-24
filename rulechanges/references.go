package rulechanges

import (
	"go/ast"
	"path"
	"strings"

	"github.com/pantacor/lawgate/lawsdoc"
)

// testKey names a Go test function by its file and name.
type testKey struct {
	file string
	name string
}

// enforcingTests is every Go test function a law document or required_tests
// names, in that reference's scope. A reference is resolved by lawsdoc's own
// resolver, so a qualified "import/path/TestName" means here exactly what
// the check means by it.
func (sources *goSources) enforcingTests(enforcers *enforcers) map[testKey]bool {
	tests := sources.testInventory()
	enforcing := make(map[testKey]bool)
	for reference := range enforcers.allNames() {
		enforcers.markInScope(enforcing, reference, resolveReference(tests, reference))
	}
	return enforcing
}

// markInScope marks the tests a reference resolved to that its document or
// module governs.
func (enforcers *enforcers) markInScope(enforcing map[testKey]bool, reference string, tests []lawsdoc.TestFunc) {
	for _, test := range tests {
		if enforcers.namesFor(test.File)[reference] {
			enforcing[testKey{test.File, test.Name}] = true
		}
	}
}

// resolveReference is every test a reference may name in some build. A
// bare name the whole repository holds more than once resolves to every
// holder: the check refuses it in a module where it is ambiguous, and the
// scope filter in enforcingTests keeps the holders the document governs. A
// qualified name held by several files (mutually exclusive build variants
// of one package) resolves to each of them: execution needs the one active
// test, ownership protects every variant.
func resolveReference(tests lawsdoc.Tests, reference string) []lawsdoc.TestFunc {
	test, err := tests.Resolve(reference)
	if err == nil {
		return []lawsdoc.TestFunc{test}
	}
	var matches []lawsdoc.TestFunc
	for _, candidates := range tests {
		matches = append(matches, namedBy(candidates, reference)...)
	}
	return matches
}

// namedBy are the candidates lawsdoc's resolver accepts for reference when
// each is the only test it is shown, so the qualified forms mean here
// exactly what the check means by them.
func namedBy(candidates []lawsdoc.TestFunc, reference string) []lawsdoc.TestFunc {
	var matches []lawsdoc.TestFunc
	for _, candidate := range candidates {
		alone := lawsdoc.Tests{}
		alone.Add(candidate)
		if _, err := alone.Resolve(reference); err == nil {
			matches = append(matches, candidate)
		}
	}
	return matches
}

// testInventory lists the Go test functions of every parsed _test.go file,
// with the import path of their directory, as lawsdoc.DiscoverTests would.
// It reads every file, whatever its build constraints: protecting a file
// the build skips costs a review, missing one would cost the gate.
func (sources *goSources) testInventory() lawsdoc.Tests {
	tests := lawsdoc.Tests{}
	for _, file := range sources.files {
		if !strings.HasSuffix(file.name, "_test.go") {
			continue
		}
		sources.addTests(tests, file)
	}
	return tests
}

func (sources *goSources) addTests(tests lawsdoc.Tests, file *goFile) {
	importPath := sources.importPath(path.Dir(file.name))
	for _, node := range file.syntax.Decls {
		name, ok := testFunctionName(node)
		if ok {
			tests.Add(lawsdoc.TestFunc{Name: name, Package: importPath, File: file.name})
		}
	}
}

// testFunctionName is the name of a plain function that may be a test. The
// check applies the go tool's exact rule; a looser one here only protects more.
func testFunctionName(node ast.Node) (string, bool) {
	fn, ok := node.(*ast.FuncDecl)
	if !ok || fn.Recv != nil {
		return "", false
	}
	return fn.Name.Name, strings.HasPrefix(fn.Name.Name, "Test")
}
