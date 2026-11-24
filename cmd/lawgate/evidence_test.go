package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/report"
)

func TestGoRunAndCompletionMustBelongToSamePackage(t *testing.T) {
	stream := `{"Action":"run","Package":"first","Test":"TestLaw"}
{"Action":"pass","Package":"second","Test":"TestLaw"}
{"Action":"run","Package":"second","Test":"TestOther"}
{"Action":"pass","Package":"second","Test":"TestOther"}`
	items, err := testEvidence("go", []byte(stream), []string{"first/TestLaw"})
	if err != nil {
		t.Fatal(err)
	}
	missing := slices.ContainsFunc(items, func(item verdict) bool {
		return item.Name == "first/TestLaw" && item.Status == infrastructure
	})
	if !missing {
		t.Fatalf("mismatched packages supplied evidence for TestLaw: %+v", items)
	}
}

func TestPackageQualifiedGoTestRequiresThatPackage(t *testing.T) {
	stream := `{"Action":"run","Package":"other","Test":"TestLaw"}
{"Action":"pass","Package":"other","Test":"TestLaw"}`
	items, err := testEvidence("go", []byte(stream), []string{"owner/TestLaw"})
	if err != nil {
		t.Fatal(err)
	}
	assertVerdict(t, items, "owner/TestLaw", infrastructure)
}

func TestGoPackageCrashHasInfrastructureStatus(t *testing.T) {
	stream := `{"Action":"run","Package":"good","Test":"TestLaw"}
{"Action":"pass","Package":"good","Test":"TestLaw"}
{"Action":"fail","Package":"crash"}`
	items, err := testEvidence("go", []byte(stream), []string{"good/TestLaw"})
	if err != nil {
		t.Fatal(err)
	}
	assertVerdict(t, items, "crash", infrastructure)
}

func TestNodeEvidenceRejectsIncompleteTAP(t *testing.T) {
	stream := "TAP version 13\nok 1 - expected test\n"
	_, err := testEvidence("node", []byte(stream), []string{"expected test"})
	if err == nil {
		t.Fatal("accepted a Node report without a completed plan")
	}
}

func TestNodeSkipDoesNotEnforceLaw(t *testing.T) {
	stream := "TAP version 13\nnot ok 1 - later # TODO\nok 2 - rule # SKIP\nok 3 - unrelated\n1..3\n"
	items, err := testEvidence("node", []byte(stream), []string{"rule"})
	if err != nil {
		t.Fatal(err)
	}
	assertVerdict(t, items, "rule", infrastructure)
	assertVerdict(t, items, "later", skipped)
}

func TestMachineAcceptsNoEmptySuiteInsideNonemptyReport(t *testing.T) {
	data := `<testsuites tests="1"><testsuite name="empty" tests="0"/><testsuite name="other" tests="1"><testcase name="law"/></testsuite></testsuites>`
	_, err := readFixtureJUnit(t, data)
	if err == nil || !strings.Contains(err.Error(), "zero verdicts") {
		t.Fatalf("invalid report should identify the empty suite, got %v", err)
	}
}

func TestMachineRejectsTruncatedDetailWithPassingCases(t *testing.T) {
	data := `<testsuite tests="1" failures="1"><testcase name="law"/></testsuite>`
	_, err := readFixtureJUnit(t, data)
	if err == nil {
		t.Fatal("accepted a passing case while the report declares a failure")
	}
}

func assertVerdict(t *testing.T, items []verdict, name, status string) {
	t.Helper()
	for _, item := range items {
		if item.Name == name && item.Status == status {
			return
		}
	}
	t.Fatalf("missing %s verdict for %q in %+v", status, name, items)
}

func readFixtureJUnit(t *testing.T, data string) ([]report.LawRun, error) {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "report.xml", data)
	return machineVerdicts(filepath.Join(root, "report.xml"))
}

func testEvidence(language string, data []byte, names []string) ([]verdict, error) {
	evidence, err := readTestEvidence(language, data)
	for _, name := range names {
		if missing := evidence.tests.require(name); missing != nil {
			evidence.Verdicts = append(evidence.Verdicts, verdict{name, infrastructure, missing.Error()})
		}
	}
	return evidence.Verdicts, err
}

func TestMachineRejectsSkippedVerdictWithoutPendingReason(t *testing.T) {
	data := "<testsuite name=\"m\"><testcase name=\"law\"><skipped/></testcase></testsuite>"
	_, err := readFixtureJUnit(t, data)
	if err == nil || !strings.Contains(err.Error(), "pending reason") {
		t.Fatalf("expected missing pending reason, got %v", err)
	}
}

func TestNodeEvidenceRejectsAmbiguousEnforcer(t *testing.T) {
	stream := "TAP version 13\nok 1 - same # SKIP\nok 2 - same\n1..2\n"
	items, err := testEvidence("node", []byte(stream), []string{"same"})
	if err != nil {
		t.Fatal(err)
	}
	assertVerdict(t, items, "same", infrastructure)
}

const nodeTAPBlockStream = `TAP version 13
# Subtest: passing
ok 1 - passing
  ---
  duration_ms: 0.1
  ...
# Subtest: parent
    # Subtest: nested fail
    not ok 1 - nested fail
      ---
      location: 'law.test.js:10:3'
      error: |-
        Expected values to be equal:

        ok 99 - fake inside yaml
      ...
    1..1
not ok 2 - parent
  ---
  error: '1 subtest failed'
  ...
not ok 3 - bare fail
1..3
# tests 4
`

func TestNodeEvidenceCapturesNamedFailureTAPBlock(t *testing.T) {
	items, err := testEvidence("node", []byte(nodeTAPBlockStream), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d verdicts, want 4 (YAML must not add a fake test): %+v", len(items), items)
	}
	assertNodeDetail(t, items, "passing", "")
	wantNested := "not ok 1 - nested fail\n  ---\n  location: 'law.test.js:10:3'\n  error: |-\n    Expected values to be equal:\n\n    ok 99 - fake inside yaml\n  ..."
	assertNodeDetail(t, items, "nested fail", wantNested)
	assertNodeDetail(t, items, "bare fail", "not ok 3 - bare fail")
}

func assertNodeDetail(t *testing.T, items []verdict, name, want string) {
	t.Helper()
	for _, item := range items {
		if item.Name != name {
			continue
		}
		if item.Detail != want {
			t.Fatalf("%q detail =\n%q\nwant:\n%q", name, item.Detail, want)
		}
		return
	}
	t.Fatalf("missing verdict %q in %+v", name, items)
}
