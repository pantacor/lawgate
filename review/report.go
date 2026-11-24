package review

import (
	"cmp"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/inventory"
)

// Report is a branch review: every changed hunk with the machine states,
// sites, subjects, tests and laws it touches and the commits that explain it.
type Report struct {
	Base      string       `json:"base"`
	MergeBase string       `json:"merge_base"`
	Head      string       `json:"head"`
	Scope     []string     `json:"scope,omitempty"` // the paths the review is limited to
	Files     []FileReport `json:"files"`
	Laws      []LawImpact  `json:"laws"`
	Commits   []Commit     `json:"commits"`
	// RuleChanges are the commits that touch a rule file, and the audit
	// trailers they carry.
	RuleChanges []RuleChange `json:"rule_changes"`
	// LockChanges say which laws' proofs or subjects the proof lock changes:
	// each needs the owner's review.
	LockChanges []string  `json:"lock_changes,omitempty"`
	Problems    []Problem `json:"problems"`
	// Notes are what the review could not establish (rule files it could
	// not classify, a lock it could not read); the rest still stands.
	Notes []string `json:"notes,omitempty"`
}

// FileReport is one changed file.
type FileReport struct {
	Path     string       `json:"path"`
	OldPath  string       `json:"old_path,omitempty"`
	Status   string       `json:"status"`
	RuleFile bool         `json:"rule_file,omitempty"`
	LockFile bool         `json:"lock_file,omitempty"`
	Hunks    []HunkReport `json:"hunks"`
}

// HunkReport is one changed range and what governs it.
type HunkReport struct {
	Hunk
	Sites        []Site    `json:"sites,omitempty"`        // sites on changed lines
	Declarations []Site    `json:"declarations,omitempty"` // site declarations on changed lines
	Subjects     []CodeRef `json:"subjects,omitempty"`     // law subjects whose code changed
	Tests        []CodeRef `json:"tests,omitempty"`        // enforcing tests whose code changed
	States       []string  `json:"states,omitempty"`       // machine/state whose code changed
	Laws         []string  `json:"laws,omitempty"`         // keys of the laws that cover the change
	LawText      []string  `json:"law_text,omitempty"`     // keys of the laws whose LAWS.md text changed
	Commits      []string  `json:"commits,omitempty"`      // short SHAs that changed these lines
	Uncommitted  bool      `json:"uncommitted,omitempty"`
}

// governed reports whether anything declared covers the hunk.
func (h HunkReport) governed() bool {
	lists := []int{len(h.Sites), len(h.Declarations), len(h.Subjects), len(h.Tests), len(h.States), len(h.Laws), len(h.LawText)}
	return slices.ContainsFunc(lists, func(count int) bool { return count > 0 })
}

// LawImpact is a law the branch touches.
type LawImpact struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Module      string    `json:"module,omitempty"`
	File        string    `json:"file,omitempty"`
	Line        int       `json:"line,omitempty"`
	Status      string    `json:"status"`
	Verdicts    []Verdict `json:"verdicts,omitempty"`
	TextChanged bool      `json:"text_changed,omitempty"`
	// Where lists the changed places it covers ("file:line site 41.19").
	Where    []string  `json:"where"`
	Known    bool      `json:"known"` // written in a LAWS.md or checked in code
	Subjects []CodeRef `json:"subjects,omitempty"`
	Tests    []CodeRef `json:"tests,omitempty"`
}

// RuleChange is a commit that touches a rule file.
type RuleChange struct {
	Commit     string   `json:"commit"`
	Subject    string   `json:"subject"`
	Files      []string `json:"files"`
	ApprovedBy []string `json:"approved_by,omitempty"`
	// Approved is set when the commit, or a later one that approves it,
	// carries the audit trailer. The approval that counts is the owner's
	// approval on the pull or merge request (rule-changes --require-approval).
	Approved bool `json:"approved"`
}

