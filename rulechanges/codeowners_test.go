package rulechanges

import (
	"bytes"
	"os"
	"testing"

	"github.com/pantacor/lawgate/inventory"
)

const requiredOwner = "@highercomve"

// templateOwner is the placeholder the templates ship: no real owner is built in.
const templateOwner = "@your-org/law-owners"

func TestGitHubPrecedenceCanRemoveCoverage(t *testing.T) {
	content := "* @highercomve\n/worker.go @someone-else\n/other.go @highercomve @someone-else\n/clear.go\n"
	sections := ownerPolicy(t, "github", content)
	assertCoverage(t, sections, "LAWS.md", coverageRequired)
	assertCoverage(t, sections, "worker.go", coverageMissing)
	assertCoverage(t, sections, "other.go", coverageMissing)
	assertCoverage(t, sections, "clear.go", coverageMissing)
}

func TestGitLabOptionalSectionCannotRequireOwner(t *testing.T) {
	content := "* @other\n^[Optional]\n* @highercomve\n[Required] @highercomve\n/.gitlab/**\n"
	sections := ownerPolicy(t, "gitlab", content)
	assertCoverage(t, sections, "LAWS.md", coverageMissing)
	assertCoverage(t, sections, ".gitlab/child/check.yml", coverageRequired)
}

func TestGeneratedOwnersCoverExactPaths(t *testing.T) {
	files := []string{"lawgate.json", "module/proof_test.go", "module/LAWS.md"}
	var block bytes.Buffer
	if err := WriteOwners(&block, files, requiredOwner); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"github", "gitlab"} {
		sections := ownerPolicy(t, platform, block.String())
		for _, file := range files {
			assertCoverage(t, sections, file, coverageRequired)
		}
		assertCoverage(t, sections, "other/LAWS.md", coverageMissing)
	}
}

func TestTemplatesRequireOwnerForNewRules(t *testing.T) {
	for _, platform := range []string{"github", "gitlab"} {
		content, err := os.ReadFile("../templates/" + platform + ".CODEOWNERS")
		if err != nil {
			t.Fatal(err)
		}
		sections := ownerPolicy(t, platform, string(content))
		if !sections.require("new/deep/place/guard.go", templateOwner) {
			t.Errorf("%s template does not cover a new rule file", platform)
		}
	}
}

func ownerPolicy(t *testing.T, platform, content string) ownerSections {
	t.Helper()
	sections, err := parseOwners(content, platform)
	if err != nil {
		t.Fatal(err)
	}
	return sections
}

type coverage string

const (
	coverageRequired coverage = "required"
	coverageMissing  coverage = "missing"
)

func assertCoverage(t *testing.T, sections ownerSections, name string, expected coverage) {
	t.Helper()
	if sections.require(name, requiredOwner) != (expected == coverageRequired) {
		t.Errorf("coverage for %q should be %s", name, expected)
	}
}

func TestExampleConfigDecodes(t *testing.T) {
	data, err := os.ReadFile("../templates/lawgate.example.json")
	if err != nil {
		t.Fatal(err)
	}
	config, err := inventory.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.RulePackages) != 2 || config.Owner != templateOwner {
		t.Fatalf("example lost its rule packages or owner: %+v", config)
	}
}
