package render

import (
	"fmt"
	"io"
	"slices"

	"github.com/pantacor/lawgate/machine"
)

// Brief writes, in Markdown, what an agent needs to change one state and
// nothing else: how the state is entered, its steps and the source lines
// their sites own, the edges and catches it must keep, the rules of the
// change and the known findings on its sites. It is the "send this state to
// an agent" view. It fails when the machine does not declare the state.
func Brief(w io.Writer, m *machine.Machine, state string) error {
	st := m.State(state)
	if st == nil {
		return fmt.Errorf("machine %s has no state %q", m.Name, state)
	}
	b := brief{w: w, m: m, st: st}
	b.intro()
	b.enteredFrom()
	b.steps()
	b.contract()
	b.rules()
	b.findings()
	return nil
}

type brief struct {
	w  io.Writer
	m  *machine.Machine
	st *machine.State
}

func (b brief) intro() {
	fmt.Fprintf(b.w, "# Change state %q of %s\n\n", b.st.ID, b.m.Name)
	if b.st.Func != "" {
		fmt.Fprintf(b.w, "Implemented by `%s` in `%s`.\n", b.st.Func, b.m.Source)
	} else {
		fmt.Fprintf(b.w, "Implemented in `%s`.\n", b.m.Source)
	}
	if b.st.When != "" {
		fmt.Fprintf(b.w, "Entered when: %s.\n", b.st.When)
	}
	if b.st.Status != "" {
		fmt.Fprintf(b.w, "The user sees the status `%s` while the job is here.\n", b.st.Status)
	}
}

func (b brief) enteredFrom() {
	fmt.Fprintf(b.w, "\n## Entered from\n")
	if b.st.ID == b.m.EntryID() {
		fmt.Fprintf(b.w, "- the start of every job (the entry state)\n")
	}
	if b.st.PassEntry {
		fmt.Fprintf(b.w, "- the start of a later pass (a pass entry)\n")
	}
	for _, other := range b.m.States {
		b.edgesInto(other)
		b.catchesInto(&other)
	}
}

func (b brief) edgesInto(from machine.State) {
	for _, edge := range from.Edges {
		if edge.To == b.st.ID {
			fmt.Fprintf(b.w, "- %s --%s--> %s\n", from.ID, edge.On, b.st.ID)
		}
	}
}

func (b brief) catchesInto(from *machine.State) {
	for _, catch := range b.m.Catches {
		caught := !catch.CarriesOn() && catch.To == b.st.ID && from.Holds(catch.On)
		if caught {
			fmt.Fprintf(b.w, "- %s --catch %s--> %s\n", from.ID, catch.On, b.st.ID)
		}
	}
}

func (b brief) steps() {
	fmt.Fprintf(b.w, "\n## Steps and the lines they own\n")
	for _, step := range b.st.Steps {
		fmt.Fprintf(b.w, "- **%s** (%s) %s\n", step.ID, step.Kind, step.Doc)
		for _, site := range step.Sites {
			fmt.Fprintf(b.w, "  - %s `%s` %s: %s\n", siteLine(site), briefSiteName(site), site.Kind, siteNote(site))
		}
	}
}

func siteLine(site machine.Site) string {
	if site.Line <= 0 {
		return "unresolved line"
	}
	return fmt.Sprintf("line %d", site.Line)
}

// briefSiteName names a site by its tag, or by its match when untagged:
// the agent looks for that text in the source.
func briefSiteName(site machine.Site) string {
	if site.Tag != "" {
		return site.Tag
	}
	return site.Match
}

func (b brief) contract() {
	fmt.Fprintf(b.w, "\n## Contract: these ways out must still exist after your change\n")
	for _, edge := range b.st.Edges {
		fmt.Fprintf(b.w, "- on %s %s %s%s\n", edge.On, edgeGlyph[edge.Kind], edge.To, edgeNote(edge))
	}
	for _, catch := range b.m.Catches {
		if b.st.Holds(catch.On) {
			b.caughtHere(catch)
		}
	}
}

func (b brief) caughtHere(catch machine.Catch) {
	if catch.CarriesOn() {
		fmt.Fprintf(b.w, "- a %s site %s: %s\n", catch.On, CarriesOn, catch.Note)
		return
	}
	fmt.Fprintf(b.w, "- a %s site is caught %s %s%s\n", catch.On, edgeGlyph[machine.EdgeExit], catch.To, limitNote(catch.Limit))
}

func (b brief) rules() {
	fmt.Fprintf(b.w, "\n## Rules\n")
	fmt.Fprintf(b.w, "- Touch only the lines listed above unless an edge has to move.\n")
	fmt.Fprintf(b.w, "- Every failure must reach the user: a failure with a message, never a silent log.\n")
	if b.m.Test != "" {
		fmt.Fprintf(b.w, "- If you add, remove or rename a log tag, update the machine declaration; `%s` must pass.\n", b.m.Test)
		return
	}
	fmt.Fprintf(b.w, "- If you add, remove or rename a log tag, update the machine declaration.\n")
}

func (b brief) findings() {
	related := b.relatedFindings()
	if len(related) == 0 {
		return
	}
	fmt.Fprintf(b.w, "\n## Known findings in this state\n")
	for _, text := range related {
		fmt.Fprintf(b.w, "- %s\n", text)
	}
}

// relatedFindings are the texts of the machine's findings on a site of the
// state (same line or same tag), each once.
func (b brief) relatedFindings() []string {
	var related []string
	for _, finding := range b.m.Findings {
		onState := b.stateHasFinding(finding)
		if onState && !slices.Contains(related, finding.Text) {
			related = append(related, finding.Text)
		}
	}
	return related
}

func (b brief) stateHasFinding(finding machine.Finding) bool {
	found := false
	b.m.Sites(func(st *machine.State, _ *machine.Step, site *machine.Site) {
		found = found || (st.ID == b.st.ID && findingOnSite(finding, site))
	})
	return found
}

func findingOnSite(finding machine.Finding, site *machine.Site) bool {
	sameLine := finding.Line > 0 && finding.Line == site.Line
	sameTag := finding.Tag != "" && finding.Tag == site.Tag
	return sameLine || sameTag
}
