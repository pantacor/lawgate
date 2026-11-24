package rulechanges

import (
	"io"
	"slices"
	"testing"
)

func TestAuditRangeReturnsTheCommitsThatChangedRuleFiles(t *testing.T) {
	fixture := auditFixture()
	fixture["show -s --format=%(trailers:only,unfold) "+retroID] = ""
	result, err := fakeHistory(t, fixture).auditRange("base", "head", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.Head != retroID {
		t.Fatalf("head = %q, want %q", result.Head, retroID)
	}
	if len(result.Changes) != 2 {
		t.Fatalf("want the rename and the merge, got %+v", result.Changes)
	}
	assertRuleCommit(t, result.Changes[0], renameID, "from.go", "to.txt")
	assertRuleCommit(t, result.Changes[1], mergeID, "resolution.go")
}

func TestAuditRangeKeepsTheChangesWhenATrailerFails(t *testing.T) {
	result, err := fakeHistory(t, auditFixture()).auditRange("base", "head", io.Discard)
	if err == nil {
		t.Fatal("a retroactive note without its own trailer must fail the audit")
	}
	if len(result.Changes) != 2 {
		t.Fatalf("a failing trailer must not hide rule changes, got %+v", result.Changes)
	}
}

func assertRuleCommit(t *testing.T, change RuleCommit, id string, files ...string) {
	t.Helper()
	if change.ID != id || !slices.Equal(change.Files, files) {
		t.Fatalf("got %s %v, want %s %v", change.ID, change.Files, id, files)
	}
}
