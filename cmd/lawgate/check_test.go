package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
)

// testLaw records its bite gap: fixture modules run a fake test command that
// prints go test -json, and a Go bite needs a direct go test command.
const testLaw = "## report-required\n\n- Source: owner decision 2026-11-20\n- Enforced by: test `TestLaw`\n- Bites: not yet. The fixture runs a fake test command, not go test.\n- Status: agreed 2026-11-20\n"

func TestCheckKeepsMissingAndCrashedModulesInAllReports(t *testing.T) {
	root := t.TempDir()
	missing := fixtureModule("missing", withoutMachines)
	crashed := fixtureModule("crashed", withMachines)
	crashed.MachineCommand = []string{"sh", "-c", "exit 7"}
	crashed.MachineBuild = &inventory.GoMachineBuild{}
	empty := fixtureModule("empty", withMachines)
	empty.MachineCommand = shellReport("<testsuites tests=\"0\"/>")
	empty.MachineBuild = &inventory.GoMachineBuild{}
	healthy := fixtureModule("healthy", withMachines)
	healthy.MachineCommand = shellReport("<testsuite name=\"worker\" tests=\"1\"><testcase name=\"must-pass\"/></testsuite>")
	healthy.MachineBuild = &inventory.GoMachineBuild{}
	modules := []inventory.Module{missing, crashed, empty, healthy}
	writeConfig(t, root, modules)
	for _, module := range modules[1:] {
		writeFixture(t, root, module.Path+"/go.mod", "module example.test/fixture\n\ngo 1.25\n")
		writeFixture(t, root, module.Laws, testLaw)
		writeFixture(t, root, module.Path+"/law_test.go", lawTestSource)
	}
	writeFixture(t, root, healthy.Laws, testLaw+machineLaw("must-pass", "worker"))
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check passed with missing or crashing modules")
	}
	for _, report := range []string{"laws-summary.md", "laws-report.html"} {
		assertReport(t, directory, report, "missing", "crashed", "empty", "healthy", "zero verdicts")
	}
	assertReport(t, directory, "001.xml", "missing", "<error")
	assertReport(t, directory, "002.xml", "crashed", "exit status 7")
	assertReport(t, directory, "003.xml", "empty", "zero verdicts")
	assertReport(t, directory, "004.xml", "must-pass", "errors=\"0\"")
}

func TestCheckRejectsGoSuccessWithoutNamedTest(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.TestCommand = []string{"go", "test", "-json", "-run", "^TestGhost$", "."}
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, "go.mod", "module example.test/noexit\n\ngo 1.25\n")
	writeFixture(t, root, "active_test.go", "package fixture\nimport \"testing\"\nfunc TestUnrelated(t *testing.T) {}\n")
	writeFixture(t, root, "excluded_test.go", "//go:build never_lawgate\n\npackage fixture\nimport \"testing\"\nfunc TestGhost(t *testing.T) {}\n")
	writeFixture(t, root, "LAWS.md", strings.ReplaceAll(testLaw, "TestLaw", "TestGhost"))
	stdout, stderr, err := execute(root, ".", module.TestCommand)
	if err != nil || !bytes.Contains(stdout, []byte("no tests to run")) {
		t.Fatalf("legacy command did not reproduce silent success: %s %s %v", stdout, stderr, err)
	}
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted Go's success without the named test")
	}
	assertReport(t, directory, "001.xml", "<error", "not a test function the build compiles")
}

func TestCheckRequiresInventoriedDocumentedLawsTest(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.RequiredTests = []string{"fixture/TestLawsAreDocumented"}
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, "go.mod", "module example.test/fixture\n\ngo 1.25\n")
	writeFixture(t, root, "LAWS.md", testLaw)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check passed without the independently inventoried test")
	}
	assertReport(t, directory, "001.xml", "fixture/TestLawsAreDocumented", "<error")
}

const nodeTapFixture = `result='ok'; [ "$LAWGATE_BITE" = report-required ] && result='not ok'; printf '%s\n' 'TAP version 13' "$result 1 - TestLaw" '1..1'; [ "$result" = ok ]`

func TestCheckRunsNodeModuleWithoutPackageManifest(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language = "node"
	// The fixture bites like a real enforcer: it reports a failure while its
	// law is switched off with LAWGATE_BITE.
	module.TestCommand = []string{"sh", "-c", nodeTapFixture}
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, "LAWS.md", testLaw)
	first, err := runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runCheck(t, root)
	if first == second || err != nil {
		t.Fatalf("expected fresh successful run, got %q %q: %v", first, second, err)
	}
	assertReport(t, second, "001.xml", "TestLaw", "errors=\"0\"")
}

func TestCheckReportsInventoryFailure(t *testing.T) {
	directory, err := runCheck(t, t.TempDir())
	if err == nil {
		t.Fatal("check accepted missing inventory")
	}
	assertReport(t, directory, "laws-summary.md", "lawgate.json", infrastructure)
	assertReport(t, directory, "laws-report.html", "lawgate.json", "Some laws could not be checked")
	assertReport(t, directory, "001.xml", "lawgate.json", "<error")
}

type machineSetup int

const (
	withoutMachines machineSetup = iota
	withMachines
)

func fixtureModule(name string, setup machineSetup) inventory.Module {
	machines := setup == withMachines
	stream := `{"Action":"run","Package":"example.test/fixture","Test":"TestLaw"}
{"Action":"pass","Package":"example.test/fixture","Test":"TestLaw"}
{"Action":"pass","Package":"example.test/fixture"}`
	command := []string{"sh", "-c", "printf '%s\\n' '" + stream + "'"}
	return inventory.Module{Path: name, Language: "go", Laws: filepath.Join(name, "LAWS.md"), Machines: &machines, TestCommand: command}
}

func shellReport(content string) []string {
	return []string{"sh", "-c", "printf '%s' \"$1\" > \"$4\"", "machine", content}
}

func writeConfig(t *testing.T, root string, modules []inventory.Module) {
	t.Helper()
	writeGateConfig(t, root, inventory.Config{Modules: modules})
}

func writeGateConfig(t *testing.T, root string, config inventory.Config) {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, inventory.Filename, string(data))
}

func runCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := checkCommand([]string{"--root", root}, &output)
	directory := strings.TrimSpace(output.String())
	if err != nil {
		logSummary(t, directory)
	}
	return directory, err
}

// logSummary puts a failed run's summary in the test log, since the run's
// directory is removed with the test's temporary folder.
func logSummary(t *testing.T, directory string) {
	t.Helper()
	summary, err := os.ReadFile(filepath.Join(directory, "laws-summary.md"))
	if err == nil {
		t.Log(string(summary))
	}
}

func writeFixture(t *testing.T, root, name, data string) {
	t.Helper()
	filename := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertReport(t *testing.T, root, filename string, fragments ...string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filename))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range fragments {
		if !bytes.Contains(content, []byte(fragment)) {
			t.Errorf("%s missing %q", filename, fragment)
		}
	}
}

const lawTestSource = "package fixture\nimport (\n\t\"os\"\n\t\"testing\"\n)\n// TestLaw enforces report-required.\nfunc TestLaw(test *testing.T) {\n\tif os.Getenv(\"LAWGATE_BITE\") != \"\" {\n\t\ttest.Fatal(\"law switched off\")\n\t}\n}\n"

func machineLaw(name, enforcer string) string {
	return "\n## " + name + "\n- Source: owner decision 2026-11-20\n- Enforced by: machine `" + enforcer + "`\n- Status: agreed 2026-11-20\n"
}
