// Package review ties a line of an adopting repository to what governs it:
// the declared machine state and site it belongs to, the laws that cover it
// and whether they hold, the law a function is the subject of or a test
// proves, and why the line changed. It works on any repository with a
// lawgate.json, the way `lawgate check` does: machines are read from each
// module's runner (`-index`), never by importing adopter code.
//
// `lawgate review` prints its branch report; `lawgate review -lsp` serves it
// to editors (package review/lsp).
package review

import (
	"github.com/pantacor/lawgate/mutate"
	"github.com/pantacor/lawgate/review/gocode"
	"github.com/pantacor/lawgate/review/index"
)

// Index is everything review knows about a repository. Paths are relative to
// Root, with forward slashes.
type Index struct {
	Root   string  `json:"root"`
	Sites  []Site  `json:"sites"`
	States []State `json:"states"`
	// Laws are keyed by law name; a name a later module documents again is
	// keyed "<LAWS.md path>#<name>" (Law.Key), so no law hides another.
	Laws     map[string]*Law `json:"laws"`
	Problems []Problem       `json:"problems"`
	// modules are the lawgate.json modules, to find the module of a file,
	// and documents the LAWS.md files each module reads.
	modules   []string
	documents map[string][]string
}

// Site is a declared site, resolved to its line.
type Site struct {
	Module  string `json:"module"`
	Machine string `json:"machine"`
	index.Site
	StateDoc string `json:"state_doc,omitempty"`
}

// State is a declared state and the code it covers.
type State struct {
	Module  string `json:"module"`
	Machine string `json:"machine"`
	index.State
	File      string `json:"file,omitempty"` // where Func is defined
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	// Spans are the code the state covers: Func, and every function that
	// holds one of its sites (a helper the state calls).
	Spans []gocode.Range `json:"spans,omitempty"`
}

// Law is a law as LAWS.md writes it and as its machines check it.
type Law struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Module  string `json:"module"`
	File    string `json:"file,omitempty"` // the LAWS.md it is written in
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
	Text    string `json:"text,omitempty"`
	// The law's fields as written.
	Source     string `json:"source,omitempty"`
	EnforcedBy string `json:"enforced_by,omitempty"`
	Status     string `json:"status,omitempty"`
	Subject    string `json:"subject,omitempty"`
	// CodeDoc is the Doc of the machine law of that name.
	CodeDoc string `json:"code_doc,omitempty"`
	// Subjects are the functions the law protects and Tests the tests that
	// prove it, resolved to their lines (Go modules only).
	Subjects []CodeRef `json:"subjects,omitempty"`
	Tests    []CodeRef `json:"tests,omitempty"`
	// Verdicts are how the law fares on each machine that checks it.
	Verdicts []Verdict `json:"verdicts,omitempty"`
	// subjects and testNames are what LAWS.md names, before resolving.
	subjects  []mutate.Subject
	testNames []string
}

// CodeRef is a function a law names: a subject or an enforcing test.
type CodeRef struct {
	Name    string `json:"name"`
	Law     string `json:"law"` // the law's key
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
	// Problem says why the function could not be found.
	Problem string `json:"problem,omitempty"`
}

// Verdict is how one law fared on one machine.
type Verdict struct {
	Machine    string   `json:"machine"`
	Status     string   `json:"status"` // holds, broken or pending
	Pending    string   `json:"pending,omitempty"`
	Violations []string `json:"violations,omitempty"`
}

// Problem is something review could not reconcile: declaration drift, a
// machine command that failed, a subject that is not in the code.
type Problem struct {
	Module  string `json:"module,omitempty"`
	Machine string `json:"machine,omitempty"`
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// The outcome of a law over its verdicts.
const (
	OutcomeHolds     = index.StatusHolds
	OutcomeBroken    = index.StatusBroken
	OutcomePending   = index.StatusPending
	OutcomeUnchecked = "unchecked" // no machine checks it: tests or scripts do
	OutcomeUnknown   = "unknown"   // no LAWS.md writes it and no machine checks it
)

// Outcome sums a law's verdicts: broken if any machine breaks it, pending if
// any is pending, holds if all hold, and unchecked when no machine checks it.
func (law *Law) Outcome() string {
	if len(law.Verdicts) == 0 {
		return OutcomeUnchecked
	}
	outcome := OutcomeHolds
	for _, verdict := range law.Verdicts {
		if verdict.Status == OutcomeBroken {
			return OutcomeBroken
		}
		if verdict.Status == OutcomePending {
			outcome = OutcomePending
		}
	}
	return outcome
}
