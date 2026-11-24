package main

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const ownedConfig = `{"owner":"@org/law-owners","platform":"github","modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false}]}`

func ownersRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	writeFixture(t, root, "lawgate.json", ownedConfig)
	writeFixture(t, root, "LAWS.md", testLaw)
	writeFixture(t, root, ".github/CODEOWNERS", "* @org/law-owners\n")
	gitIn(t, root, "init", "-q", "-b", "main")
	commitAll(t, root)
	return root
}

func gitIn(t *testing.T, root string, args ...string) string {
	t.Helper()
	identity := []string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}
	command := exec.Command("git", append(identity, args...)...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func commitAll(t *testing.T, root string) string {
	t.Helper()
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "change")
	return gitIn(t, root, "rev-parse", "HEAD")
}

func TestCheckCodeownersUsesTheConfiguredOwner(t *testing.T) {
	root := ownersRepository(t)
	if err := run([]string{"check-codeowners", "--root", root}, io.Discard); err != nil {
		t.Fatalf("configured team owner not used: %v", err)
	}
	writeFixture(t, root, "lawgate.json", strings.ReplaceAll(ownedConfig, `"owner":"@org/law-owners",`, ""))
	err := run([]string{"check-codeowners", "--root", root}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "CODEOWNER") {
		t.Fatalf("check without any owner = %v", err)
	}
}

// TestCandidateCannotChooseItsOwnReviewer enforces rule-changes-are-reviewed:
// with --base, the owner comes from the base's lawgate.json.
func TestCandidateCannotChooseItsOwnReviewer(t *testing.T) {
	root := ownersRepository(t)
	base := gitIn(t, root, "rev-parse", "HEAD")
	writeFixture(t, root, "lawgate.json", strings.ReplaceAll(ownedConfig, "@org/law-owners", "@intruder"))
	writeFixture(t, root, ".github/CODEOWNERS", "* @intruder\n")
	commitAll(t, root)
	if err := run([]string{"check-codeowners", "--root", root}, io.Discard); err != nil {
		t.Fatalf("worktree check should accept its own owner: %v", err)
	}
	err := run([]string{"check-codeowners", "--root", root, "--base", base}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "@org/law-owners") {
		t.Fatalf("candidate picked its own reviewer: %v", err)
	}
}

func TestRuleFilesReportsBrokenFileAndListsTheRest(t *testing.T) {
	root := ownersRepository(t)
	writeFixture(t, root, "broken.go", "package main\nfunc {")
	writeFixture(t, root, "testdata/bad.go", "func {")
	var output bytes.Buffer
	err := run([]string{"rule-files", "--root", root}, &output)
	if err == nil || !strings.Contains(err.Error(), "broken.go") {
		t.Fatalf("broken file not reported: %v", err)
	}
	listed := output.String()
	if !strings.Contains(listed, "broken.go\n") || !strings.Contains(listed, "LAWS.md\n") {
		t.Fatalf("rule-files stopped at the broken file:\n%s", listed)
	}
}
