package review

import (
	"slices"
	"strconv"

	"github.com/pantacor/lawgate/review/index"
)

// reviewer ties each changed hunk to what governs it and collects the laws
// the branch touches.
type reviewer struct {
	idx     *Index
	history *history
	touched map[string]*LawImpact
}

func newReviewer(idx *Index, h *history) *reviewer {
	return &reviewer{idx: idx, history: h, touched: make(map[string]*LawImpact)}
}

func (r *reviewer) file(diff FileDiff, rules ruleSet) FileReport {
	report := FileReport{Path: diff.Path, OldPath: diff.OldPath, Status: diff.Status}
	report.RuleFile = rules.contains(diff.Path)
	report.LockFile = diff.Path == rules.lock
	for _, hunk := range diff.Hunks {
		report.Hunks = append(report.Hunks, r.hunk(diff, hunk))
	}
	return report
}

func (r *reviewer) hunk(diff FileDiff, hunk Hunk) HunkReport {
	report := HunkReport{Hunk: hunk}
	if diff.Status != "deleted" {
		r.governing(&report, diff.Path)
		report.Commits = r.commitsTouching(diff.Path, hunk)
		report.Uncommitted = r.history.changedInWorktree(diff.Path, hunk.NewStart, hunk.NewEnd())
	}
	report.Laws = mergeNames(report.Laws, r.namedLaws(diff.Path, hunk.Added))
	r.touchAll(&report, diff.Path+":"+strconv.Itoa(hunk.NewStart))
	return report
}

// governing fills in the states, sites, subjects, tests and law text the
// hunk's new lines overlap, and the laws they bring.
func (r *reviewer) governing(report *HunkReport, file string) {
	from, to := report.NewStart, report.NewEnd()
	report.Sites = r.idx.SitesIn(file, from, to)
	report.Declarations = r.idx.DeclsIn(file, from, to)
	report.Subjects = r.idx.SubjectsIn(file, from, to)
	report.Tests = r.idx.TestsIn(file, from, to)
	for _, state := range r.idx.StatesIn(file, from, to) {
		report.States = append(report.States, state.Machine+"/"+state.ID)
		report.Laws = mergeNames(report.Laws, state.Laws)
	}
	for _, site := range slices.Concat(report.Sites, report.Declarations) {
		report.Laws = mergeNames(report.Laws, site.Laws)
	}
	for _, ref := range slices.Concat(report.Subjects, report.Tests) {
		report.Laws = mergeNames(report.Laws, []string{ref.Law})
	}
	for _, law := range r.idx.LawsIn(file, from, to) {
		report.LawText = append(report.LawText, law.Key)
	}
}

// namedLaws are the laws added lines name as "law <name>".
func (r *reviewer) namedLaws(file string, added []string) []string {
	var keys []string
	for _, name := range index.LawRefs(added...) {
		key := name
		if law := r.idx.LawFor(name, file); law != nil {
			key = law.Key
		}
		keys = append(keys, key)
	}
	return keys
}

func (r *reviewer) commitsTouching(file string, hunk Hunk) []string {
	if hunk.NewLines == 0 {
		return nil
	}
	shas, err := CommitsTouching(r.history.root, r.history.mergeBase, file, hunk.NewStart, hunk.NewEnd())
	if err != nil {
		return nil // an uncommitted file has no history yet
	}
	var short []string
	for _, sha := range shas {
		if name := r.history.short[sha]; name != "" {
			short = append(short, name)
		}
	}
	return short
}

// touchAll records where the hunk touches each law.
// A law the hunk touches through a site, a subject, a test or its text is
// listed with that reason only, not again as a bare place.
func (r *reviewer) touchAll(report *HunkReport, where string) {
	named := map[string]bool{}
	for _, site := range report.Sites {
		r.touchEach(named, site.Laws, where+" site "+siteName(site))
	}
	for _, ref := range report.Subjects {
		r.touchEach(named, []string{ref.Law}, where+" subject "+ref.Name)
	}
	for _, ref := range report.Tests {
		r.touchEach(named, []string{ref.Law}, where+" test "+ref.Name)
	}
	for _, key := range report.LawText {
		r.touch(key, where+" (law text)").TextChanged = true
		named[key] = true
	}
	for _, key := range report.Laws {
		if !named[key] {
			r.touch(key, where)
		}
	}
}

func (r *reviewer) touchEach(named map[string]bool, keys []string, where string) {
	for _, key := range keys {
		r.touch(key, where)
		named[key] = true
	}
}

// touch records that the branch changes a place a law covers.
func (r *reviewer) touch(key, where string) *LawImpact {
	impact := r.touched[key]
	if impact == nil {
		impact = r.newImpact(key)
		r.touched[key] = impact
	}
	if !slices.Contains(impact.Where, where) {
		impact.Where = append(impact.Where, where)
	}
	return impact
}

func (r *reviewer) newImpact(key string) *LawImpact {
	law := r.idx.Laws[key]
	if law == nil {
		return &LawImpact{Key: key, Name: key, Status: OutcomeUnknown}
	}
	return &LawImpact{
		Key: key, Name: law.Name, Module: law.Module, File: law.File, Line: law.Line,
		Status: law.Outcome(), Verdicts: law.Verdicts, Known: true,
		Subjects: law.Subjects, Tests: law.Tests,
	}
}

func (r *reviewer) impacts() []LawImpact {
	laws := make([]LawImpact, 0, len(r.touched))
	for _, impact := range r.touched {
		laws = append(laws, *impact)
	}
	sortImpacts(laws)
	return laws
}

func siteName(site Site) string {
	if site.Tag != "" {
		return site.Tag
	}
	return site.State + "/" + site.Step
}

// mergeNames appends the names of more that names lacks.
func mergeNames(names, more []string) []string {
	for _, name := range more {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}
