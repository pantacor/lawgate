package rulechanges

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vendoredLaws = "vendor/" + vendoredKernel + "/laws.go"

// TestPatchedVendoredKernelNeedsTheOwner enforces rule-changes-are-reviewed
// : in a module that vendors lawgate, a change that patches the
// vendored kernel must need the owner's review and appear in the audit.
func TestPatchedVendoredKernelNeedsTheOwner(t *testing.T) {
	fixture := newGitFixture(t)
	base := vendoredSources()
	delete(base, "vendor/github.com/other/unused/bad.go")
	base[".github/CODEOWNERS"] = "/LAWS.md " + requiredOwner + "\n/w/law_test.go " + requiredOwner + "\n"
	baseID := fixture.commit(base)
	patch := map[string]string{vendoredLaws: base[vendoredLaws] + "// verdict without violations\n"}
	headID := fixture.commit(patch)
	listed, err := RuleFiles(fixture.root)
	files := listed.Files
	if err != nil {
		t.Fatal(err)
	}
	coverage := CheckOwners(fixture.root, files, requiredOwner, "github")
	if coverage == nil || !strings.Contains(coverage.Error(), vendoredLaws) {
		t.Fatalf("check-codeowners missed the vendored kernel: %v", coverage)
	}
	var audit bytes.Buffer
	if err := Audit(fixture.root, baseID, headID, &audit); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(audit.String(), vendoredLaws) {
		t.Fatalf("audit missed the vendored kernel:\n%s", audit.String())
	}
}

// TestWorktreeSymlinkedScriptEnforcerIsRefused: the check
// refuses a symlinked script enforcer, so ownership reports it too.
func TestWorktreeSymlinkedScriptEnforcerIsRefused(t *testing.T) {
	fixture := newGitFixture(t)
	fixture.write(map[string]string{
		"LAWS.md":        "## law\n- Enforced by: script `scripts/check.sh`",
		"tools/check.sh": "#!/bin/sh\n",
	})
	if err := os.Symlink("tools", filepath.Join(fixture.root, "scripts")); err != nil {
		t.Fatal(err)
	}
	_, err := RuleFiles(fixture.root)
	if err == nil || !strings.Contains(err.Error(), "symbolic link scripts") {
		t.Fatalf("symlinked script enforcer: %v", err)
	}
}

// TestStarterListsVendoredPackages: init's starter walk of a directory
// that is not a Git worktree yet sees vendor/ and .github like Git would.
func TestStarterListsVendoredPackages(t *testing.T) {
	root := t.TempDir()
	files := vendoredSources()
	delete(files, "vendor/github.com/other/unused/bad.go")
	files[".github/workflows/ci.yml"] = "on: push\n"
	for name, data := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := StarterRuleFiles(root, []byte(`{"modules":[]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(listed, "\n")
	for _, name := range []string{vendoredLaws, "vendor/modules.txt", ".github/workflows/ci.yml"} {
		if !strings.Contains(joined, name) {
			t.Errorf("starter missed %s: %v", name, listed)
		}
	}
}
