package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

func TestDocumentedGapsStayPendingInReports(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language = "node"
	module.TestCommand = []string{"sh", "-c", nodeTapFixture}
	writeConfig(t, root, []inventory.Module{module})
	doc := testLaw + "\n### missing-law\n- Source: owner decision 2026-11-20\n- Enforced by: not yet. Needs a fixture.\n- Status: agreed 2026-11-20\n"
	writeFixture(t, root, "LAWS.md", doc)
	directory, err := runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, directory, "laws-summary.md", "2 pending", "bite gap: The fixture", "Needs a fixture.")
	assertReport(t, directory, "laws-report.html", "2 pending", "bite gap: The fixture", "Needs a fixture.")
	assertReport(t, directory, "001.xml", `skipped="2"`, `message="pending: bite gap: The fixture`, `message="pending: Needs a fixture."`)
}

func TestBiteRejectsUnreachableOrInactiveGoHooks(t *testing.T) {
	for _, name := range []string{"unrelated/hook.go", "ignored.go"} {
		t.Run(name, func(t *testing.T) { assertUnreachableHook(t, name) })
	}
}

func assertUnreachableHook(t *testing.T, name string) {
	t.Helper()
	root := t.TempDir()
	writeBiteModuleManifest(t, root)
	writeFixture(t, root, "law_test.go", biteTestSource)
	source := biteHookSource
	if name == "ignored.go" {
		source = "//go:build ignore\n\n" + source
	}
	writeFixture(t, root, filepath.FromSlash(name), source)
	result := runGoLawBite(t, root)
	expectBite(t, result, "go-law", failed, "bite hook missing")
}

func TestBiteRejectsAnEnforcerThatFailsForAnySelection(t *testing.T) {
	root := t.TempDir()
	writeBiteGoModule(t, root, ".")
	source := `package sub
import ("os"; "testing")
func TestLaw(t *testing.T) {
	if os.Getenv("LAWGATE_BITE") != "" { t.Fatal("go-law") }
}`
	writeFixture(t, root, "law_test.go", source)
	result := runGoLawBite(t, root)
	expectBite(t, result, "go-law", failed, "control")
}

func TestBiteReportsExitWithoutTestVerdict(t *testing.T) {
	root := t.TempDir()
	writeBiteGoModule(t, root, ".")
	source := `package sub
import ("fmt"; "os"; "testing")
func TestMain(m *testing.M) {
	if !acceptsLaw() { fmt.Fprintln(os.Stderr, "mutation broke setup"); os.Exit(1) }
	os.Exit(m.Run())
}
func TestLaw(t *testing.T) { _ = "go-law" }
`
	writeFixture(t, root, "law_test.go", source)
	result := runGoLawBite(t, root)
	expectBite(t, result, "go-law", failed, "did not fail with LAWGATE_BITE=go-law: exit status 1")
	expectBite(t, result, "go-law", failed, "mutation broke setup")
	expectNoEventStream(t, result)
}

func TestAdopterBiteDoesNotDisableFrameworkLaw(t *testing.T) {
	root := t.TempDir()
	writeBiteModuleManifest(t, root)
	writeFixture(t, root, "hook.go", `package sub
import "github.com/pantacor/lawgate/bite"
func accidentalHook() bool { return bite.Off("laws-have-a-source") }
`)
	writeFixture(t, root, "law_test.go", `package sub
import ("testing"; "github.com/pantacor/lawgate/lawsdoc")
func TestLaw(t *testing.T) {
	doc := "### missing-law\n- Enforced by: not yet. needs a fixture\n- Status: agreed 2026-11-20\n"
	if len(lawsdoc.CheckPlan(lawsdoc.Plan(doc), lawsdoc.Inputs{})) == 0 { t.Fatal("laws-have-a-source") }
}
`)
	command := []string{"go", "test", "-json", "."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	result := runTestBite(root, module, biteLaw("laws-have-a-source", "test", "TestLaw"))
	expectBite(t, result, "laws-have-a-source", failed, "passed with")
}

func TestScriptBiteDoesNotRequireEnvOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	writeExecutable(t, root, "rule.sh", "#!/bin/sh\n"+rejectScriptLaw("script-law"))
	result := scriptBite(t, root, "rule.sh")
	expectBite(t, result, "script-law", passed, "")
}

