package lawsdoc

import (
	"strings"
	"testing"
)

const agreed = "agreed 2026-11-20"

func machineInputs() Inputs {
	laws := []string{"first-law"}
	machines := map[string][]string{"worker": laws}
	return Inputs{Required: machines, Machines: machines}
}

func TestCheckAcceptsAMachineEnforcedLaw(t *testing.T) {
	doc := section("first-law", "machine `worker`", agreed)
	if got := checkText(doc, machineInputs()); got != "" {
		t.Fatalf("want no problem, got:\n%s", got)
	}
}

// TestCheckReportsEachProblem enforces laws-have-a-source: a law with no
// source, or an enforcer that does not exist, is a problem.
func TestCheckReportsEachProblem(t *testing.T) {
	in := machineInputs()
	requireProblem(t, "", in, `law first-law (machine worker) has no "## first-law" section`)
	requireProblem(t, section("first-law", "machine `ghost`", agreed), in, "which is not a registered machine")
	requireProblem(t, section("other-law", "machine `worker`", agreed), in, "which has no law of that name")
	requireProblem(t, section("first-law", "not yet.", agreed), in, "must say why")
	requireProblem(t, section("first-law", "trust me", agreed), in, "names no machine")
	requireProblem(t, section("first-law", "machine `worker`", "done"), in, "is neither \"agreed <YYYY-MM-DD>\" nor \"proposed\"")
	requireProblem(t, section("first-law", "proposed", agreed), in, "is agreed but nothing enforces it")
	twice := section("first-law", "machine `worker`", agreed) + section("first-law", "machine `worker`", agreed)
	requireProblem(t, twice, in, "has more than one section")
	noSource := "### first-law\n\n- Enforced by: machine `worker`\n- Status: " + agreed + "\n"
	requireProblem(t, noSource, in, `has no "- Source:" line`)
	requireSourceChecks(t, in)
}

// requireSourceChecks covers each way a Source is refused, and the cited
// forms it accepts, for laws-have-a-source.
func requireSourceChecks(t *testing.T, in Inputs) {
	t.Helper()
	refused := map[string]string{
		"my memory":                     "has invalid Source",
		"my memory (agreed 2026-11-20)": "has invalid Source",
		"https://example.test/plan (agreed 2026-02-30)": "has invalid Source",
		"owner decision 2026-02-30":                     "not a valid calendar date",
	}
	for source, want := range refused {
		requireProblem(t, withSource(source), in, want)
	}
	for _, source := range []string{"https://example.test/plan", "https://example.test/plan (agreed 2026-11-20)"} {
		if got := checkText(withSource(source), in); got != "" {
			t.Errorf("Source %q: want no problem, got:\n%s", source, got)
		}
	}
}

func withSource(source string) string {
	return "### first-law\n\n- Source: " + source + "\n- Enforced by: machine `worker`\n- Status: " + agreed + "\n"
}

func TestCheckAcceptsGapsAndProposals(t *testing.T) {
	in := machineInputs()
	gap := section("first-law", "not yet. Needs layer 3.", agreed)
	proposed := section("first-law", "proposed", "proposed")
	for _, doc := range []string{gap, proposed} {
		if got := checkText(doc, in); got != "" {
			t.Errorf("want no problem for:\n%s\ngot:\n%s", doc, got)
		}
	}
}

// A script enforcer was only checked when a repo root happened to be found;
// now a missing root is a problem, and so is a path out of the repo.
func TestCheckScriptsNeedARepoRoot(t *testing.T) {
	doc := section("first-law", "script `scripts/check.sh`", agreed)
	requireProblem(t, doc, Inputs{}, "cannot be checked without a repo root")
	escaping := section("first-law", "script `../../etc/passwd`", agreed)
	requireProblem(t, escaping, Inputs{RepoRoot: fixtureRoot}, "not a path inside the repo")
	missing := section("first-law", "script `scripts/gone.sh`", agreed)
	requireProblem(t, missing, Inputs{RepoRoot: fixtureRoot}, "does not exist")
}

func TestParseReadsSectionsFieldsAndFences(t *testing.T) {
	doc := strings.Join([]string{
		"# Title",
		"## A group",
		"### first-law",
		"- Source: a plan,",
		"  continued here",
		"```",
		"### fenced-law",
		"```",
		"## Another group",
		"- Source: belongs to no law",
		"## `second-law`",
	}, "\n")
	sections := Parse(doc)
	if names := strings.Join(Names(doc), ","); names != "first-law,second-law" {
		t.Fatalf("names %q", names)
	}
	if got := sections[0].Fields[FieldSource]; got != "a plan, continued here" {
		t.Fatalf("source %q", got)
	}
	if sections[1].Line != 11 {
		t.Fatalf("second-law at line %d, want 11", sections[1].Line)
	}
}

