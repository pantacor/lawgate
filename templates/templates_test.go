package templates

import (
	"strings"
	"testing"
)

func TestGitHubActionsArePinned(t *testing.T) {
	want := map[string]int{
		"actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4":        2,
		"actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5":        2,
		"actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4":      1,
		"actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4": 1,
	}
	for line := range strings.SplitSeq(GitHub, "\n") {
		action, ok := strings.CutPrefix(strings.TrimSpace(line), "- uses: ")
		if !ok {
			continue
		}
		want[action]--
	}
	for action, remaining := range want {
		if remaining != 0 {
			t.Errorf("action %q: expected count minus actual count is %d", action, remaining)
		}
	}
}

func TestGitLabTemplateDocumentsNodeImage(t *testing.T) {
	if !strings.Contains(GitLab, "Node.js") {
		t.Fatal("GitLab CI template does not document the Node.js image requirement")
	}
}

func TestCodeownersFallbackUsesPlatformSection(t *testing.T) {
	const owner = "@group/law-owners"
	github := CodeownersFallback("github", owner)
	if strings.Contains(github, "[Law gate]") || !strings.HasSuffix(github, "* "+owner+"\n\n") {
		t.Errorf("unexpected GitHub fallback:\n%s", github)
	}
	gitlab := CodeownersFallback("gitlab", owner)
	if !strings.HasSuffix(gitlab, "[Law gate]\n* "+owner+"\n\n") {
		t.Errorf("unexpected GitLab fallback:\n%s", gitlab)
	}
}

func TestLawChecksReceiveBaseCommit(t *testing.T) {
	if !strings.Contains(GitHub, `check --base "$LAW_BASE"`) {
		t.Error("GitHub law checks do not replay the base proof")
	}
	if !strings.Contains(GitLab, `check --base "$CI_MERGE_REQUEST_TARGET_BRANCH_SHA"`) {
		t.Error("GitLab law checks do not replay the base proof")
	}
}
