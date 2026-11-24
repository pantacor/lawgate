package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/rulechanges"
)

// userOwnedRepository is a repository whose base lawgate.json names a user
// owner, with the commit that changed nothing yet as BASE.
func userOwnedRepository(t *testing.T) (root, base string) {
	t.Helper()
	root = ownersRepository(t)
	writeFixture(t, root, "lawgate.json", strings.ReplaceAll(ownedConfig, "@org/law-owners", "@owner"))
	return root, commitAll(t, root)
}

// outsideCI clears the variables a CI job would set, so the test judges
// the same way on a laptop and in lawgate's own workflow.
func outsideCI(t *testing.T) {
	for _, name := range []string{"GITHUB_EVENT_NAME", "GITHUB_EVENT_PATH", "GITHUB_TOKEN", "CI_MERGE_REQUEST_IID", "LAWGATE_API_TOKEN"} {
		t.Setenv(name, "")
	}
}

func TestRequireApprovalFailsClosedOutsideCI(t *testing.T) {
	outsideCI(t)
	root, base := userOwnedRepository(t)
	writeFixture(t, root, "LAWS.md", testLaw+"\n")
	head := commitAll(t, root)
	var output bytes.Buffer
	err := run([]string{"rule-changes", "--root", root, "--require-approval", base, head}, &output)
	if err == nil || !strings.Contains(err.Error(), "GITHUB_EVENT_NAME is not set") {
		t.Fatalf("a rule change outside CI must fail and name the missing variable: %v", err)
	}
	if !strings.Contains(output.String(), `rule file: "LAWS.md"`) {
		t.Fatalf("the audit output was lost:\n%s", &output)
	}
}

func TestRequireApprovalPassesWithoutRuleFiles(t *testing.T) {
	outsideCI(t)
	root, base := userOwnedRepository(t)
	writeFixture(t, root, "notes.txt", "not a rule file\n")
	head := commitAll(t, root)
	var output bytes.Buffer
	if err := run([]string{"rule-changes", "--root", root, "--require-approval", base, head}, &output); err != nil {
		t.Fatalf("a change without rule files needs no approval: %v", err)
	}
	if !strings.Contains(output.String(), "no rule files changed: no approval needed") {
		t.Fatalf("the pass is not explained:\n%s", &output)
	}
}

func TestRequireApprovalRefusesATeamOwner(t *testing.T) {
	outsideCI(t)
	root := ownersRepository(t)
	base := gitIn(t, root, "rev-parse", "HEAD")
	writeFixture(t, root, "LAWS.md", testLaw+"\n")
	head := commitAll(t, root)
	err := run([]string{"rule-changes", "--root", root, "--require-approval", base, head}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "is a team or group") {
		t.Fatalf("a team owner cannot approve: %v", err)
	}
}

func TestApprovalOwnerComesFromTheBase(t *testing.T) {
	root, base := userOwnedRepository(t)
	writeFixture(t, root, "lawgate.json", strings.ReplaceAll(ownedConfig, "@org/law-owners", "@intruder"))
	head := commitAll(t, root)
	options := changesOptions{root: root, base: base, head: head}
	audited, err := rulechanges.AuditRange(root, base, head, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	request, err := approvalRequest(options, audited)
	if err != nil {
		t.Fatal(err)
	}
	fromBase := request.Owner == "@owner" && request.Platform == "github"
	if !fromBase || request.Head != head {
		t.Fatalf("the candidate chose its own approver: %+v", request)
	}
}

func TestRuleChangesWithoutTheFlagOnlyAudits(t *testing.T) {
	outsideCI(t)
	root, base := userOwnedRepository(t)
	writeFixture(t, root, "LAWS.md", testLaw+"\n")
	head := commitAll(t, root)
	if err := run([]string{"rule-changes", "--root", root, base, head}, io.Discard); err != nil {
		t.Fatalf("the audit alone reads no approval: %v", err)
	}
}

// approvalOnRepository is userOwnedRepository with require_approval on at
// BASE.
func approvalOnRepository(t *testing.T) (root, base string) {
	t.Helper()
	root = ownersRepository(t)
	config := strings.ReplaceAll(ownedConfig, `"owner":"@org/law-owners"`, `"owner":"@owner","require_approval":true`)
	writeFixture(t, root, "lawgate.json", config)
	return root, commitAll(t, root)
}

func TestRequireApprovalAtBaseFailsClosedWithoutToken(t *testing.T) {
	outsideCI(t)
	root, base := approvalOnRepository(t)
	writeFixture(t, root, "LAWS.md", testLaw+"\n")
	head := commitAll(t, root)
	err := run([]string{"rule-changes", "--root", root, base, head}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN is not set") {
		t.Fatalf("require_approval at BASE must check the approval and fail closed: %v", err)
	}
}

func TestSwitchingApprovalOffInTheChangeKeepsTheCheck(t *testing.T) {
	outsideCI(t)
	root, base := approvalOnRepository(t)
	writeFixture(t, root, "lawgate.json", strings.ReplaceAll(ownedConfig, "@org/law-owners", "@owner"))
	writeFixture(t, root, "LAWS.md", testLaw+"\n")
	head := commitAll(t, root)
	err := run([]string{"rule-changes", "--root", root, base, head}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "GITHUB_EVENT_NAME is not set") {
		t.Fatalf("a change switched require_approval off for itself: %v", err)
	}
}

func TestSwitchingApprovalOnTakesEffectFromTheNextChange(t *testing.T) {
	outsideCI(t)
	root, base := userOwnedRepository(t)
	config := strings.ReplaceAll(ownedConfig, `"owner":"@org/law-owners"`, `"owner":"@owner","require_approval":true`)
	writeFixture(t, root, "lawgate.json", config)
	head := commitAll(t, root)
	if err := run([]string{"rule-changes", "--root", root, base, head}, io.Discard); err != nil {
		t.Fatalf("the change that turns require_approval on is judged by its BASE: %v", err)
	}
}
