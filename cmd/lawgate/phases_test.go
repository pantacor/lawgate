package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

func TestCheckUsesMachineReportAndSharedDocumentation(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withMachines)
	module.MachineCommand = shellReport(`<testsuites><testsuite name="worker"><testcase name="local-law"/><testcase name="shared-law"/></testsuite></testsuites>`)
	module.MachineBuild = &inventory.GoMachineBuild{}
	modules := []inventory.Module{module}
	config := inventory.Config{Product: "Product & team", Modules: modules}
	writeGateConfig(t, root, config)
	writeGoLawModule(t, root, module)
	writeFixture(t, root, module.Laws, testLaw+machineLaw("local-law", "worker"))
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("the gate accepted an undocumented machine law")
	}
	assertReport(t, directory, "laws-summary.md", "law shared-law (machine worker)")
	assertReport(t, directory, "laws-report.html", "<title>Laws · Product &amp; team</title>")
	assertReport(t, directory, "001.xml", "local-law", "<failure")

	config.Modules[0].SharedLaws = []string{"lib/LAWS.md"}
	writeGateConfig(t, root, config)
	assertSharedDocProblem(t, root, "# Library laws\n\n## shared-law\n", "Source:", "Enforced by:", "Status:")
	duplicate := machineLaw("shared-law", "worker") + machineLaw("shared-law", "worker")
	assertSharedDocProblem(t, root, duplicate, "more than one section")
	assertSharedDocProblem(t, root, machineLaw("local-law", "worker"), "sections in both")
	assertSharedDocProblem(t, root, machineLaw("shared-law", "ghost"), "ghost", "not a registered machine")
	writeFixture(t, root, "lib/LAWS.md", machineLaw("shared-law", "*"))
	directory, err = runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, directory, "001.xml", "shared-law", `failures="0"`)
	assertReport(t, directory, "laws-summary.md", "1 machine, 2 machine laws,")
}

func assertSharedDocProblem(t *testing.T, root, doc string, details ...string) {
	t.Helper()
	writeFixture(t, root, "lib/LAWS.md", doc)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted an invalid shared law document")
	}
	assertReport(t, directory, "001.xml", details...)
}

func TestCheckRequiresGoEnforcerToNameItsLaw(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeConfig(t, root, []inventory.Module{module})
	writeGoLawModule(t, root, module)
	writeFixture(t, root, "law_test.go", "package fixture\nimport (\n\t\"os\"\n\t\"testing\"\n)\nfunc TestLaw(test *testing.T) {\n\tif os.Getenv(\"LAWGATE_BITE\") != \"\" {\n\t\ttest.Fatal(\"law switched off\")\n\t}\n}\n")
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("the gate accepted a test that never named its law")
	}
	assertReport(t, directory, "001.xml", "<failure", "does not name the law report-required")
}

func TestNodeDocsCheckFieldsAndExecutedTests(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language = "node"
	module.TestCommand = []string{"sh", "-c", "printf '%s\\n' 'TAP version 13' 'ok 1 - active' '1..1'"}
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, module.Laws, "## checking-output\n- Enforced by: test `missing`\n- Status: agreed 2026-11-20\n")
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("the gate accepted incomplete Node law documentation")
	}
	assertReport(t, directory, "001.xml", "<failure", "did not run to completion", "Source:", `errors="0"`)
	writeFixture(t, root, module.Laws, machineLaw("external-check", "external"))
	directory, err = runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted an unverified external machine")
	}
	assertReport(t, directory, "001.xml", "not a registered machine")
}

