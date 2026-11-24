package rulechanges

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

var baseID = strings.Repeat("a", 40)
var renameID = strings.Repeat("b", 40)
var mergeID = strings.Repeat("c", 40)
var retroID = strings.Repeat("d", 40)

func TestAuditIncludesBothRenamePathsAndMergeResolution(t *testing.T) {
	fixture := auditFixture()
	fixture["show -s --format=%(trailers:only,unfold) "+retroID] = "Rule-change-approved-by: owner\nRule-change-approves: " + renameID
	var audit bytes.Buffer
	err := fakeHistory(t, fixture).audit("base", "head", &audit)
	if err != nil {
		t.Fatal(err)
	}
	renameStart := strings.Index(audit.String(), renameID)
	mergeStart := strings.Index(audit.String(), mergeID)
	retroStart := strings.Index(audit.String(), retroID)
	inOrder := renameStart >= 0 && mergeStart > renameStart && retroStart > mergeStart
	if !inOrder {
		t.Fatalf("audit lost a commit or changed order:\n%s", &audit)
	}
	renameOutput := audit.String()[renameStart:mergeStart]
	mergeOutput := audit.String()[mergeStart:retroStart]
	assertAuditText(t, renameOutput, "from.go", "to.txt", "(none)")
	assertAuditText(t, mergeOutput, "resolution.go")
	assertAuditText(t, audit.String()[retroStart:], "Rule-change-approves (audit)", renameID)
}

// TestAuditRejectsRetroactiveNoteWithoutItsOwnApprovalTrailer enforces
// rule-changes-are-reviewed for the audit trail of approvals.
func TestAuditRejectsRetroactiveNoteWithoutItsOwnApprovalTrailer(t *testing.T) {
	fixture := auditFixture()
	approvedEarlier := "show -s --format=%(trailers:only,unfold) " + renameID
	fixture[approvedEarlier] = "Rule-change-approved-by: owner"
	err := fakeHistory(t, fixture).audit("base", "head", io.Discard)
	want := retroID + ": Rule-change-approves requires a Rule-change-approved-by audit trailer"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected retroactive audit trailer failure on %s, got %v", retroID, err)
	}
	fixture["show -s --format=%(trailers:only,unfold) "+retroID] = ""
	if err := fakeHistory(t, fixture).audit("base", "head", io.Discard); err != nil {
		t.Fatalf("a commit with no approval note must pass the audit: %v", err)
	}
}

func TestAuditRejectsMissingOrUnrelatedBase(t *testing.T) {
	fixture := auditFixture()
	delete(fixture, "rev-parse --verify --end-of-options base^{commit}")
	delete(fixture, "merge-base --all "+baseID+" "+retroID)
	repo := fakeHistory(t, fixture)
	if err := repo.audit("base", "head", io.Discard); err == nil {
		t.Fatal("missing base produced a successful audit")
	}
	fixture["rev-parse --verify --end-of-options base^{commit}"] = baseID
	fixture["merge-base --all "+baseID+" "+retroID] = ""
	if err := repo.audit("base", "head", io.Discard); err == nil {
		t.Fatal("unrelated base produced a successful audit")
	}
}

func auditFixture() map[string]string {
	sharedBlobs := "100644 blob machine\tresolution.go\x00"
	return map[string]string{
		"rev-parse --is-shallow-repository":                                         "false\n",
		"rev-parse --verify --end-of-options base^{commit}":                         baseID,
		"rev-parse --verify --end-of-options head^{commit}":                         retroID,
		"merge-base --all " + baseID + " " + retroID:                                baseID + "\n",
		"rev-list --reverse --topo-order --parents " + retroID + " --not " + baseID: auditHistory(),
		diffCommand(renameID):                                                       "R100\x00from.go\x00to.txt\x00",
		diffCommand(mergeID):                                                        "M\x00resolution.go\x00M\x00resolution.go\x00",
		diffCommand(retroID):                                                        "M\x00README.md\x00",
		"ls-tree -r -z " + baseID:                                                   "100644 blob machine\tfrom.go\x00",
		"ls-tree -r -z " + renameID:                                                 "100644 blob machine\tto.txt\x00",
		"ls-tree -r -z " + mergeID:                                                  sharedBlobs,
		"ls-tree -r -z " + retroID:                                                  sharedBlobs,
		"show -s --format=%(trailers:only,unfold) " + renameID:                      "",
		"show -s --format=%(trailers:only,unfold) " + mergeID:                       "",
		"show -s --format=%(trailers:only,unfold) " + retroID:                       "Rule-change-approves: " + renameID,
	}
}

func auditHistory() string {
	return fmt.Sprintf("%s %s\n%s %s %s\n%s %s\n", renameID, baseID, mergeID, renameID, baseID, retroID, mergeID)
}

func diffCommand(commit string) string {
	return "diff-tree --no-commit-id -r -m --root -M --name-status -z " + commit
}

func fakeHistory(t *testing.T, commands map[string]string) history {
	t.Helper()
	return history{
		command: func(args ...string) ([]byte, error) {
			command := strings.Join(args, " ")
			output, recorded := commands[command]
			if !recorded {
				return nil, errors.New("unexpected Git command: " + command)
			}
			return []byte(output), nil
		},
		blobs: func(id string) ([]byte, error) {
			if id != "machine" {
				return nil, fmt.Errorf("unexpected blob %q", id)
			}
			return []byte(`package test; import "github.com/pantacor/lawgate/machine"; var declared = machine.Machine{}`), nil
		},
	}
}

func assertAuditText(t *testing.T, audit string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(audit, fragment) {
			t.Errorf("audit missing %q:\n%s", fragment, audit)
		}
	}
}

func TestCIUsesVersionedExternalVerifier(t *testing.T) {
	command := regexp.MustCompile(`go install ["']?github\.com/pantacor/lawgate/cmd/lawgate@(v[0-9]+\.[0-9]+\.[0-9]+|\$LAW_GATE_VERSION["'])`)
	for _, name := range []string{"github-actions.yml", "gitlab-ci.yml"} {
		data, err := os.ReadFile("../templates/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !command.Match(data) {
			t.Errorf("%s does not install the pinned verifier", name)
		}
	}
}
