package lawsdoc

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
)

var fixtureRoot = filepath.Join("testdata", "module")

func fixtureTests(t *testing.T) Tests {
	t.Helper()
	tests, err := DiscoverTests(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	return tests
}

// section renders one law section with the three fields.
func section(name, enforcedBy, status string) string {
	return "### " + name + "\n\nNo thing.\n\n- Source: owner decision 2026-11-20\n- Enforced by: " + enforcedBy + "\n- Status: " + status + "\n"
}

func checkText(doc string, in Inputs) string {
	return strings.Join(Check(doc, in), "\n")
}

func requireProblem(t *testing.T, doc string, in Inputs, want string) {
	t.Helper()
	if got := checkText(doc, in); !strings.Contains(got, want) {
		t.Errorf("want a problem with %q, got:\n%s", want, got)
	}
}

// Regression: test discovery was a regex over the
// text, so a test no build compiles counted as an enforcer.
func TestDiscoverTestsHonoursBuildConstraints(t *testing.T) {
	tests := fixtureTests(t)
	if len(tests["TestGhost"]) != 0 {
		t.Fatal("TestGhost is behind //go:build never and must not be listed")
	}
	if len(tests["Testlowercase"]) != 0 {
		t.Fatal("Testlowercase is not a test to the go tool")
	}
	if len(tests["TestNamesLaw"]) != 1 {
		t.Fatalf("TestNamesLaw must be listed once, got %v", tests["TestNamesLaw"])
	}
}

func TestCheckRefusesATestTheBuildExcludes(t *testing.T) {
	doc := section("first-law", "test `TestGhost`", "agreed 2026-11-20")
	requireProblem(t, doc, Inputs{Tests: fixtureTests(t)}, "not a test function the build compiles")
}

// Regression: any existing test name satisfied an enforcer, so a
// test could enforce a law it never mentions.
func TestCheckRefusesATestThatDoesNotNameItsLaw(t *testing.T) {
	doc := section("first-law", "test `TestNamesNothing`", "agreed 2026-11-20")
	requireProblem(t, doc, Inputs{Tests: fixtureTests(t)}, "does not name the law first-law")
}

func TestCheckModuleAcceptsTheFixture(t *testing.T) {
	root, err := filepath.Abs(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	problems, err := CheckModule(root, Inputs{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("the fixture's LAWS.md agrees with its code:\n%s", strings.Join(problems, "\n"))
	}
}

func TestCheckModuleReportsAMissingFile(t *testing.T) {
	problems, err := CheckModule(t.TempDir(), Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "does not exist") {
		t.Fatalf("got %v", problems)
	}
}

func TestNamesLawMatchesWholeWords(t *testing.T) {
	test := NewTestFunc("TestX", "x_test.go", 1, "checks no-stuck-job-ever and law:done")
	if test.NamesLaw("no-stuck-job") {
		t.Error("no-stuck-job is only part of no-stuck-job-ever")
	}
	if !test.NamesLaw("done") {
		t.Error("law:done names done")
	}
}

func TestTestTextHandlesBodylessFunction(t *testing.T) {
	fn := new(ast.FuncDecl)
	if text := testText(fn); text != "" {
		t.Fatalf("bodyless function text %q", text)
	}
}

func TestResolveTestRefAcceptsDottedImportPath(t *testing.T) {
	tests := fixtureTests(t)
	test := tests["TestNamesLaw"][0]
	resolved, ok := ResolveTestRef(fixtureRoot, test.Package+"."+test.Name)
	if !ok || resolved != test.File {
		t.Fatalf("resolved %q, %v; want %q", resolved, ok, test.File)
	}
}
