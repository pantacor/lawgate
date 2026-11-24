package lsp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/pantacor/lawgate/review"
)

var lawName = regexp.MustCompile(`[a-z0-9]+(?:-[a-z0-9]+)+`)

// lawAt returns the known law whose name is under a position of a line of
// file.
func lawAt(idx *review.Index, file, line string, char16 int) *review.Law {
	column := byteColumn(line, char16)
	for _, at := range lawName.FindAllStringIndex(line, -1) {
		under := column >= at[0] && column <= at[1]
		if !under {
			continue
		}
		if law := idx.LawFor(line[at[0]:at[1]], file); law != nil {
			return law
		}
	}
	return nil
}

// byteColumn converts an LSP (UTF-16) character offset into a byte offset.
func byteColumn(line string, char16 int) int {
	units := 0
	for offset, r := range line {
		if units >= char16 {
			return offset
		}
		units += len(utf16.Encode([]rune{r}))
	}
	return len(line)
}

// hoverPlace is what a hover reads: the index, the file and its line.
type hoverPlace struct {
	idx  *review.Index
	file string
	line int // 1-based
	text string
}

func (s *server) place(p documentPosition) (hoverPlace, bool) {
	file, inside := s.rel(p.TextDocument.URI)
	if !inside {
		return hoverPlace{}, false
	}
	text := s.lineText(p.TextDocument.URI, p.Position.Line)
	return hoverPlace{idx: s.index(), file: file, line: p.Position.Line + 1, text: text}, true
}

func (s *server) hover(params json.RawMessage) (any, error) {
	var p documentPosition
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	at, inside := s.place(p)
	if !inside {
		return nil, nil
	}
	cards := s.lawCards(at, p.Position.Character)
	code := s.codeCards(at)
	if len(cards)+len(code) == 0 {
		return nil, nil
	}
	if len(code) > 0 {
		code = append(code, s.whyCard(at))
	}
	contents := map[string]any{"kind": "markdown", "value": strings.Join(append(cards, code...), "\n---\n\n")}
	return map[string]any{"contents": contents}, nil
}

// lawCards describe the law named under the cursor, or the law whose
// LAWS.md heading the cursor is on.
func (s *server) lawCards(at hoverPlace, char16 int) []string {
	if law := lawAt(at.idx, at.file, at.text, char16); law != nil {
		return []string{s.lawCard(law)}
	}
	var cards []string
	for _, law := range at.idx.LawsIn(at.file, at.line, at.line) {
		if law.Line == at.line {
			cards = append(cards, s.lawCard(law))
		}
	}
	return cards
}

// codeCards describe the sites, site declarations, states, subjects and
// tests declared on the line.
func (s *server) codeCards(at hoverPlace) []string {
	var cards []string
	for _, site := range at.idx.SitesAt(at.file, at.line) {
		cards = append(cards, s.siteCard(at.idx, site, site.DeclFile, site.DeclLine, "Declared"))
	}
	for _, site := range at.idx.DeclsAt(at.file, at.line) {
		cards = append(cards, s.siteCard(at.idx, site, site.File, site.Line, "Resolved"))
	}
	for _, state := range at.idx.States {
		if state.File == at.file && state.StartLine == at.line {
			cards = append(cards, stateCard(state))
		}
	}
	for _, ref := range refsStartingAt(at.idx.SubjectsIn(at.file, at.line, at.line), at.line) {
		cards = append(cards, s.refCard(at.idx, ref, "🛡 Protected by law"))
	}
	for _, ref := range refsStartingAt(at.idx.TestsIn(at.file, at.line, at.line), at.line) {
		cards = append(cards, s.refCard(at.idx, ref, "🧪 Proves law"))
	}
	return cards
}

// refsStartingAt keeps the subjects or tests whose function is declared on
// line: inside the function, gopls answers.
func refsStartingAt(refs []review.CodeRef, line int) []review.CodeRef {
	var starting []review.CodeRef
	for _, ref := range refs {
		if ref.Line == line {
			starting = append(starting, ref)
		}
	}
	return starting
}

func (s *server) whyCard(at hoverPlace) string {
	commit, err := review.Blame(at.idx.Root, at.file, at.line)
	if err != nil {
		return ""
	}
	return whyText(commit)
}

func (s *server) lawLink(law *review.Law) string {
	if law.File == "" {
		return "`" + law.Name + "`"
	}
	return fmt.Sprintf("[`%s`](%s#L%d)", law.Name, s.uri(law.File), law.Line)
}

func (s *server) lawCard(law *review.Law) string {
	header := fmt.Sprintf("### %s Law %s\n\n", review.StatusIcon(law.Outcome()), s.lawLink(law))
	return header + s.lawBody(law)
}

