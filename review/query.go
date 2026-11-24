package review

import (
	"cmp"
	"slices"
)

// SitesAt returns the sites resolved to a line of a file.
func (idx *Index) SitesAt(file string, line int) []Site {
	return idx.SitesIn(file, line, line)
}

// SitesIn returns the sites of a file between two lines, inclusive.
func (idx *Index) SitesIn(file string, from, to int) []Site {
	var sites []Site
	for _, site := range idx.Sites {
		if site.File == file && within(site.Line, from, to) {
			sites = append(sites, site)
		}
	}
	return sites
}

// DeclsAt returns the sites whose declaration is on a line of a file.
func (idx *Index) DeclsAt(file string, line int) []Site {
	return idx.DeclsIn(file, line, line)
}

// DeclsIn returns the sites declared between two lines of a file.
func (idx *Index) DeclsIn(file string, from, to int) []Site {
	var sites []Site
	for _, site := range idx.Sites {
		if site.DeclFile == file && within(site.DeclLine, from, to) {
			sites = append(sites, site)
		}
	}
	return sites
}

func within(line, from, to int) bool {
	return from <= line && line <= to
}

// StatesIn returns the states whose code overlaps a range of a file.
func (idx *Index) StatesIn(file string, from, to int) []State {
	var states []State
	for _, state := range idx.States {
		if state.covers(file, from, to) {
			states = append(states, state)
		}
	}
	return states
}

func (state State) covers(file string, from, to int) bool {
	for _, span := range state.Spans {
		if span.Overlaps(file, from, to) {
			return true
		}
	}
	return false
}

// LawsIn returns the laws whose LAWS.md section overlaps a range of a file,
// in the order they are written.
func (idx *Index) LawsIn(file string, from, to int) []*Law {
	var laws []*Law
	for _, law := range idx.Laws {
		written := law.File == file && law.Line > 0
		overlaps := law.Line <= to && law.EndLine >= from
		if written && overlaps {
			laws = append(laws, law)
		}
	}
	slices.SortFunc(laws, func(left, right *Law) int { return cmp.Compare(left.Line, right.Line) })
	return laws
}

// SubjectsIn returns the subjects whose function overlaps a range of a file.
func (idx *Index) SubjectsIn(file string, from, to int) []CodeRef {
	return idx.refsIn(file, from, to, func(law *Law) []CodeRef { return law.Subjects })
}

// TestsIn returns the enforcing tests whose function overlaps a range of a
// file.
func (idx *Index) TestsIn(file string, from, to int) []CodeRef {
	return idx.refsIn(file, from, to, func(law *Law) []CodeRef { return law.Tests })
}

func (idx *Index) refsIn(file string, from, to int, refs func(*Law) []CodeRef) []CodeRef {
	var found []CodeRef
	for _, law := range idx.Laws {
		for _, ref := range refs(law) {
			found = appendOverlapping(found, ref, file, from, to)
		}
	}
	slices.SortFunc(found, func(left, right CodeRef) int {
		return cmp.Or(cmp.Compare(left.Line, right.Line), cmp.Compare(left.Law, right.Law))
	})
	return found
}

func appendOverlapping(found []CodeRef, ref CodeRef, file string, from, to int) []CodeRef {
	overlaps := ref.File == file && ref.Line <= to && ref.EndLine >= from
	if overlaps && ref.Line > 0 {
		return append(found, ref)
	}
	return found
}

// Summary counts what the index holds, for a status line.
type Summary struct {
	Root    string `json:"root"`
	Sites   int    `json:"sites"`
	States  int    `json:"states"`
	Laws    int    `json:"laws"` // machine law verdicts
	Holds   int    `json:"holds"`
	Broken  int    `json:"broken"`
	Pending int    `json:"pending"`
	Drift   int    `json:"drift"` // problems
}

// Summarize counts the index.
func (idx *Index) Summarize() Summary {
	summary := Summary{Root: idx.Root, Sites: len(idx.Sites), States: len(idx.States), Drift: len(idx.Problems)}
	for _, law := range idx.Laws {
		for _, verdict := range law.Verdicts {
			summary.count(verdict.Status)
		}
	}
	summary.Laws = summary.Holds + summary.Broken + summary.Pending
	return summary
}

func (summary *Summary) count(status string) {
	switch status {
	case OutcomeHolds:
		summary.Holds++
	case OutcomeBroken:
		summary.Broken++
	case OutcomePending:
		summary.Pending++
	}
}
