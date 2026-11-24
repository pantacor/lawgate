package lawsdoc

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/pantacor/lawgate/gosource"
)

func TestSourceRejectsSelfCitationsAndGoManifests(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"rules.md", "other.md", "go.mod"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	in := Inputs{RepoRoot: root, DocPath: "rules.md"}
	sources := []string{"rules.md", "see `./rules.md#scope`", "go.mod", "`go.mod` (agreed 2026-11-20)"}
	for _, source := range sources {
		if err := checkSource(source, in); err == nil {
			t.Errorf("accepted Source %q", source)
		}
	}
	if err := checkSource("other.md", in); err != nil {
		t.Fatal(err)
	}
}

func TestBiteGapRequiresAnExplicitReason(t *testing.T) {
	doc := section("first-law", "machine `worker`", agreed)
	requireProblem(t, doc+"- Bites: not yet.\n", machineInputs(), "must say why")
	requireProblem(t, doc+"- Bites: passing\n", machineInputs(), "must be")
	requireProblem(t, doc+"- Bites: not yet. First.\n- Bites: not yet. Second.\n", machineInputs(), "must be")
}

func TestAgreedStatusRequiresACalendarDate(t *testing.T) {
	for _, date := range []string{"2026-13-45", "2026-02-29", "2026-04-31"} {
		doc := section("first-law", "machine `worker`", "agreed "+date)
		requireProblem(t, doc, machineInputs(), "valid calendar date")
	}
	doc := section("first-law", "machine `worker`", "agreed 2024-02-29")
	if got := checkText(doc, machineInputs()); got != "" {
		t.Fatalf("leap day: %s", got)
	}
}

func TestDatedSourcesRequireACalendarDate(t *testing.T) {
	in := Inputs{RepoRoot: fixtureRoot, DocPath: "nested/LAWS.md"}
	sources := []string{
		"owner decision 2026-02-29",
		"`../LAWS.md` (agreed 2026-13-45)",
		"https://example.test/plan; owner decision 2026-02-29",
		"owner decision 2026-11-20; owner decision 2026-02-29",
	}
	for _, source := range sources {
		if err := checkSource(source, in); err == nil {
			t.Errorf("accepted Source %q", source)
		}
	}
}

func TestBiteGapRequiresTestOrScript(t *testing.T) {
	doc := section("first-law", "machine `worker`", agreed)
	requireProblem(t, doc+"- Bites: not yet. no mutation yet\n", machineInputs(), "no test or script")
}

func TestGapFieldsUseSameSpelling(t *testing.T) {
	input := machineInputs()
	for _, value := range []string{"not yet: later", "not yet, later", "not yet.later"} {
		requireProblem(t, section("first-law", value, agreed), input, "not yet. <why>")
		doc := section("first-law", "script `rule.sh`", agreed)
		requireProblem(t, doc+"- Bites: "+value+"\n", input, "not yet. <why>")
	}
}

func TestGoBiteHooksFindsCallsInModuleCode(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"hook.go":        biteHooksSource,
		"hook_test.go":   "package sample\n" + biteHookImport + "\nvar _ = witness.Off(\"test-only\")",
		"nested/go.mod":  "module example.test/nested\n\ngo 1.25\n",
		"nested/hook.go": "package nested\n" + biteHookImport + "\nvar _ = witness.Off(\"nested-module\")",
		"unrelated.go":   "package sample\nvar _ = bite.Off(\"unrelated\")",
		"_ignored.go":    "package sample\n" + biteHookImport + "\nvar _ = witness.Off(\"ignored\")",
	}
	for name, source := range files {
		writeBiteHookSource(t, root, name, source)
	}
	path := filepath.Join(root, "hook_test.go")
	test := TestFunc{Package: "example.test/sample", File: path}
	pkg := gosource.Package{Dir: root, ImportPath: "example.test/sample [example.test/sample.test]", ForTest: test.Package}
	pkg.GoFiles = []string{"hook.go", "hook_test.go", "unrelated.go"}
	hooks, err := GoBiteHooks(root, upstreamBite, test, []gosource.Package{pkg})
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 || !hooks["valid"] {
		t.Fatalf("got hooks %v, want only valid", hooks)
	}
}