func TestBiteFindsTaggedHookThroughTestImport(t *testing.T) {
	root := t.TempDir()
	writeBiteModuleManifest(t, root)
	writeFixture(t, root, "helper/plain.go", "//go:build !lawgate_bite\n\npackage helper\nfunc AcceptsLaw() bool { return true }\n")
	writeFixture(t, root, "helper/hook.go", taggedImportHook)
	writeFixture(t, root, "subject.go", "package sub\n")
	writeFixture(t, root, "law_test.go", `package sub_test
import ("testing"; "example.test/bites/helper")
func TestLaw(t *testing.T) {
	if !helper.AcceptsLaw() { t.Fatal("go-law") }
}
`)
	result := runGoLawBite(t, root)
	expectBite(t, result, "go-law", passed, "")
}

const taggedImportHook = `//go:build lawgate_bite

package helper
import "github.com/pantacor/lawgate/bite"
func AcceptsLaw() bool { return !bite.Off("go-law") }
`

func TestBiteDoesNotUseImportsFromAnotherTestPackage(t *testing.T) {
	root := t.TempDir()
	writeBiteModuleManifest(t, root)
	writeFixture(t, root, "subject.go", "package sub\n")
	writeFixture(t, root, "helper/hook.go", strings.Replace(taggedImportHook, "//go:build lawgate_bite\n\n", "", 1))
	writeFixture(t, root, "external_test.go", strings.Replace(biteTestSource, "package sub", "package sub_test", 1))
	writeFixture(t, root, "internal_test.go", "package sub\nimport _ \"example.test/bites/helper\"\n")
	result := runGoLawBite(t, root)
	expectBite(t, result, "go-law", failed, "bite hook missing")
}

func TestBiteRejectsAbnormalTestExit(t *testing.T) {
	assertAbnormalTestExit(t, "exit 126", "exit status 126")
	assertAbnormalTestExit(t, "exit 127", "exit status 127")
	assertAbnormalTestExit(t, "kill -TERM $$", "signal: terminated")
}

func TestGoBiteCommandMergesTagsAndRetainsBehaviorFlags(t *testing.T) {
	dir := t.TempDir()
	command := []string{"env", "GOWORK=off", "go", "test", "-C", "sub", "-tags", "hook", "-race",
		"-short", "-timeout", "1m", "-run", "Test.*", "-json", "-count=4", "./...", "-args", "-strict"}
	build, err := inventory.ParseGoCommand(dir, command)
	if err != nil {
		t.Fatal(err)
	}
	got := build.SelectTest("TestLaw", biteTag, []string{"example.test/bites/sub"})
	want := []string{"env", "GOWORK=off", "go", "test", "-C", filepath.Join(dir, "sub"), "-json", "-count=1",
		"-run", "^TestLaw$", "-race", "-short", "-timeout", "1m", "-tags=hook,lawgate_bite", "example.test/bites/sub", "-args", "-strict"}
	if !slices.Equal(got, want) {
		t.Fatalf("selected command = %q, want %q", got, want)
	}
}

func assertAbnormalTestExit(t *testing.T, exit, detail string) {
	t.Helper()
	root := t.TempDir()
	script := "if [ \"$LAWGATE_BITE\" = go-law ]; then " + exit + "; fi\n"
	script += "printf '%s\\n' 'TAP version 13' 'ok 1 - Example' '1..1'\n"
	command := []string{"sh", "-c", script}
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md", TestCommand: command}
	result := runTestBite(root, module, biteLaw("go-law", "test", "Example"))
	expectBite(t, result, "go-law", infrastructure, detail)
}

func runGoLawBite(t *testing.T, root string) moduleResult {
	t.Helper()
	command := []string{"go", "test", "-json", "-run", "^TestLaw$", "."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	return runTestBite(root, module, biteLaw("go-law", "test", "TestLaw"))
}

func runTestBite(root string, module inventory.Module, doc string) moduleResult {
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(doc)}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	return result
}
