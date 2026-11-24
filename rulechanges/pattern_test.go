package rulechanges

import (
	"bytes"
	"testing"

	"github.com/pantacor/lawgate/templates"
)

type patternCase struct {
	pattern string
	file    string
	covered coverage
}

func patternCases() []patternCase {
	return []patternCase{
		{"/*", "LAWS.md", coverageRequired},
		{"/*", "pkg/sub/LAWS.md", coverageMissing},
		{"/*.go", "pkg/guard.go", coverageMissing},
		{"/docs/*", "docs/LAWS.md", coverageRequired},
		{"/docs/*", "docs/deep/LAWS.md", coverageMissing},
		{"*", "pkg/sub/LAWS.md", coverageRequired},
		{"*.go", "pkg/sub/guard.go", coverageRequired},
		{"/**", "pkg/sub/LAWS.md", coverageRequired},
		{"/docs/**", "docs/deep/LAWS.md", coverageRequired},
		{"**/LAWS.md", "pkg/sub/LAWS.md", coverageRequired},
		{"/a/**/LAWS.md", "a/LAWS.md", coverageRequired},
		{"/pkg**", "pkg/sub/LAWS.md", coverageMissing},
		{"/pkg/", "pkg/sub/LAWS.md", coverageRequired},
		{"/pkg", "pkg/sub/LAWS.md", coverageRequired},
	}
}

// TestPatternsFollowPlatformSemantics enforces rule-changes-are-reviewed:
// a pattern the platform reads narrowly must not count as coverage.
func TestPatternsFollowPlatformSemantics(t *testing.T) {
	for _, test := range patternCases() {
		assertPattern(t, "github", test)
		assertPattern(t, "gitlab", test)
	}
}

func assertPattern(t *testing.T, platform string, test patternCase) {
	t.Helper()
	sections := ownerPolicy(t, platform, test.pattern+" "+requiredOwner+"\n")
	if sections.require(test.file, requiredOwner) != (test.covered == coverageRequired) {
		t.Errorf("%s: %q on %q should be %s", platform, test.pattern, test.file, test.covered)
	}
}

func TestOwnerHandles(t *testing.T) {
	for _, owner := range []string{"@org/team", "@group/sub/team", "@john_doe", "@jane.doe", "@user"} {
		if err := ValidOwner(owner); err != nil {
			t.Errorf("%s rejected: %v", owner, err)
		}
	}
	for _, owner := range []string{"", "user", "@", "@a b", "@org/", "@a;rm"} {
		if ValidOwner(owner) == nil {
			t.Errorf("%q accepted", owner)
		}
	}
}

func TestTeamOwnerAndGitLabSections(t *testing.T) {
	const team = "@org/law-owners"
	content := "[Law gate][2] " + team + "\n/LAWS.md\n[law GATE]\n/guard.go " + team + "\n^[Docs] " + team + "\n/docs/*\n"
	sections := ownerPolicy(t, "gitlab", content)
	expectations := map[string]bool{"LAWS.md": true, "guard.go": true, "docs/x.md": false}
	for file, expected := range expectations {
		if sections.require(file, team) != expected {
			t.Errorf("gitlab coverage for %s should be %v", file, expected)
		}
	}
}

func TestFallbackCoversNewRuleFiles(t *testing.T) {
	for _, platform := range []string{"github", "gitlab"} {
		var owners bytes.Buffer
		owners.WriteString(templates.CodeownersFallback(platform, requiredOwner))
		if err := WriteOwners(&owners, []string{"LAWS.md"}, requiredOwner); err != nil {
			t.Fatal(err)
		}
		sections := ownerPolicy(t, platform, owners.String())
		assertCoverage(t, sections, "new/rule_test.go", coverageRequired)
	}
}