func TestSourceRequiresCitation(t *testing.T) {
	doc := section("first-law", "machine `worker`", agreed)
	requireProblem(t, strings.Replace(doc, "owner decision 2026-11-20", "trust me", 1), machineInputs(), "must cite")
}

func TestScriptPathAllowsAnInRepoParentReference(t *testing.T) {
	got, err := ScriptPath(fixtureRoot, "nested/LAWS.md", "../scripts/check.sh")
	if err != nil || !strings.HasSuffix(got, "scripts/check.sh") {
		t.Fatalf("script path = %q, %v", got, err)
	}
}

func TestSourceAcceptsPathURLAndOwnerDecision(t *testing.T) {
	in := machineInputs()
	in.RepoRoot = fixtureRoot
	sources := []string{
		"scripts/check.sh#scope, *Scope*",
		"`scripts/check.sh`, *Fixtures*",
		"[Design](https://example.test/plan#rules)",
		"owner decision 2026-11-20",
	}
	for _, source := range sources {
		doc := "## first-law\n- Source: " + source + "\n- Enforced by: machine `worker`\n- Status: " + agreed
		if got := checkText(doc, in); got != "" {
			t.Errorf("source %q: %s", source, got)
		}
	}
}

func TestSourceRejectsMissingPathsAndInvalidDates(t *testing.T) {
	in := machineInputs()
	in.RepoRoot = fixtureRoot
	for _, source := range []string{"`missing.md`", "owner decision on 2026-02-30", "`../../../LAWS.md`"} {
		doc := "## first-law\n- Source: " + source + "\n- Enforced by: machine `worker`\n- Status: " + agreed
		requireProblem(t, doc, in, "must cite")
	}
}

func TestSourceResolvesFromDocument(t *testing.T) {
	in := machineInputs()
	in.RepoRoot, in.DocPath = fixtureRoot, "nested/LAWS.md"
	if err := checkSource("`../LAWS.md#scope`", in); err != nil {
		t.Fatal(err)
	}
	if err := checkSource("`LAWS.md`", in); err == nil {
		t.Fatal("accepted a path relative to the repository root")
	}
}

func TestSourceRequiresAnExplicitDecision(t *testing.T) {
	for _, source := range []string{"trust me 2026-11-20", "(agreed 2026-11-20)", "trust me (agreed 2026-11-20)", "owner decision on 2026-11-20"} {
		if err := checkSource(source, Inputs{}); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
	if err := checkSource("`scripts/check.sh`, Proof gate (agreed 2026-11-20)", Inputs{RepoRoot: fixtureRoot}); err != nil {
		t.Fatal(err)
	}
}

// TestDatedSourceRequiresAnExistingContainedPath enforces laws-have-a-source
// when the citation also records its agreement date.
func TestDatedSourceRequiresAnExistingContainedPath(t *testing.T) {
	in := Inputs{RepoRoot: fixtureRoot, DocPath: "nested/LAWS.md"}
	for _, source := range []string{"`missing.md`", "`../../LAWS.md`"} {
		if err := checkSource(source+" (agreed 2026-11-20)", in); err == nil {
			t.Errorf("accepted dated source %s", source)
		}
	}
	if err := checkSource("`../LAWS.md` (agreed 2026-11-20)", in); err != nil {
		t.Fatal(err)
	}
}

// A law under the framework's prefix would be rerun with a selector that
// lawgate's own hooks answer to, so the document is refused and the law gets
// no enforcer to run.
func TestCheckRefusesALawUnderTheFrameworkPrefix(t *testing.T) {
	for _, heading := range []string{"lawgate/laws-bite", "`lawgate/control/no-such-law`", "lawgate/"} {
		doc := section(heading, "test `TestLaw`", agreed)
		requireProblem(t, doc, Inputs{}, `uses the reserved "lawgate/" prefix`)
		if kept := plannedEnforcers(doc); len(kept) != 0 {
			t.Errorf("heading %q: the reserved law keeps enforcers %v", heading, kept)
		}
	}
	machines := map[string][]string{"worker": {"lawgate-like"}}
	bare := section("lawgate-like", "machine `worker`", agreed)
	if got := checkText(bare, Inputs{Machines: machines}); got != "" {
		t.Fatalf("a bare name that only starts like the prefix must pass, got:\n%s", got)
	}
}

func plannedEnforcers(doc string) []Enforcer {
	var enforcers []Enforcer
	for _, plan := range Plan(doc) {
		enforcers = append(enforcers, plan.Enforcers...)
	}
	return enforcers
}