func TestCheckReadabilityRatchetAndRefresh(t *testing.T) {
	root := t.TempDir()
	settings := &inventory.Readability{Baseline: "rules/readability.txt"}
	module := fixtureModule("worker", withoutMachines)
	modules := []inventory.Module{module}
	config := inventory.Config{Modules: modules, Readability: settings}
	writeGateConfig(t, root, config)
	writeGoLawModule(t, root, module)
	writeFixture(t, root, settings.Baseline, "")
	writeFixture(t, root, "worker/bad.go", badGo)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("the readability ratchet accepted a new offending file")
	}
	assertReport(t, directory, "laws-summary.md", "worker/bad.go: 1 findings (new file, baseline 0)")

	if err := run([]string{"readability-baseline", "--root", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	assertReport(t, root, settings.Baseline, "1 worker/bad.go")
	writeFixture(t, root, "worker/bad.go", badGo+"func another(flag bool) {}\n")
	directory, err = runCheck(t, root)
	if err == nil {
		t.Fatal("the readability ratchet accepted growth")
	}
	assertReport(t, directory, "laws-summary.md", "grew from baseline 1")

	writeFixture(t, root, "worker/bad.go", "package fixture\n")
	directory, err = runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, directory, "laws-summary.md", "### Notes", "worker/bad.go: baseline can shrink to 0")
}

func TestReadabilitySelectionUsesRepositoryGlobs(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule("worker", withoutMachines)
	settings := &inventory.Readability{Baseline: "baseline.txt"}
	settings.Include = []string{"worker/**/keep*.go"}
	settings.Exclude = []string{"worker/**/keep-generated.go"}
	modules := []inventory.Module{module}
	config := inventory.Config{Modules: modules, Readability: settings}
	writeGateConfig(t, root, config)
	writeGoLawModule(t, root, module)
	for _, name := range []string{"keep.go", "nested/keep.go", "keep-generated.go", "ignore.go"} {
		writeFixture(t, root, "worker/"+name, badGo)
	}
	if err := run([]string{"readability-baseline", "--root", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, settings.Baseline))
	want := "1 worker/keep.go\n1 worker/nested/keep.go\n"
	if err != nil || string(data) != want {
		t.Fatalf("baseline = %q, error = %v, want %q", data, err, want)
	}
}

func TestReadabilityCommandsRefuseSymlinkedBaselineWithoutOwner(t *testing.T) {
	file := baselineLink{name: "baseline.txt", target: "actual/baseline.txt", baseline: "baseline.txt"}
	directory := baselineLink{name: "rules", target: "actual", baseline: "rules/baseline.txt"}
	t.Run("file", func(t *testing.T) { assertReadabilityBaselineLinkRefused(t, file) })
	t.Run("parent", func(t *testing.T) { assertReadabilityBaselineLinkRefused(t, directory) })
}

type baselineLink struct {
	name, target, baseline string
}

func assertReadabilityBaselineLinkRefused(t *testing.T, link baselineLink) {
	t.Helper()
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language, module.TestCommand = "node", nodeLawCommand()
	settings := &inventory.Readability{Baseline: link.baseline}
	modules := []inventory.Module{module}
	writeGateConfig(t, root, inventory.Config{Modules: modules, Readability: settings})
	writeFixture(t, root, "LAWS.md", testLaw)
	writeFixture(t, root, "actual/baseline.txt", "# owner baseline\n")
	if err := os.Symlink(link.target, filepath.Join(root, link.name)); err != nil {
		t.Fatal(err)
	}
	directory, err := runCheck(t, root)
	if err == nil {
		t.Error("check accepted a symlinked readability baseline")
	}
	assertReport(t, directory, "laws-summary.md", "readability", "symbolic link")
	err = run([]string{"readability-baseline", "--root", root}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("baseline refresh got %v, want symbolic link error", err)
	}
	assertReport(t, root, "actual/baseline.txt", "# owner baseline")
}

// TestCheckFailsUncoveredRuleFile enforces rule-changes-are-reviewed: a
// rule file the required owner does not review fails the gate.
func TestCheckFailsUncoveredRuleFile(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	settings := &inventory.Readability{Baseline: "baseline.txt"}
	modules := []inventory.Module{module}
	config := inventory.Config{Modules: modules, Owner: "@owner", Platform: "gitlab", Readability: settings}
	writeGateConfig(t, root, config)
	writeGoLawModule(t, root, module)
	writeFixture(t, root, settings.Baseline, "")
	useGitFileList(t, root)
	writeFixture(t, root, "CODEOWNERS", "* @owner\n/baseline.txt @another\n")
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("the gate accepted an uncovered baseline")
	}
	assertReport(t, directory, "laws-summary.md", "CODEOWNERS coverage", "does not require @owner for: baseline.txt")
	writeFixture(t, root, "CODEOWNERS", "* @owner\n")
	if _, err := runCheck(t, root); err != nil {
		t.Fatal(err)
	}
}