const upstreamBite = "github.com/pantacor/lawgate/bite"

const biteHookImport = "import witness \"" + upstreamBite + "\""

const biteHooksSource = `package sample
` + biteHookImport + `
var _ = witness.Off("valid")
func shadowed(witness interface{ Off(string) bool }) bool {
	return witness.Off("shadowed")
}
func redefined() bool {
	witness := struct{ Off func(string) bool }{}
	return witness.Off("redefined")
}
`

// A fork, a vanity path or a later major version imports its own bite
// package: only calls through that import are hooks.
func TestGoBiteHooksUseTheGivenBitePackage(t *testing.T) {
	root := t.TempDir()
	writeBiteHookSource(t, root, "fork.go", "package sample\nimport \"example.test/fork/v2/bite\"\nvar _ = bite.Off(\"fork-law\")")
	writeBiteHookSource(t, root, "upstream.go", "package sample\nimport \""+upstreamBite+"\"\nvar _ = bite.Off(\"upstream-law\")")
	path := filepath.Join(root, "hook_test.go")
	test := TestFunc{Package: "example.test/sample", File: path}
	pkg := gosource.Package{Dir: root, ImportPath: "example.test/sample [example.test/sample.test]", ForTest: test.Package}
	pkg.GoFiles = []string{"fork.go", "upstream.go", "hook_test.go"}
	hooks, err := GoBiteHooks(root, "example.test/fork/v2/bite", test, []gosource.Package{pkg})
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 || !hooks["fork-law"] {
		t.Fatalf("got hooks %v, want only fork-law", hooks)
	}
}

// The closure follows the test variant's imports, through the import map
// that renames recompiled and vendored packages, and stops at the standard
// library and at packages the test does not import.
func TestGoBiteHooksFollowTheTestsImports(t *testing.T) {
	root := t.TempDir()
	hook := "package %s\nimport \"" + upstreamBite + "\"\nvar _ = bite.Off(%q)"
	for _, name := range []string{"direct", "mapped", "deep", "unreached", "std"} {
		writeBiteHookSource(t, root, name+"/hook.go", fmt.Sprintf(hook, name, name+"-law"))
	}
	path := filepath.Join(root, "law_test.go")
	test := TestFunc{Package: "example.test/sample", File: path}
	variant := gosource.Package{Dir: root, ImportPath: "example.test/sample [example.test/sample.test]", ForTest: test.Package}
	variant.GoFiles = []string{"law_test.go"}
	variant.Imports = []string{"example.test/direct", "example.test/mapped", "example.test/std"}
	variant.ImportMap = map[string]string{"example.test/mapped": "example.test/vendor/mapped"}
	other := gosource.Package{Dir: root, ImportPath: "example.test/other [example.test/other.test]", ForTest: "example.test/other"}
	other.Imports = []string{"example.test/unreached"}
	packages := []gosource.Package{variant, other,
		closurePackage(root, "direct", "example.test/direct", "example.test/deep"),
		closurePackage(root, "mapped", "example.test/vendor/mapped"),
		closurePackage(root, "deep", "example.test/deep", "example.test/direct"),
		closurePackage(root, "unreached", "example.test/unreached"),
		standardPackage(root, "std", "example.test/std"),
	}
	hooks, err := GoBiteHooks(root, upstreamBite, test, packages)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"direct-law": true, "mapped-law": true, "deep-law": true}
	if !maps.Equal(hooks, want) {
		t.Fatalf("got hooks %v, want %v", hooks, want)
	}
}

func closurePackage(root, dir, importPath string, imports ...string) gosource.Package {
	pkg := gosource.Package{Dir: filepath.Join(root, dir), ImportPath: importPath, Imports: imports}
	pkg.GoFiles = []string{"hook.go"}
	return pkg
}

func standardPackage(root, dir, importPath string) gosource.Package {
	pkg := closurePackage(root, dir, importPath)
	pkg.Standard = true
	return pkg
}

func writeBiteHookSource(t *testing.T, root, name, source string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}
