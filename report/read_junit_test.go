package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJUnitRejectsUnidentifiedOrConflictingVerdicts(t *testing.T) {
	cases := map[string]string{
		`<testsuite><testcase name="law"/></testsuite>`:                                                                                     "machine suite has no name",
		`<testsuite name="m"><testcase name="law"/><testcase name="law"/></testsuite>`:                                                      "duplicate verdict",
		`<testsuite name="m"><testcase name="law"><failure/><skipped/></testcase></testsuite>`:                                              "conflicting verdicts",
		`<testsuites><testsuite name="m"><testcase name="l"/></testsuite><testsuite name="m"><testcase name="l"/></testsuite></testsuites>`: "duplicate machine suite",
	}
	for input, expected := range cases {
		readJUnitError(t, writeJUnitInput(t, input), expected)
	}
}

func TestReadJUnitRejectsOversizeInput(t *testing.T) {
	content := `<testsuite name="m"><testcase name="law"/></testsuite>`
	content += strings.Repeat(" ", 16<<20)
	readJUnitError(t, writeJUnitInput(t, content), "JUnit report exceeds")
}

func TestReadJUnitRejectsNonRegularInput(t *testing.T) {
	readJUnitError(t, t.TempDir(), "JUnit report is not a regular file")
}

func writeJUnitInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.xml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readJUnitError(t *testing.T, path, expected string) {
	t.Helper()
	if _, err := ReadJUnit(path); err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("ReadJUnit error = %v, want %q", err, expected)
	}
}