func useGitFileList(t *testing.T, root string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\0' CODEOWNERS LAWS.md go.mod law_test.go\n"
	writeFixture(t, root, "tools/git", script)
	bin := filepath.Join(root, "tools")
	if err := os.Chmod(filepath.Join(bin, "git"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeGoLawModule(t *testing.T, root string, module inventory.Module) {
	t.Helper()
	writeFixture(t, root, filepath.Join(module.Path, "go.mod"), "module example.test/fixture\n\ngo 1.25\n")
	writeFixture(t, root, filepath.Join(module.Path, "law_test.go"), lawTestSource)
	writeFixture(t, root, module.Laws, testLaw)
}

const badGo = "package fixture\nfunc broken(flag bool) {}\n"

func TestNodeRejectsDuplicateLawsAndUnverifiedMachines(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language = "node"
	module.TestCommand = []string{"sh", "-c", "printf \"%s\\n\" \"TAP version 13\" \"ok 1 - TestLaw\" \"1..1\""}
	writeConfig(t, root, []inventory.Module{module})
	doc := testLaw + testLaw + machineLaw("lost-law", "absent")
	writeFixture(t, root, module.Laws, doc)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted duplicate Node laws and an absent machine")
	}
	assertReport(t, directory, "001.xml", "has more than one section", "not a registered machine")
}

func TestCheckScriptExitIsLawVerdict(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeConfig(t, root, []inventory.Module{module})
	writeGoLawModule(t, root, module)
	doc := "## script-law\n- Source: owner decision 2026-11-20\n- Enforced by: script `check.sh`\n- Status: agreed 2026-11-20\n"
	writeFixture(t, root, module.Laws, testLaw+doc)
	writeFixture(t, root, "check.sh", "#!/bin/sh\nexit 9\n")
	if err := os.Chmod(filepath.Join(root, "check.sh"), 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted a failing script")
	}
	assertReport(t, directory, "001.xml", "script-law", "exit status 9", "<failure")
}

func TestCheckRejectsAmbiguousGoLawTest(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.TestCommand = []string{"go", "test", "-json", "-count=1", "./..."}
	writeConfig(t, root, []inventory.Module{module})
	writeGoLawModule(t, root, module)
	body := "package fixture\nimport \"testing\"\n// TestLaw enforces report-required.\nfunc TestLaw(t *testing.T) { t.Skip(\"fixture\") }\n"
	writeFixture(t, root, "law_test.go", body)
	writeFixture(t, root, "other/law_test.go", lawTestSource)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted an ambiguous bare name satisfied by another package")
	}
	assertReport(t, directory, "001.xml", "ambiguous", "example.test/fixture/other/TestLaw")
	rootTest := strings.ReplaceAll(testLaw, "TestLaw", "example.test/fixture/TestLaw")
	writeFixture(t, root, module.Laws, rootTest)
	directory, err = runCheck(t, root)
	if err == nil {
		t.Fatal("test execution in another package satisfied the qualified skipped test")
	}
	assertReport(t, directory, "001.xml", "example.test/fixture/TestLaw", "did not run to completion")
	other := strings.ReplaceAll(testLaw, "TestLaw", "example.test/fixture/other/TestLaw")
	writeFixture(t, root, module.Laws, other)
	if _, err := runCheck(t, root); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRejectsUninventoriedMachineModule(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeConfig(t, root, []inventory.Module{module})
	writeGoLawModule(t, root, module)
	writeMachineModuleAt(t, root, "worker", "example.test/worker")
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted an unlisted machine module")
	}
	assertReport(t, directory, "laws-summary.md", "worker", "not listed")
}

func TestCheckRejectsDisabledMachineModule(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeConfig(t, root, []inventory.Module{module})
	writeGoLawModule(t, root, module)
	writeMachineModule(t, root)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted a machine module with machines disabled")
	}
	assertReport(t, directory, "laws-summary.md", "machines: true")
}

