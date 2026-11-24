package lsp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/review"
)

type lensCommand struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

type codeLens struct {
	Range   lspRange     `json:"range"`
	Command *lensCommand `json:"command,omitempty"`
}

func lens(line int, title, command string, args ...any) codeLens {
	target := &lensCommand{Title: title, Command: command, Arguments: args}
	return codeLens{Range: lineRange(line), Command: target}
}

func (s *server) codeLenses(params json.RawMessage) (any, error) {
	var p documentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	file, inside := s.rel(p.TextDocument.URI)
	lenses := []codeLens{}
	if !inside {
		return lenses, nil
	}
	idx := s.index()
	if strings.HasSuffix(file, "LAWS.md") {
		return lawLenses(idx, file), nil
	}
	lenses = append(lenses, stateLenses(idx, file)...)
	if s.siteLenses {
		lenses = append(lenses, siteLenses(idx, file)...)
	}
	lenses = append(lenses, refLenses(idx, idx.SubjectsIn(file, 1, 1<<30), "🛡 protected by law", clientShowSubject)...)
	lenses = append(lenses, refLenses(idx, idx.TestsIn(file, 1, 1<<30), "🧪 proves law", clientShowTest)...)
	return lenses, nil
}

// lawLenses put each law's verdict on its LAWS.md heading.
func lawLenses(idx *review.Index, file string) []codeLens {
	lenses := []codeLens{}
	for _, law := range idx.LawsIn(file, 1, 1<<30) {
		lenses = append(lenses, lens(law.Line, lawLensTitle(law), clientShowLaw, law.Key))
	}
	return lenses
}

func lawLensTitle(law *review.Law) string {
	outcome := law.Outcome()
	title := review.StatusIcon(outcome) + " proved by tests or scripts"
	if len(law.Verdicts) > 0 {
		machines := make([]string, 0, len(law.Verdicts))
		for _, verdict := range law.Verdicts {
			machines = append(machines, verdict.Machine)
		}
		title = fmt.Sprintf("%s %s on %s", review.StatusIcon(outcome), outcome, strings.Join(machines, ", "))
	}
	return title + refCounts(law)
}

func refCounts(law *review.Law) string {
	counts := ""
	if n := len(law.Subjects); n > 0 {
		counts += fmt.Sprintf(" · 🛡 %d %s", n, plural(n, "subject"))
	}
	if n := len(law.Tests); n > 0 {
		counts += fmt.Sprintf(" · 🧪 %d %s", n, plural(n, "test"))
	}
	return counts
}

func plural(count int, noun string) string {
	if count == 1 {
		return noun
	}
	return noun + "s"
}

func stateLenses(idx *review.Index, file string) []codeLens {
	var lenses []codeLens
	for _, state := range idx.States {
		if state.File != file || state.StartLine == 0 {
			continue
		}
		sites := stateSites(idx, state)
		title := fmt.Sprintf("⚙ state %s/%s · %d %s", state.Machine, state.ID, sites, plural(sites, "site"))
		if len(state.Laws) > 0 {
			title += " · " + lawSummary(idx, state.Laws)
		}
		lenses = append(lenses, lens(state.StartLine, title, clientShowState, state.Machine, state.ID))
	}
	return lenses
}

// stateSites counts the sites the state declares (several states may share
// one function).
func stateSites(idx *review.Index, state review.State) int {
	count := 0
	for _, site := range idx.Sites {
		if site.Machine == state.Machine && site.State == state.ID {
			count++
		}
	}
	return count
}

// siteLenses put one lens on each line that holds a site or a site's
// declaration.
func siteLenses(idx *review.Index, file string) []codeLens {
	byLine := map[int]review.Site{}
	for _, site := range idx.Sites {
		if site.File == file {
			addFirst(byLine, site.Line, site)
		}
		if site.DeclFile == file {
			addFirst(byLine, site.DeclLine, site)
		}
	}
	lines := make([]int, 0, len(byLine))
	for line := range byLine {
		lines = append(lines, line)
	}
	slices.Sort(lines)
	var lenses []codeLens
	for _, line := range lines {
		lenses = append(lenses, siteLens(idx, line, byLine[line]))
	}
	return lenses
}

// addFirst keeps the first site found for a line.
func addFirst(byLine map[int]review.Site, line int, site review.Site) {
	if _, taken := byLine[line]; !taken {
		byLine[line] = site
	}
}

func siteLens(idx *review.Index, line int, site review.Site) codeLens {
	name := siteLabel(site)
	title := fmt.Sprintf("◆ %s %s · %s/%s", site.Kind, name, site.State, site.Step)
	if len(site.Laws) > 0 {
		title += " · " + lawSummary(idx, site.Laws)
	}
	return lens(line, title, clientShowSite, site.Machine, site.State, site.Step, name)
}

// refLenses put "protected by law X" on a subject and "proves law X" on a
// test.
func refLenses(idx *review.Index, refs []review.CodeRef, role, command string) []codeLens {
	var lenses []codeLens
	for _, ref := range refs {
		law := idx.Laws[ref.Law]
		if law == nil {
			continue
		}
		title := fmt.Sprintf("%s %s %s%s", review.StatusIcon(law.Outcome()), role, law.Name, refCounts(law))
		lenses = append(lenses, lens(ref.Line, title, command, law.Key))
	}
	return lenses
}

func lawSummary(idx *review.Index, keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		outcome := review.OutcomeUnknown
		name := key
		if law := idx.Laws[key]; law != nil {
			outcome, name = law.Outcome(), law.Name
		}
		parts = append(parts, review.StatusIcon(outcome)+" "+name)
	}
	return strings.Join(parts, " ")
}
