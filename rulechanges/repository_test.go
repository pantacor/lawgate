package rulechanges

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitFixture is a throwaway repository in a test directory.
type gitFixture struct {
	t    *testing.T
	root string
}

func newGitFixture(t *testing.T) gitFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	fixture := gitFixture{t: t, root: t.TempDir()}
	fixture.git("init", "-q", "-b", "main")
	return fixture
}

func (fixture gitFixture) git(args ...string) string {
	fixture.t.Helper()
	identity := []string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}
	command := exec.Command("git", append(identity, args...)...)
	command.Dir = fixture.root
	output, err := command.CombinedOutput()
	if err != nil {
		fixture.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (fixture gitFixture) write(files map[string]string) {
	fixture.t.Helper()
	for name, data := range files {
		full := filepath.Join(fixture.root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			fixture.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			fixture.t.Fatal(err)
		}
	}
}

func (fixture gitFixture) commit(files map[string]string) string {
	fixture.t.Helper()
	fixture.write(files)
	fixture.git("add", "-A")
	fixture.git("commit", "-q", "-m", "change")
	return fixture.git("rev-parse", "HEAD")
}

// TestEmptiedHelperIsARuleChange enforces rule-changes-are-reviewed: a
// change that empties the helper an enforcing test calls must need the
// owner's review and must appear in the audit.
func TestEmptiedHelperIsARuleChange(t *testing.T) {
	fixture := newGitFixture(t)
	base := closureSources()
	base[".github/CODEOWNERS"] = "/LAWS.md " + requiredOwner + "\n/owner_test.go " + requiredOwner + "\n"
	baseID := fixture.commit(base)
	headID := fixture.commit(map[string]string{"assert.go": "package project\nfunc assertSame(t any) {}\n// emptied"})
	listed, err := RuleFiles(fixture.root)
	files := listed.Files
	if err != nil {
		t.Fatal(err)
	}
	coverage := CheckOwners(fixture.root, files, requiredOwner, "github")
	if coverage == nil || !strings.Contains(coverage.Error(), "assert.go") {
		t.Fatalf("check-codeowners missed the helper: %v", coverage)
	}
	var audit bytes.Buffer
	if err := Audit(fixture.root, baseID, headID, &audit); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(audit.String(), `rule file: "assert.go"`) {
		t.Fatalf("audit missed the helper change:\n%s", &audit)
	}
}

func TestAuditCarriesOnPastUnparsableCommit(t *testing.T) {
	fixture := newGitFixture(t)
	baseID := fixture.commit(closureSources())
	headID := fixture.commit(map[string]string{"helper.go": "package project\nfunc {"})
	var audit bytes.Buffer
	if err := Audit(fixture.root, baseID, headID, &audit); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(audit.String(), `rule file: "helper.go"`) {
		t.Fatalf("audit lost the broken rule file:\n%s", &audit)
	}
}

func TestConfigAtReadsTheCommittedOwner(t *testing.T) {
	fixture := newGitFixture(t)
	config := `{"owner":"@base-owner","platform":"github","modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false}]}`
	baseID := fixture.commit(map[string]string{"lawgate.json": config})
	fixture.write(map[string]string{"lawgate.json": strings.ReplaceAll(config, "@base-owner", "@intruder")})
	committed, err := ConfigAt(fixture.root, baseID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Owner != "@base-owner" {
		t.Fatalf("owner at base = %q", committed.Owner)
	}
}