const workerMachineSource = "package worker\nimport \"github.com/pantacor/lawgate/machine\"\nvar Worker = machine.Machine{Name: \"worker\"}\n"

func TestCheckRejectsMissingDeclaredMachine(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withMachines)
	module.MachineCommand = shellReport("<testsuite name=\"visible\"><testcase name=\"machine-law\"/></testsuite>")
	module.MachineBuild = &inventory.GoMachineBuild{}
	writeConfig(t, root, []inventory.Module{module})
	writeGoLawModule(t, root, module)
	writeFixture(t, root, module.Laws, testLaw+machineLaw("machine-law", "visible"))
	writeMachineModule(t, root)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted an omitted declared machine")
	}
	assertReport(t, directory, "001.xml", "worker", "missing from runner verdicts")
}

func writeMachineModule(t *testing.T, root string) {
	t.Helper()
	writeMachineModuleAt(t, root, ".", "example.test/fixture")
}

func writeMachineModuleAt(t *testing.T, root, dir, modulePath string) {
	t.Helper()
	root = filepath.Join(root, dir)
	manifest := "module " + modulePath + "\n\ngo 1.25\nrequire github.com/pantacor/lawgate v0.0.0\n"
	manifest += "replace github.com/pantacor/lawgate => ./testdata/framework\n"
	writeFixture(t, root, "go.mod", manifest)
	writeFixture(t, root, "testdata/framework/go.mod", "module github.com/pantacor/lawgate\n\ngo 1.25\n")
	writeFixture(t, root, "testdata/framework/machine/machine.go", "package machine\ntype Machine struct{ Name string }\n")
	writeFixture(t, root, "worker/machine.go", workerMachineSource)
}

func TestNodeRequiresEveryDocumentedMachineLaw(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withMachines)
	module.Language = "node"
	module.TestCommand = nodeLawCommand()
	module.MachineCommand = shellReport("<testsuite name=\"worker\"><testcase name=\"seen-law\"/></testsuite>")
	writeConfig(t, root, []inventory.Module{module})
	doc := testLaw + machineLaw("seen-law", "worker") + machineLaw("missing-law", "worker")
	writeFixture(t, root, module.Laws, doc)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted a partial machine report for a Node module")
	}
	assertReport(t, directory, "001.xml", "missing-law", "has no law of that name")
}

func TestNodeScriptResolvesFromDocumentButRunsFromRoot(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule("worker", withoutMachines)
	module.Language = "node"
	module.TestCommand = nodeLawCommand()
	writeConfig(t, root, []inventory.Module{module})
	doc := "## script-law\n- Source: owner decision 2026-11-20\n- Enforced by: script `check.sh`\n- Status: agreed 2026-11-20\n"
	writeFixture(t, root, module.Laws, testLaw+doc)
	writeFixture(t, root, "worker/check.sh", "#!/bin/sh\ntest -f lawgate.json && touch from-script || exit 2\n"+rejectScriptLaw("script-law"))
	if err := os.Chmod(filepath.Join(root, "worker/check.sh"), 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "from-script")); err != nil {
		t.Fatal("script did not run in the repository root")
	}
	assertReport(t, directory, "001.xml", "script-law (script check.sh)", `failures="0"`)
}

func nodeLawCommand() []string {
	return []string{"sh", "-c", "printf \"%s\\n\" \"TAP version 13\" \"ok 1 - TestLaw\" \"1..1\""}
}

