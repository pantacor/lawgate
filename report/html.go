package report

import (
	"bytes"
	"html/template"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pantacor/lawgate/machine"
)

// The HTML report is one self-contained file, styled with the Pantacor
// design language, that a reviewer opens from the job artifacts without a
// build step and without a network.

// Meta is what the page says about the run, beside the runs themselves.
type Meta struct {
	Product   string    // shown in the title, e.g. "Pantacor Fleet"
	Commit    string    // the commit the report describes; may be empty
	Reproduce string    // the command that reproduces the page; may be empty
	Generated time.Time // when the report was made; zero leaves it out
}

// htmlLaw is one law as the page shows it.
type htmlLaw struct {
	Name       string
	Doc        string
	State      string // "Holds", "Broken", "Pending"
	Tone       string // a status-pill tone
	Pending    string
	Violations []string
}

// htmlMachine is one machine's card.
type htmlMachine struct {
	Module     string
	Name       string
	Source     string
	Count      int
	GateChecks bool
	Broken     int
	Pending    int
	Failure    string
	Verdict    string // the pill's label
	Tone       string // the pill's tone
	Laws       []htmlLaw
	Attention  []htmlLaw   // broken or pending, shown open
	Holding    []htmlLaw   // laws that hold, folded away
	Prefix     string      // the module prefix cut from gate check names
	Groups     []htmlGroup // gate checks that hold, by package
}

// HTMLData is the page's model.
type HTMLData struct {
	Product      string
	Generated    string
	Commit       string
	Reproduce    string
	Machines     int
	Laws         int
	Checks       int
	Broken       int
	Pending      int
	Failed       int
	Headline     string
	Verdict      string
	Tone         string
	Cards        []htmlMachine
	Attention    []htmlAttention // every broken or pending item, worst first
	Reasons      []htmlReason    // the same items, by reason
	MachineCards []htmlMachine   // machine cards, worst first
	Gates        []htmlMachine   // gate check cards
}

// pageVerdict is how the page states an outcome.
type pageVerdict struct {
	pill, headline, tone string
}

func verdictFor(outcome Outcome) pageVerdict {
	switch outcome {
	case OutcomeFailed:
		return pageVerdict{"✗ Not checked", "Some laws could not be checked. The gate refuses this change.", "danger"}
	case OutcomeBroken:
		return pageVerdict{"✗ Broken", "A law broke. The gate refuses this change.", "danger"}
	case OutcomePending:
		return pageVerdict{"… Pending", "Some laws are pending.", "warning"}
	}
	return pageVerdict{"✓ All hold", "Every law holds.", "success"}
}

// NewHTMLData turns the runs into the page's model.
func NewHTMLData(runs []LawRun, meta Meta) HTMLData {
	totals := Sum(runs)
	verdict := verdictFor(totals.Outcome())
	if totals.Checks > 0 {
		verdict = gateVerdict(totals.Outcome())
	}
	data := HTMLData{
		Product:   meta.Product,
		Commit:    meta.Commit,
		Reproduce: meta.Reproduce,
		Machines:  totals.Machines,
		Laws:      totals.Checked,
		Checks:    totals.Checks,
		Broken:    totals.Broken,
		Pending:   totals.Pending,
		Failed:    totals.Failed,
		Verdict:   verdict.pill,
		Headline:  verdict.headline,
		Tone:      verdict.tone,
	}
	if !meta.Generated.IsZero() {
		data.Generated = meta.Generated.Format("2 Jan 2006, 15:04 MST")
	}
	for _, run := range runs {
		card := htmlCard(run)
		data.Cards = append(data.Cards, card)
		data.Attention = append(data.Attention, attentionOf(card)...)
		if card.GateChecks {
			data.Gates = append(data.Gates, card)
			continue
		}
		data.MachineCards = append(data.MachineCards, card)
	}
	sortAttention(data.Attention)
	data.Reasons = groupAttention(data.Attention)
	sortCards(data.MachineCards)
	return data
}

func gateVerdict(outcome Outcome) pageVerdict {
	switch outcome {
	case OutcomeBroken:
		return pageVerdict{"✗ Broken", "A check failed. The gate refuses this change.", "danger"}
	case OutcomePending:
		return pageVerdict{"… Pending", "Some checks are pending.", "warning"}
	case OutcomeHolds:
		return pageVerdict{"✓ Passed", "All checks passed.", "success"}
	default:
		return pageVerdict{"✗ Not checked", "Some checks could not finish. The gate refuses this change.", "danger"}
	}
}

func htmlCard(run LawRun) htmlMachine {
	card := htmlMachine{Module: run.Module, Name: run.Machine, Source: run.Source, Count: len(run.Verdicts),
		Broken: run.Broken(), Pending: run.Pending(), Failure: run.failure(), GateChecks: run.GateChecks}
	card.Verdict, card.Tone = cardVerdict(card)
	for _, verdict := range run.Verdicts {
		card.Laws = append(card.Laws, htmlLawFor(verdict))
	}
	card.Attention, card.Holding = splitLaws(card.Laws)
	if card.GateChecks {
		card.Prefix, card.Groups = groupChecks(card.Holding)
		card.Attention = trimPrefix(card.Prefix, card.Attention)
	}
	return card
}

func htmlLawFor(verdict machine.Verdict) htmlLaw {
	law := htmlLaw{Name: verdict.Law.Name, Doc: verdict.Law.Doc, State: "Holds", Tone: "success", Violations: visibleViolations(verdict)}
	switch {
	case verdict.Broken():
		law.State, law.Tone = "Broken", "danger"
	case verdict.IsPending():
		law.State, law.Tone = "Pending", "info"
		law.Pending = verdict.Law.Pending
	}
	return law
}

func cardVerdict(card htmlMachine) (label, tone string) {
	switch {
	case card.Failure != "":
		return "Not checked", "danger"
	case card.Broken > 0:
		return strconv.Itoa(card.Broken) + " broken", "danger"
	case card.Pending > 0:
		return strconv.Itoa(card.Pending) + " pending", "info"
	}
	return "All hold", "success"
}

// HTML renders the page from the model.
func HTML(data HTMLData) ([]byte, error) {
	var buf bytes.Buffer
	if err := htmlTemplate.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteHTML stores the page.
func WriteHTML(path string, data HTMLData) error {
	body, err := HTML(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

var htmlTemplate = template.Must(template.New("laws").Funcs(template.FuncMap{
	"join": strings.Join,
}).Parse(strings.Replace(htmlPage, "/*css*/", htmlCSS, 1)))
