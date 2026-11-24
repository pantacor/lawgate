// Package render shows a declared machine to people and agents: Text is the
// terminal outline, Brief the one-state brief an agent needs to change a
// state, and Summary the one-line count of sites. The machine runner's
// -color, -brief and outline output come from here (package runner).
//
// Run package resolve first, so each site carries its source line.
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// Style says whether Text colours its outline.
type Style int

const (
	// Plain writes no escape codes: for files, pipes and tests.
	Plain Style = iota
	// ANSI colours the outline for a terminal.
	ANSI
)

const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiBold   = "\x1b[1m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiSalmon = "\x1b[38;5;209m"
)

// CarriesOn is how the outline, the brief and the machine index name the
// target of a catch that leaves the pass where it is (machine.Catch).
const CarriesOn = "carries on"

// doneStateID is the terminal state the outline paints green, the others
// red. It is only a colour: both adopting products name their success
// state "done", and nothing else in lawgate reads it.
const doneStateID = "done"

var siteGlyph = map[machine.SiteKind]string{
	machine.SiteFault:  "⚡",
	machine.SiteFail:   "✗",
	machine.SiteWrite:  "✎",
	machine.SiteSilent: "∅",
	machine.SiteWarn:   "⚠",
	machine.SiteLog:    "·",
	machine.SiteGuard:  "◦",
}

var siteColor = map[machine.SiteKind]string{
	machine.SiteFail:   ansiRed,
	machine.SiteFault:  ansiYellow,
	machine.SiteSilent: ansiRed + ansiBold,
	machine.SiteWrite:  ansiBlue,
	machine.SiteWarn:   ansiYellow,
}

var edgeGlyph = map[machine.EdgeKind]string{
	machine.EdgeNext: "→",
	machine.EdgeLoop: "↺",
	machine.EdgeExit: "⇥",
}

type painter struct{ style Style }

func (p painter) paint(code, text string) string {
	if p.style != ANSI {
		return text
	}
	return code + text + ansiReset
}

// Text writes the machine as a terminal outline: its labels and catches on
// top, then each state with its marks, its steps, the sites under each step
// and the edges out of it, and last the findings.
func Text(w io.Writer, m *machine.Machine, style Style) {
	paint := painter{style: style}
	out := outline{w: w, m: m, p: paint}
	out.header()
	for i := range m.States {
		out.state(&m.States[i])
	}
	out.findings()
}

type outline struct {
	w io.Writer
	m *machine.Machine
	p painter
}

func (o outline) header() {
	fmt.Fprintf(o.w, "%s  %s\n", o.p.paint(ansiBold, o.m.Name), o.p.paint(ansiDim, o.m.Source))
	if len(o.m.Labels) > 0 {
		fmt.Fprintf(o.w, "%s %s\n", o.p.paint(ansiDim, "labels"), strings.Join(o.m.Labels, ", "))
	}
	for _, catch := range o.m.Catches {
		o.catch(catch)
	}
	fmt.Fprintln(o.w)
}

// catch writes one catch rule; one that carries on is drawn as a loop back
// to where the pass is.
func (o outline) catch(catch machine.Catch) {
	note := catch.Note + limitNote(catch.Limit)
	glyph, target := o.p.paint(ansiYellow, "⇥"), o.p.paint(ansiSalmon, catch.To)
	if catch.CarriesOn() {
		glyph, target = o.p.paint(ansiYellow, "↺"), o.p.paint(ansiDim, CarriesOn)
	}
	fmt.Fprintf(o.w, "%s %-7s %s %-22s %s\n", o.p.paint(ansiDim, "catch"), catch.On, glyph, target, o.p.paint(ansiDim, note))
}

func (o outline) state(st *machine.State) {
	fmt.Fprintln(o.w, o.stateHead(st))
	o.labelled("when", st.When)
	if endsPass(st) {
		o.labelled("", st.Doc)
		o.labelled("woken by", st.WokenBy)
	}
	for _, step := range st.Steps {
		o.step(step)
	}
	o.edges(st)
	fmt.Fprintln(o.w)
}