func TestCheckGoTestUsesActiveBuildTags(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.TestCommand = []string{"go", "test", "-tags", "lawgate_fixture", "-json", "-count=1", "./..."}
	writeConfig(t, root, []inventory.Module{module})
	writeGoLawModule(t, root, module)
	source := "//go:build lawgate_fixture\n\n" + lawTestSource
	writeFixture(t, root, "law_test.go", source)
	if _, err := runCheck(t, root); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRejectsMissingSharedMachineLaw(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withMachines)
	module.Language = "node"
	module.TestCommand = nodeLawCommand()
	module.MachineCommand = shellReport("<testsuite name=\"worker\"><testcase name=\"local-law\"/></testsuite>")
	module.SharedLaws = []string{"lib/LAWS.md"}
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, module.Laws, testLaw+machineLaw("local-law", "worker"))
	writeFixture(t, root, "lib/LAWS.md", machineLaw("shared-law", "worker"))
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted a machine that omitted a shared law")
	}
	assertReport(t, directory, "001.xml", "worker", "shared-law")
}

func TestScriptStartFailureIsInfrastructure(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "script.sh", "#!/bin/sh\nexit 0\n")
	result := moduleResult{}
	enforcer := lawsdoc.Enforcer{Kind: lawsdoc.Script, Name: "script.sh"}
	runScript(root, "LAWS.md", "script-law", enforcer, &result)
	if got := result.Verdicts[0].Status; got != infrastructure {
		t.Fatalf("script verdict = %q", got)
	}
}

func TestGeneratedSourceRequiresFreshnessCommand(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeGoLawModule(t, root, module)
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, "worker_gen.go", "// Code generated by fixture. DO NOT EDIT.\npackage fixture\n")
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted a generated file without generate_check")
	}
	assertReport(t, directory, "001.xml", "generated-is-fresh", "generate_check", "worker_gen.go")
}

func TestGeneratedSourceCommandControlsFreshnessVerdict(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule("worker", withoutMachines)
	writeGoLawModule(t, root, module)
	writeFreshnessFixture(t, root, "worker")
	module.GenerateCheck = []string{"sh", "check.sh"}
	writeConfig(t, root, []inventory.Module{module})
	directory, err := runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, directory, "001.xml", "generated-is-fresh (worker/states_gen.go: fresh)", `failures="0"`)
	writeFixture(t, root, "worker/states_gen.go", generatedFixture+"// manual edit\n")
	directory, err = runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted stale source")
	}
	assertReport(t, directory, "001.xml", "<failure", "generated-is-fresh (worker/states_gen.go: stale)")
	if err := os.Remove(filepath.Join(root, "worker/states_gen.go")); err != nil {
		t.Fatal(err)
	}
	directory, err = runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted missing source")
	}
	assertReport(t, directory, "001.xml", "<failure", "generated-is-fresh (worker/states_gen.go: missing)")
}

func TestSharedScriptUsesItsOwnDocumentPath(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language = "node"
	module.TestCommand = nodeLawCommand()
	module.SharedLaws = []string{"shared/LAWS.md"}
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, module.Laws, testLaw)
	writeFixture(t, root, "shared/LAWS.md", "## shared-law\n- Source: `../LAWS.md`\n- Enforced by: script `check.sh`\n- Status: agreed 2026-11-20\n")
	writeFixture(t, root, "shared/check.sh", "#!/bin/sh\ntest -f lawgate.json || exit 2\n"+rejectScriptLaw("shared-law"))
	if err := os.Chmod(filepath.Join(root, "shared/check.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	directory, err := runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, directory, "001.xml", "shared-law (script check.sh)", `failures="0"`)
}

func TestUnlistedGeneratedModuleCannotUseParentCheck(t *testing.T) {
	root := t.TempDir()
	parent := fixtureModule(".", withoutMachines)
	parent.GenerateCheck = []string{"sh", "check.sh"}
	writeGoLawModule(t, root, parent)
	writeFreshnessFixture(t, root, ".")
	writeConfig(t, root, []inventory.Module{parent})
	writeFixture(t, root, "nested/go.mod", "module example.test/nested\n")
	writeFixture(t, root, "nested/states_gen.go", "// Code generated by fixture. DO NOT EDIT.\npackage nested\n")
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted generated source in an unlisted module")
	}
	assertReport(t, directory, "laws-summary.md", "generated-is-fresh", "nested/states_gen.go", "no configured module")
}