// ReportOptions tune a review.
type ReportOptions struct {
	// Paths limit the review to these repository paths (directories or
	// files); none reviews the whole repository.
	Paths []string
	// MergeRequests asks the platform's CLI (gh or glab) which pull or merge
	// request brought each commit. Off by default: it needs the network.
	MergeRequests bool
}

// Review reports the changes from base's merge base with HEAD to the
// working tree.
func Review(idx *Index, base string, options ReportOptions) (*Report, error) {
	history, err := readHistory(idx.Root, base, options.Paths)
	if err != nil {
		return nil, err
	}
	config, configErr := inventory.Load(idx.Root)
	if options.MergeRequests {
		history.lookupMergeRequests(config.Platform)
	}
	report := &Report{Base: base, MergeBase: history.mergeBase, Head: history.head, Scope: options.Paths, Commits: history.commits, Problems: idx.Problems}
	if configErr != nil {
		report.Notes = append(report.Notes, "lawgate.json: "+configErr.Error())
	}
	rules := readRuleFiles(idx.Root, config.LockPath(), report)
	reviewer := newReviewer(idx, history)
	for _, file := range history.diff {
		report.Files = append(report.Files, reviewer.file(file, rules))
	}
	report.Laws = reviewer.impacts()
	report.RuleChanges = ruleChanges(idx.Root, history.commits, rules)
	report.LockChanges = lockChanges(idx.Root, history, config.LockPath(), report)
	return report, nil
}

// history is what git says about the branch.
type history struct {
	root      string
	mergeBase string
	head      string
	diff      []FileDiff
	dirty     []FileDiff
	commits   []Commit
	short     map[string]string // full SHA → short SHA of the branch's commits
}

func readHistory(root, base string, paths []string) (*history, error) {
	h := &history{root: root, short: make(map[string]string)}
	var err error
	if h.mergeBase, err = MergeBase(root, base); err != nil {
		return nil, err
	}
	if h.head, err = Head(root); err != nil {
		return nil, err
	}
	if err = h.readChanges(paths); err != nil {
		return nil, err
	}
	if h.commits, err = Commits(root, h.mergeBase, "HEAD"); err != nil {
		return nil, err
	}
	for _, commit := range h.commits {
		h.short[commit.SHA] = commit.Short
	}
	return h, nil
}

// readChanges reads the diff from the merge base, the part of it not
// committed yet, and the files git does not track (all of them changes).
func (h *history) readChanges(paths []string) error {
	var err error
	if h.diff, err = Diff(h.root, h.mergeBase, paths...); err != nil {
		return err
	}
	if h.dirty, err = Diff(h.root, "HEAD", paths...); err != nil {
		return err
	}
	untracked, err := Untracked(h.root, paths...)
	h.diff = append(h.diff, untracked...)
	h.dirty = append(h.dirty, untracked...)
	return err
}

func (h *history) lookupMergeRequests(platform string) {
	for i := range h.commits {
		h.commits[i].MergeRequest = MergeRequestFor(h.root, platform, h.commits[i].SHA)
	}
}

// changedInWorktree reports whether lines from..to of path differ from HEAD.
func (h *history) changedInWorktree(path string, from, to int) bool {
	for _, file := range h.dirty {
		if file.Path != path {
			continue
		}
		overlaps := func(hunk Hunk) bool { return hunk.NewStart <= to && hunk.NewEnd() >= from }
		if slices.ContainsFunc(file.Hunks, overlaps) {
			return true
		}
	}
	return false
}

// rank orders laws for a reviewer: broken, changed text, unknown, pending,
// then the rest.
func rank(law LawImpact) int {
	switch {
	case law.Status == OutcomeBroken:
		return 0
	case law.TextChanged:
		return 1
	case !law.Known:
		return 2
	case law.Status == OutcomePending:
		return 3
	}
	return 4
}

func sortImpacts(laws []LawImpact) {
	slices.SortFunc(laws, func(left, right LawImpact) int {
		return cmp.Or(cmp.Compare(rank(left), rank(right)), strings.Compare(left.Key, right.Key))
	})
}
