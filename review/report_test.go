package review

import (
	"slices"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/review/internal/fixture"
)

// branchFixture is the shop repository on a feature branch that changes a
// site, the subject of never-overspend and its law text in one commit, and
// leaves a change to the law's test uncommitted.
func branchFixture(t *testing.T) string {
	t.Helper()
	root := fixture.Repo(t)
	fixture.Git(t, root, "checkout", "--quiet", "-b", "feature")
	fixture.Replace(t, root, "worker.go", "SHOP_JOB 1 over balance", "SHOP_JOB 1 over the balance")
	fixture.Replace(t, root, "worker.go", "return amount <= balance", "return amount > 0 && amount <= balance")
	fixture.Replace(t, root, "LAWS.md", "recorded over the balance.", "recorded over the account's balance.")
	fixture.Commit(t, root, "shop: refuse empty payments")
	fixture.Replace(t, root, "worker_test.go", "Pay(5, 3)", "Pay(6, 3)")
	return root
}

func reviewFixture(t *testing.T, root string, options ReportOptions) *Report {
	t.Helper()
	report, err := Review(buildFixture(t, root), "main", options)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func fileReport(t *testing.T, report *Report, path string) FileReport {
	t.Helper()
	for _, file := range report.Files {
		if file.Path == path {
			return file
		}
	}
	t.Fatalf("%s is not in the report: %+v", path, report.Files)
	return FileReport{}
}

// TestReviewGroupsChangesByWhatGovernsThem: each hunk names the states,
// sites, subjects, tests and laws it touches and the commit behind it.
func TestReviewGroupsChangesByWhatGovernsThem(t *testing.T) {
	report := reviewFixture(t, branchFixture(t), ReportOptions{})
	if len(report.Commits) != 1 || report.Commits[0].Subject != "shop: refuse empty payments" {
		t.Fatalf("commits = %+v", report.Commits)
	}
	worker := fileReport(t, report, "worker.go")
	var states, laws, subjects []string
	for _, hunk := range worker.Hunks {
		states = append(states, hunk.States...)
		laws = append(laws, hunk.Laws...)
		for _, ref := range hunk.Subjects {
			subjects = append(subjects, ref.Name)
		}
	}
	if !slices.Contains(states, "pay/pay") || !slices.Contains(subjects, "example.com/shop.affordable") {
		t.Fatalf("worker.go: states %v, subjects %v", states, subjects)
	}
	if !slices.Contains(laws, "never-overspend") || !slices.Contains(laws, "no-silent-errors") {
		t.Fatalf("worker.go laws = %v", laws)
	}
	if commits := worker.Hunks[0].Commits; len(commits) != 1 || worker.Hunks[0].Uncommitted {
		t.Fatalf("worker.go first hunk: commits %v, uncommitted %v", commits, worker.Hunks[0].Uncommitted)
	}
	test := fileReport(t, report, "worker_test.go").Hunks[0]
	if len(test.Tests) != 1 || !test.Uncommitted {
		t.Fatalf("worker_test.go hunk = %+v", test)
	}
	checkLawText(t, report)
}

func checkLawText(t *testing.T, report *Report) {
	t.Helper()
	laws := fileReport(t, report, "LAWS.md")
	if !laws.RuleFile || strings.Join(laws.Hunks[0].LawText, ",") != "never-overspend" {
		t.Fatalf("LAWS.md = %+v", laws)
	}
	impact := report.Laws[0]
	if impact.Key != "never-overspend" || !impact.TextChanged {
		t.Fatalf("first law = %+v, want never-overspend with changed text first", impact)
	}
}

// TestReviewListsRuleChangesAndTheirTrailers: a commit that touches LAWS.md
// is a rule change; its audit trailer is reported.
func TestReviewListsRuleChangesAndTheirTrailers(t *testing.T) {
	root := branchFixture(t)
	fixture.Replace(t, root, "LAWS.md", "Status: agreed 2026-11-24\n\n## never", "Status: agreed 2026-11-25\n\n## never")
	fixture.Commit(t, root, "laws: redate no-silent-errors\n\nRule-change-approved-by: @shop-owner")
	report := reviewFixture(t, root, ReportOptions{})
	if len(report.RuleChanges) != 2 {
		t.Fatalf("rule changes = %+v", report.RuleChanges)
	}
	approved, unapproved := report.RuleChanges[0], report.RuleChanges[1]
	if !approved.Approved || strings.Join(approved.ApprovedBy, ",") != "@shop-owner" {
		t.Fatalf("approved change = %+v", approved)
	}
	if unapproved.Approved || !slices.Contains(unapproved.Files, "LAWS.md") {
		t.Fatalf("unapproved change = %+v", unapproved)
	}
	markdown := Markdown(report, "")
	for _, want := range []string{ReportMarker, "### Rule changes", "approved by @shop-owner", "needs the owner's review", "`never-overspend`"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
}

const lockTemplate = `{
  "version": 1,
  "laws": [
    {"module": ".", "document": "LAWS.md", "law": "never-overspend", "proof": "PROOF"}
  ]
}
`

// TestReviewCallsOutProofLockChanges: a branch that changes LAWS.lock says
// which law's proof changed.
func TestReviewCallsOutProofLockChanges(t *testing.T) {
	root := fixture.Repo(t)
	fixture.Write(t, root, "LAWS.lock", strings.Replace(lockTemplate, "PROOF", "aaaa", 1))
	fixture.Commit(t, root, "lock the proofs")
	fixture.Git(t, root, "checkout", "--quiet", "-b", "feature")
	fixture.Write(t, root, "LAWS.lock", strings.Replace(lockTemplate, "PROOF", "bbbb", 1))
	report := reviewFixture(t, root, ReportOptions{})
	changed := len(report.LockChanges) == 1 && strings.Contains(report.LockChanges[0], "proof of never-overspend changed")
	if !changed {
		t.Fatalf("lock changes = %v", report.LockChanges)
	}
	if lock := fileReport(t, report, "LAWS.lock"); !lock.LockFile {
		t.Fatalf("LAWS.lock = %+v", lock)
	}
	if !strings.Contains(Markdown(report, ""), "### Proof lock") {
		t.Fatal("the markdown report does not call out the proof lock")
	}
}

// TestReviewScopeAndUntrackedFiles: a review limited to a path reports only
// it, and a file git does not track yet is a change.
func TestReviewScopeAndUntrackedFiles(t *testing.T) {
	root := branchFixture(t)
	fixture.Write(t, root, "notes/todo.txt", "law never-overspend needs a second test\n")
	report := reviewFixture(t, root, ReportOptions{Paths: []string{"notes"}})
	if len(report.Files) != 1 || report.Files[0].Status != "untracked" {
		t.Fatalf("files = %+v", report.Files)
	}
	if len(report.Laws) != 1 || report.Laws[0].Key != "never-overspend" {
		t.Fatalf("laws = %+v", report.Laws)
	}
}

func TestParseDiff(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\nindex 1..2 100644\n--- a/x.go\n+++ b/x.go\n" +
		"@@ -10,2 +10,3 @@ func decide() {\n-\told\n-\tb\n+\ta\n+\tb\n+\tc\n" +
		"@@ -40 +41,0 @@ func other() {\n-\tgone\n" +
		"diff --git a/new.md b/new.md\nnew file mode 100644\n--- /dev/null\n+++ b/new.md\n@@ -0,0 +1 @@\n+hello\n"
	files := ParseDiff(diff)
	if len(files) != 2 || len(files[0].Hunks) != 2 {
		t.Fatalf("files = %+v", files)
	}
	first := files[0].Hunks[0]
	if first.NewEnd() != 12 || first.Header != "func decide() {" {
		t.Fatalf("first hunk = %+v", first)
	}
	if removal := files[0].Hunks[1]; removal.NewLines != 0 || removal.NewEnd() != 41 {
		t.Fatalf("removal = %+v", removal)
	}
	if files[1].Status != "added" || files[1].Path != "new.md" {
		t.Fatalf("new file = %+v", files[1])
	}
}
