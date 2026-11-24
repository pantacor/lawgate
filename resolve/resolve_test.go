package resolve

import (
	"regexp"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

func testVocabulary() Vocabulary {
	calls := map[string]machine.SiteKind{
		"FailJob":    machine.SiteFail,
		"logFault":   machine.SiteFault,
		"StoreDone":  machine.SiteWrite,
		"log.Printf": machine.SiteSilent,
		"Info":       machine.SiteLog,
	}
	storeDone := []string{"StoreDone"}
	labels := map[string][]string{"stores-done": storeDone}
	pattern := regexp.MustCompile(`^(X_JOB(?: \d+)?)`)
	return Vocabulary{TagPattern: pattern, Calls: calls, LabelCalls: labels}
}

// oneSiteMachine declares a single site in state work, then done.
func oneSiteMachine(site machine.Site) *machine.Machine {
	sites := []machine.Site{site}
	step := machine.Step{ID: "do", Kind: "computer", Sites: sites}
	steps := []machine.Step{step}
	toDone := machine.Edge{On: "ok", To: "done", Kind: machine.EdgeNext}
	edges := []machine.Edge{toDone}
	work := machine.State{ID: "work", Steps: steps, Edges: edges}
	end := machine.State{ID: "done", Terminal: true}
	states := []machine.State{work, end}
	return &machine.Machine{Name: "x", States: states}
}

func tagSite(tag string, kind machine.SiteKind) machine.Site {
	return machine.Site{Tag: tag, Kind: kind, Effect: "told"}
}

func resolveText(m *machine.Machine, src string, vocab Vocabulary) string {
	var lines []string
	for _, err := range Resolve(m, []byte(src), vocab, "X_JOB") {
		lines = append(lines, err.Error())
	}
	return strings.Join(lines, "\n")
}

const failingSource = `package x

func work() {
	FailJob(
		"X_JOB 1: could not read",
		err)
}
`

func TestResolveBindsATagInsideAMultiLineCall(t *testing.T) {
	m := oneSiteMachine(tagSite("1", machine.SiteFail))
	if got := resolveText(m, failingSource, testVocabulary()); got != "" {
		t.Fatalf("want no drift, got:\n%s", got)
	}
	if line := m.States[0].Steps[0].Sites[0].Line; line != 5 {
		t.Fatalf("site line %d, want 5", line)
	}
}

// Regression: a tag left in a trailing or block comment, after
// the failure path was deleted, still satisfied the declared site.
func TestResolveIgnoresTagsInComments(t *testing.T) {
	sources := map[string]string{
		"trailing": "package x\n\nfunc work() {\n\t_ = 0 // was: FailJob(\"X_JOB 1\")\n}\n",
		"block":    "package x\n\nfunc work() {\n\t/* FailJob(\"X_JOB 1\") */\n}\n",
	}
	for name, src := range sources {
		m := oneSiteMachine(tagSite("1", machine.SiteFail))
		got := resolveText(m, src, testVocabulary())
		if !strings.Contains(got, `tag "1" (match "") is not in the code`) {
			t.Errorf("%s comment: want the site reported missing, got:\n%s", name, got)
		}
	}
}

// Regression: an untagged Match bound to any substring, comments
// included, without checking the kind or the labels.
func TestResolveMatchIgnoresComments(t *testing.T) {
	src := "package x\n\nfunc work() {\n\t// StoreDone(job)\n\treturn\n}\n"
	site := machine.Site{Match: "StoreDone(job)", Kind: machine.SiteWrite, Effect: "done"}
	got := resolveText(oneSiteMachine(site), src, testVocabulary())
	if !strings.Contains(got, `match "StoreDone(job)" is not in the code`) {
		t.Fatalf("a match only in a comment must not bind, got:\n%s", got)
	}
}

// Regression: a Match site labelled stores-done passed without a
// StoreDone call on its line.
func TestResolveChecksLabelsOnMatchSites(t *testing.T) {
	src := "package x\n\nfunc work() {\n\treturn\n}\n"
	labels := []string{"stores-done"}
	site := machine.Site{Match: "return", Kind: machine.SiteWrite, Effect: "done", Labels: labels}
	got := resolveText(oneSiteMachine(site), src, testVocabulary())
	if !strings.Contains(got, "labelled stores-done but calls none of StoreDone") {
		t.Fatalf("want the label refused, got:\n%s", got)
	}
}

func TestResolveChecksLabelsOnTagSites(t *testing.T) {
	site := tagSite("1", machine.SiteFail)
	site.Labels = []string{"stores-done"}
	got := resolveText(oneSiteMachine(site), failingSource, testVocabulary())
	if !strings.Contains(got, "labelled stores-done") {
		t.Fatalf("want the label refused, got:\n%s", got)
	}
}

// Regression: a guard was compatible with every detected kind, so
// a failure or a silent log could be declared a guard and drop out of the
// laws.
func TestResolveGuardCannotHideAFailure(t *testing.T) {
	silent := "package x\n\nfunc work() {\n\tlog.Printf(\"X_JOB 1: %v\", err)\n}\n"
	for name, src := range map[string]string{"fail": failingSource, "silent": silent} {
		got := resolveText(oneSiteMachine(tagSite("1", machine.SiteGuard)), src, testVocabulary())
		if !strings.Contains(got, "is declared guard") {
			t.Errorf("%s: a guard on a reporting call must be refused, got:\n%s", name, got)
		}
	}
}

func TestResolveSilentLogIsNotAFailSite(t *testing.T) {
	silent := "package x\n\nfunc work() {\n\tlog.Printf(\"X_JOB 1: %v\", err)\n}\n"
	got := resolveText(oneSiteMachine(tagSite("1", machine.SiteFail)), silent, testVocabulary())
	if !strings.Contains(got, "is declared fail but the code is silent") {
		t.Fatalf("a bare log does not tell the user, got:\n%s", got)
	}
	if got := resolveText(oneSiteMachine(tagSite("1", machine.SiteFault)), silent, testVocabulary()); got != "" {
		t.Fatalf("a log followed by a return is a fault, got:\n%s", got)
	}
}

// Regression: the call -> kind table is the product's, so the
// same code reads differently under another vocabulary.
func TestResolveTakesTheKindsFromTheVocabulary(t *testing.T) {
	vocab := testVocabulary()
	vocab.Calls = map[string]machine.SiteKind{"FailJob": machine.SiteWarn}
	got := resolveText(oneSiteMachine(tagSite("1", machine.SiteFail)), failingSource, vocab)
	if !strings.Contains(got, "is declared fail but the code is warn") {
		t.Fatalf("want the product's kind, got:\n%s", got)
	}
}

func TestResolveReportsUnclaimedTags(t *testing.T) {
	src := "package x\n\nfunc work() {\n\tFailJob(\"X_JOB 1\")\n\tFailJob(\"X_JOB 2\")\n}\n"
	got := resolveText(oneSiteMachine(tagSite("1", machine.SiteFail)), src, testVocabulary())
	if !strings.Contains(got, `line 5: tag "2" (fail) is not claimed by any site`) {
		t.Fatalf("want the second tag reported, got:\n%s", got)
	}
}

func TestResolveRunsTheGraphValidation(t *testing.T) {
	m := oneSiteMachine(tagSite("1", machine.SiteFail))
	catch := machine.Catch{On: machine.SiteFail, To: "nowhere"}
	m.Catches = []machine.Catch{catch}
	if !strings.Contains(resolveText(m, failingSource, testVocabulary()), "unknown state nowhere") {
		t.Fatal("Resolve must report graph problems too")
	}
}

func TestResolveNeedsATagPattern(t *testing.T) {
	m := oneSiteMachine(tagSite("1", machine.SiteFail))
	if errs := Resolve(m, []byte(failingSource), Vocabulary{}, "X_JOB"); len(errs) != 1 {
		t.Fatalf("want one error, got %v", errs)
	}
}

func TestResolveReportsUnparsableSource(t *testing.T) {
	got := resolveText(oneSiteMachine(tagSite("1", machine.SiteFail)), "package x\nfunc {", testVocabulary())
	if !strings.Contains(got, "resolve x:") {
		t.Fatalf("want a parse error, got:\n%s", got)
	}
}

// TestResolveSilentSiteCannotBeDeclaredLog covers this case.
func TestResolveSilentSiteCannotBeDeclaredLog(t *testing.T) {
	src := "package x\nfunc work() { log.Printf(\"X_JOB 1: %v\", err) }"
	m := oneSiteMachine(tagSite("1", machine.SiteLog))
	if got := resolveText(m, src, testVocabulary()); !strings.Contains(got, "is declared log but the code is silent") {
		t.Fatalf("want the silent failure reported, got %q", got)
	}
}

func TestResolveMatchUsesTheCallsAtItsSubstring(t *testing.T) {
	src := `package x
func work() {
	Info(log.Printf("nested needle"))
	log.Printf(
		"multiline needle",
	)
	Info("first needle"); log.Printf("second needle")
}`
	for _, match := range []string{"nested needle", "multiline needle", "second needle"} {
		site := machine.Site{Match: match, Kind: machine.SiteLog}
		got := resolveText(oneSiteMachine(site), src, testVocabulary())
		if !strings.Contains(got, "is declared log but the code is silent") {
			t.Errorf("match %q must bind to the silent call, got %q", match, got)
		}
		site.Kind = machine.SiteSilent
		if got := resolveText(oneSiteMachine(site), src, testVocabulary()); got != "" {
			t.Errorf("match %q with the silent kind reported drift %q", match, got)
		}
	}
	outer := machine.Site{Match: `Info(log.Printf("nested needle"))`, Kind: machine.SiteLog}
	if got := resolveText(oneSiteMachine(outer), src, testVocabulary()); got != "" {
		t.Errorf("match containing the whole outer call reported drift %q", got)
	}
}