func (o outline) stateHead(st *machine.State) string {
	head := o.p.paint(ansiSalmon, st.ID)
	for _, mark := range stateMarks(o.m, st) {
		head += o.p.paint(ansiDim, " · "+mark)
	}
	if st.Status != "" {
		head += "  " + o.p.paint(ansiCyan, "■ "+st.Status)
	}
	return head
}

// stateMarks are the facts about a state a reader needs beside its name:
// where passes start, and whether it ends the job or parks it.
func stateMarks(m *machine.Machine, st *machine.State) []string {
	var marks []string
	if st.ID == m.EntryID() {
		marks = append(marks, "entry")
	}
	if st.PassEntry {
		marks = append(marks, "pass entry")
	}
	switch {
	case st.Terminal:
		marks = append(marks, "end")
	case st.Wait:
		marks = append(marks, "park")
	}
	return marks
}

func endsPass(st *machine.State) bool { return st.Terminal || st.Wait }

// labelled writes one indented line, its label dimmed; nothing when text is
// empty. Without a label the whole line is dimmed.
func (o outline) labelled(label, text string) {
	if text == "" {
		return
	}
	if label == "" {
		fmt.Fprintf(o.w, "  %s\n", o.p.paint(ansiDim, text))
		return
	}
	fmt.Fprintf(o.w, "  %s %s\n", o.p.paint(ansiDim, label), text)
}

func (o outline) step(step machine.Step) {
	fmt.Fprintf(o.w, "  %-28s %s\n", step.ID, o.p.paint(ansiDim, string(step.Kind)))
	for _, site := range step.Sites {
		fmt.Fprintf(o.w, "    %s %s\n", o.siteLabel(site), o.p.paint(ansiDim, siteNote(site)))
	}
}

func (o outline) siteLabel(site machine.Site) string {
	code, known := siteColor[site.Kind]
	if !known {
		code = ansiDim
	}
	place := siteName(site) + lineNote(":", site.Line)
	return o.p.paint(code, fmt.Sprintf("%s %-6s %-13s", siteGlyph[site.Kind], site.Kind, place))
}

func (o outline) edges(st *machine.State) {
	if len(st.Edges) == 0 {
		return
	}
	fmt.Fprintf(o.w, "  %s\n", o.p.paint(ansiDim, "on outcome"))
	last := len(st.Edges) - 1
	for i, edge := range st.Edges {
		branch := "├"
		if i == last {
			branch = "└"
		}
		fmt.Fprintf(o.w, "  %s %s %s %s%s\n", o.p.paint(ansiDim, branch), edge.On,
			o.p.paint(ansiYellow, edgeGlyph[edge.Kind]), o.target(edge.To), o.p.paint(ansiDim, edgeNote(edge)))
	}
}

func (o outline) target(id string) string {
	st := o.m.State(id)
	switch {
	case st == nil || !st.Terminal:
		return o.p.paint(ansiSalmon, id)
	case id == doneStateID:
		return o.p.paint(ansiGreen, id)
	}
	return o.p.paint(ansiRed, id)
}

func (o outline) findings() {
	if len(o.m.Findings) == 0 {
		return
	}
	fmt.Fprintln(o.w, o.p.paint(ansiBold, "findings"))
	for _, finding := range o.m.Findings {
		place := o.m.Source + lineNote(":", finding.Line)
		fmt.Fprintf(o.w, "  %s %s %s\n", o.p.paint(ansiRed, finding.Severity), place, finding.Text)
	}
}

// siteName is how a site is called: its tag, else "untagged".
func siteName(site machine.Site) string {
	if site.Tag == "" {
		return "untagged"
	}
	return site.Tag
}

// siteNote is a site's effect and its labels.
func siteNote(site machine.Site) string {
	if len(site.Labels) == 0 {
		return site.Effect
	}
	return strings.TrimSpace(site.Effect + " [" + strings.Join(site.Labels, ", ") + "]")
}

// edgeNote is an edge's guard and limit, as the outline and the brief show
// them after its target.
func edgeNote(edge machine.Edge) string {
	note := limitNote(edge.Limit)
	if edge.Guard != "" {
		note = " when " + edge.Guard + note
	}
	return note
}

func limitNote(limit int) string {
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf(" (limit %d)", limit)
}

func lineNote(prefix string, line int) string {
	if line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s%d", prefix, line)
}
