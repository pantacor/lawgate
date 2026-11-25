package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pantacor/lawgate/machine"
)

func verdict(name string, violations ...string) machine.Verdict {
	law := machine.Law{Name: name, Doc: name + " doc"}
	return machine.Verdict{Law: law, Violations: violations}
}

func pendingVerdict(name, why string, violations ...string) machine.Verdict {
	v := verdict(name, violations...)
	v.Law.Pending = why
	return v
}

func lawRun(module, name string, verdicts ...machine.Verdict) LawRun {
	return LawRun{Module: module, Machine: name, Source: name + ".go", Verdicts: verdicts}
}

func holdingRun() LawRun { return lawRun("mod", "good", verdict("a"), verdict("b")) }

func mixedRuns() []LawRun {
	broken := lawRun("mod", "bad", verdict("a", "x is wrong", "y is wrong"), pendingVerdict("b", "owner", "z"))
	crashed := FailedRun("other", "cmd/machine exited 2 and wrote no report")
	return []LawRun{holdingRun(), broken, crashed}
}

// Regression: a module that crashed and wrote no report was left
// out, and the joined summary read green.
func TestAFailedModuleIsNeverGreen(t *testing.T) {
	runs := []LawRun{holdingRun(), FailedRun("crashed", "exit 2")}
	if outcome := Sum(runs).Outcome(); outcome != OutcomeFailed || !outcome.Refuses() {
		t.Fatalf("outcome %s, want failed", outcome)
	}
	markdown := Markdown(runs)
	if !strings.Contains(markdown, "**✗ Infrastructure failure") || strings.Contains(markdown, "**✓") {
		t.Fatalf("the summary must say it failed and never read green:\n%s", markdown)
	}
	page, err := HTML(NewHTMLData(runs, Meta{Product: "Test"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "Every law holds") || !strings.Contains(string(page), "exit 2") {
		t.Fatal("the page must name the failure and not say every law holds")
	}
}

// Regression: zero verdicts read as a pass.
func TestNothingCheckedIsAFailure(t *testing.T) {
	if Sum(nil).Outcome() != OutcomeFailed {
		t.Fatal("no run at all must be a failure")
	}
	empty := lawRun("mod", "empty")
	if Sum([]LawRun{empty}).Outcome() != OutcomeFailed {
		t.Fatal("a run that checked no law must be a failure")
	}
}

func TestOutcomes(t *testing.T) {
	pending := lawRun("mod", "p", pendingVerdict("a", "owner", "v"))
	broken := lawRun("mod", "b", verdict("a", "v"))
	checkOutcome(t, OutcomeHolds, holdingRun())
	checkOutcome(t, OutcomePending, holdingRun(), pending)
	checkOutcome(t, OutcomeBroken, pending, broken)
}

func checkOutcome(t *testing.T, want Outcome, runs ...LawRun) {
	t.Helper()
	if got := Sum(runs).Outcome(); got != want {
		t.Errorf("outcome %s, want %s", got, want)
	}
}

func TestJUnitRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "laws.xml")
	if err := WriteJUnit(path, mixedRuns()); err != nil {
		t.Fatal(err)
	}
	runs, err := ReadJUnit(path)
	if err != nil {
		t.Fatal(err)
	}
	want, got := Sum(mixedRuns()), Sum(runs)
	if got != want {
		t.Fatalf("totals %+v after the round trip, want %+v", got, want)
	}
	if runs[2].Failure != "cmd/machine exited 2 and wrote no report" {
		t.Fatalf("the failure reason must survive, got %q", runs[2].Failure)
	}
	if runs[1].Verdicts[1].Law.Pending != "owner" {
		t.Fatalf("the pending reason must survive, got %+v", runs[1].Verdicts[1])
	}
}

func TestReadJUnitRefusesAnEmptyReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.xml")
	if err := os.WriteFile(path, []byte(`<testsuites name="laws"></testsuites>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadJUnit(path); err == nil {
		t.Fatal("a report with no machine describes nothing and must be refused")
	}
}

func TestMarkdownNamesWhatBroke(t *testing.T) {
	markdown := Markdown(mixedRuns())
	for _, want := range []string{"x is wrong", "is pending: owner", "could not be checked: cmd/machine exited 2"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
}

// Regression: the page named one product in its title.
func TestHTMLNamesTheProductItIsGiven(t *testing.T) {
	meta := Meta{Product: "Example Product", Commit: "abc123", Reproduce: "make laws", Generated: time.Unix(0, 0).UTC()}
	page, err := HTML(NewHTMLData([]LawRun{holdingRun()}, meta))
	if err != nil {
		t.Fatal(err)
	}
	text := string(page)
	for _, want := range []string{"<title>Laws · Example Product</title>", "abc123", "make laws", "Every law holds."} {
		if !strings.Contains(text, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(text, "Pantavisor AI Stack") {
		t.Error("the core page must not name a product of its own")
	}
}

func TestMarkdownOmitsDanglingDetailsSeparator(t *testing.T) {
	empty := verdict("empty", "one violation")
	empty.Law.Doc = ""
	multiline := verdict("multiline", "another violation\nwith more detail")
	multiline.Law.Doc = "First line\nSecond line"
	run := lawRun("mod", "worker", empty, multiline)
	summary := Markdown([]LawRun{run})
	for _, want := range []string{
		"**`mod/worker` · `empty`**\n  - one violation",
		"**`mod/worker` · `multiline`**\n  - First line\n    Second line\n  - another violation\n    with more detail",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("markdown lacks %q:\n%s", want, summary)
		}
	}
}

func gateRun(verdicts ...machine.Verdict) LawRun {
	return LawRun{Module: "mod", Machine: "gate checks", GateChecks: true, Verdicts: verdicts}
}

// A green gate with recorded gaps has nothing under "What broke".
func TestPendingItemsHaveTheirOwnHeading(t *testing.T) {
	green := Markdown([]LawRun{gateRun(verdict("a"), pendingVerdict("gap-law", "needs a fixture", "needs a fixture"))})
	if strings.Contains(green, "What broke") {
		t.Fatalf("a green summary must not say something broke:\n%s", green)
	}
	if !strings.Contains(green, "### Pending\n\n- `mod/gate checks` · `gap-law` is pending: needs a fixture") {
		t.Fatalf("the pending item must be listed under Pending:\n%s", green)
	}
	mixed := Markdown(mixedRuns())
	broke, waiting, found := strings.Cut(mixed, "\n### Pending\n")
	failureUnderPending := strings.Contains(waiting, "x is wrong") || strings.Contains(waiting, "could not be checked")
	misplaced := strings.Contains(broke, "is pending") || failureUnderPending
	sectioned := found && strings.Contains(broke, "### What broke")
	if !sectioned || misplaced {
		t.Fatalf("failures belong under What broke and gaps under Pending:\n%s", mixed)
	}
}

func TestFailureCountAgreesInNumber(t *testing.T) {
	one := Markdown([]LawRun{gateRun(verdict("a", "x is wrong"))})
	two := Markdown([]LawRun{gateRun(verdict("a", "x is wrong"), verdict("b", "y is wrong"))})
	if !strings.Contains(one, "✗ 1 failure. ") || !strings.Contains(two, "✗ 2 failures. ") {
		t.Fatalf("headlines must agree in number:\n%s\n%s", one, two)
	}
}

// The page lists what needs attention first, broken before pending, and
// folds what holds.
func TestHTMLPutsWhatNeedsAttentionFirst(t *testing.T) {
	runs := []LawRun{
		holdingRun(),
		gateRun(pendingVerdict("gap-law", "needs a fixture", "needs a fixture")),
		lawRun("mod", "worker", verdict("broken-law", "x is wrong")),
	}
	data := NewHTMLData(runs, Meta{})
	var names []string
	for _, item := range data.Attention {
		names = append(names, item.Name)
	}
	if got := strings.Join(names, ","); got != "broken-law,gap-law" {
		t.Fatalf("attention must list the broken law, then the pending one, got %s", got)
	}
	if data.MachineCards[0].Name != "worker" {
		t.Fatalf("the machine with a broken law must come first, got %q", data.MachineCards[0].Name)
	}
	page, err := HTML(data)
	if err != nil {
		t.Fatal(err)
	}
	text := string(page)
	attention, machines := strings.Index(text, "Needs attention"), strings.Index(text, `aria-label="Machines"`)
	if attention < 0 || attention > machines {
		t.Fatalf("Needs attention must come before the machines")
	}
	if !strings.Contains(text, `<details class="mrow" data-verdict="ok"`) || strings.Contains(text, `data-verdict="ok" data-name="good" open`) {
		t.Fatalf("a machine whose laws hold must be a closed row")
	}
}

// Gate checks drop the prefix they share and group by package.
func TestGateChecksGroupByPackage(t *testing.T) {
	run := gateRun(
		verdict("example.com/app/modules/api/TestA"),
		verdict("example.com/app/modules/api/TestB/case_one"),
		verdict("example.com/app/modules/storage/TestC"),
		verdict("module inventory"),
		verdict("laws-bite (some-law: LAWS.md, subject)"),
	)
	card := NewHTMLData([]LawRun{run}, Meta{}).Gates[0]
	if card.Prefix != "example.com/app/modules/" {
		t.Fatalf("prefix = %q", card.Prefix)
	}
	got := map[string][]string{}
	for _, group := range card.Groups {
		for _, check := range group.Checks {
			got[group.Name] = append(got[group.Name], check.Name)
		}
	}
	want := map[string][]string{
		"api":        {"TestA", "TestB/case_one"},
		"storage":    {"TestC"},
		"repository": {"module inventory"},
		"laws-bite":  {"(some-law: LAWS.md, subject)"},
	}
	for name, checks := range want {
		if strings.Join(got[name], ",") != strings.Join(checks, ",") {
			t.Errorf("group %q = %v, want %v", name, got[name], checks)
		}
	}
}
