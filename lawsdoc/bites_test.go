package lawsdoc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceRejectsSelfCitationsAndGoManifests(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"rules.md", "other.md", "go.mod"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	in := Inputs{RepoRoot: root, DocPath: "rules.md"}
	sources := []string{"rules.md", "see `./rules.md#scope`", "go.mod", "`go.mod` (agreed 2026-11-20)"}
	for _, source := range sources {
		if err := checkSource(source, in); err == nil {
			t.Errorf("accepted Source %q", source)
		}
	}
	if err := checkSource("other.md", in); err != nil {
		t.Fatal(err)
	}
}

func TestBiteGapRequiresAnExplicitReason(t *testing.T) {
	doc := section("first-law", "machine `worker`", agreed)
	requireProblem(t, doc+"- Bites: not yet.\n", machineInputs(), "must say why")
	requireProblem(t, doc+"- Bites: passing\n", machineInputs(), "must be")
	requireProblem(t, doc+"- Bites: not yet. First.\n- Bites: not yet. Second.\n", machineInputs(), "must be")
}

func TestAgreedStatusRequiresACalendarDate(t *testing.T) {
	for _, date := range []string{"2026-13-45", "2026-02-29", "2026-04-31"} {
		doc := section("first-law", "machine `worker`", "agreed "+date)
		requireProblem(t, doc, machineInputs(), "valid calendar date")
	}
	doc := section("first-law", "machine `worker`", "agreed 2024-02-29")
	if got := checkText(doc, machineInputs()); got != "" {
		t.Fatalf("leap day: %s", got)
	}
}

func TestDatedSourcesRequireACalendarDate(t *testing.T) {
	in := Inputs{RepoRoot: fixtureRoot, DocPath: "nested/LAWS.md"}
	sources := []string{
		"owner decision 2026-02-29",
		"`../LAWS.md` (agreed 2026-13-45)",
		"https://example.test/plan; owner decision 2026-02-29",
		"owner decision 2026-11-20; owner decision 2026-02-29",
	}
	for _, source := range sources {
		if err := checkSource(source, in); err == nil {
			t.Errorf("accepted Source %q", source)
		}
	}
}

func TestBiteGapRequiresTestOrScript(t *testing.T) {
	doc := section("first-law", "machine `worker`", agreed)
	requireProblem(t, doc+"- Bites: not yet. no mutation yet\n", machineInputs(), "no test or script")
}

func TestGapFieldsUseSameSpelling(t *testing.T) {
	input := machineInputs()
	for _, value := range []string{"not yet: later", "not yet, later", "not yet.later"} {
		requireProblem(t, section("first-law", value, agreed), input, "not yet. <why>")
		doc := section("first-law", "script `rule.sh`", agreed)
		requireProblem(t, doc+"- Bites: "+value+"\n", input, "not yet. <why>")
	}
}