// lawBody is what a law card says below its title: the machine law's doc,
// the verdicts, subjects, tests and the law's text.
func (s *server) lawBody(law *review.Law) string {
	var b strings.Builder
	if law.CodeDoc != "" {
		fmt.Fprintf(&b, "%s\n\n", law.CodeDoc)
	}
	writeVerdicts(&b, law)
	s.writeRefs(&b, "Subject", law.Subjects)
	s.writeRefs(&b, "Proved by", law.Tests)
	if law.Text != "" {
		fmt.Fprintf(&b, "\n---\n\n%s\n", clip(law.Text, 14))
	}
	return b.String()
}

func writeVerdicts(b *strings.Builder, law *review.Law) {
	if len(law.Verdicts) == 0 && law.File != "" {
		fmt.Fprintf(b, "- ➖ enforced by %s (checked by `lawgate check`)\n", law.EnforcedBy)
	}
	for _, verdict := range law.Verdicts {
		fmt.Fprintf(b, "- %s %s on `%s`", review.StatusIcon(verdict.Status), verdict.Status, verdict.Machine)
		if verdict.Pending != "" {
			fmt.Fprintf(b, " — pending: %s", verdict.Pending)
		}
		b.WriteString("\n")
		for _, violation := range verdict.Violations {
			fmt.Fprintf(b, "  - %s\n", violation)
		}
	}
}

func (s *server) writeRefs(b *strings.Builder, label string, refs []review.CodeRef) {
	for _, ref := range refs {
		if ref.File == "" {
			fmt.Fprintf(b, "- %s `%s`: ❓ %s\n", label, ref.Name, ref.Problem)
			continue
		}
		fmt.Fprintf(b, "- %s [`%s`](%s#L%d)\n", label, ref.Name, s.uri(ref.File), ref.Line)
	}
}

func clip(text string, lines int) string {
	all := strings.Split(text, "\n")
	if len(all) <= lines {
		return text
	}
	return strings.Join(all[:lines], "\n") + "\n\n*…see LAWS.md*"
}

func (s *server) siteCard(idx *review.Index, site review.Site, file string, line int, label string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### Site `%s` · %s\n\n", siteLabel(site), site.Kind)
	fmt.Fprintf(&b, "Machine `%s` → state `%s` → step `%s`\n\n", site.Machine, site.State, site.Step)
	if site.Effect != "" {
		fmt.Fprintf(&b, "**Effect:** %s\n\n", site.Effect)
	}
	if site.StepDoc != "" {
		fmt.Fprintf(&b, "**Step:** %s\n\n", site.StepDoc)
	}
	if file != "" {
		fmt.Fprintf(&b, "%s at [%s:%d](%s#L%d)\n\n", label, file, line, s.uri(file), line)
	}
	for _, key := range site.Laws {
		b.WriteString(s.lawLine(idx, key))
	}
	return b.String()
}

func (s *server) lawLine(idx *review.Index, key string) string {
	law := idx.Laws[key]
	if law == nil {
		return fmt.Sprintf("- ❓ law `%s` is not written in any LAWS.md\n", key)
	}
	return fmt.Sprintf("- %s law %s\n", review.StatusIcon(law.Outcome()), s.lawLink(law))
}

func siteLabel(site review.Site) string {
	if site.Tag != "" {
		return site.Tag
	}
	return site.Step
}

func stateCard(state review.State) string {
	var b strings.Builder
	kind := ""
	if state.Terminal {
		kind = " · terminal"
	} else if state.Wait {
		kind = " · wait"
	}
	fmt.Fprintf(&b, "### State `%s/%s`%s\n\n", state.Machine, state.ID, kind)
	writeField(&b, "Status", state.Status)
	writeField(&b, "", state.Doc)
	writeField(&b, "When", state.When)
	writeField(&b, "Woken by", state.WokenBy)
	for _, edge := range state.Edges {
		fmt.Fprintf(&b, "- %s\n", edge)
	}
	return b.String()
}

func writeField(b *strings.Builder, label, value string) {
	switch {
	case value == "":
	case label == "":
		fmt.Fprintf(b, "%s\n\n", value)
	default:
		fmt.Fprintf(b, "**%s:** %s\n\n", label, value)
	}
}

func (s *server) refCard(idx *review.Index, ref review.CodeRef, role string) string {
	law := idx.Laws[ref.Law]
	if law == nil {
		return fmt.Sprintf("### %s `%s`\n", role, ref.Law)
	}
	header := fmt.Sprintf("### %s %s %s\n\n", review.StatusIcon(law.Outcome()), role, s.lawLink(law))
	return header + s.lawBody(law)
}

func whyText(commit *review.Commit) string {
	if commit.Uncommitted {
		return "**Why:** not committed yet\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Why this line changed:** `%s` %s — %s, %s\n", commit.Short, commit.Subject, commit.Author, commit.Date)
	if commit.Body != "" {
		paragraph, _, _ := strings.Cut(commit.Body, "\n\n")
		fmt.Fprintf(&b, "\n> %s\n", strings.ReplaceAll(paragraph, "\n", "\n> "))
	}
	if approvers := commit.Trailers[review.RuleChangeTrailer]; len(approvers) > 0 {
		fmt.Fprintf(&b, "\nRule change approved by %s\n", strings.Join(approvers, ", "))
	}
	return b.String()
}
