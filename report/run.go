// Package report writes how the laws fared: a JUnit report for CI test views,
// a markdown summary for a job page and a self-contained HTML page. All three
// read the same LawRun values, so they cannot disagree, and ReadJUnit turns a
// written JUnit report back into runs, which is how several modules' reports
// become one page.
//
// A module that crashed, wrote no report or checked no law is an
// infrastructure failure: it is shown as such, and the result is never green.
package report

import "github.com/pantacor/lawgate/machine"

// LawRun is how one machine's laws fared, or why they could not be checked.
type LawRun struct {
	Module     string // the module the machine belongs to; may be empty
	Machine    string
	Source     string
	Verdicts   []machine.Verdict
	GateChecks bool // verdicts from tests, documentation or repository checks
	// Failure says why the laws could not be checked (the module did not
	// build, crashed, wrote no report). A run with a Failure has no verdicts.
	Failure string
}

// FailedRun records that a module's laws could not be checked, and why.
func FailedRun(module, reason string) LawRun {
	return LawRun{Module: module, Machine: "(not checked)", Failure: reason}
}

// failure is why the run is an infrastructure failure, or "".
func (r LawRun) failure() string {
	if r.Failure != "" {
		return r.Failure
	}
	if len(r.Verdicts) == 0 {
		return "no law was checked"
	}
	return ""
}

// Broken counts the laws that found violations and are not pending.
func (r LawRun) Broken() int {
	count := 0
	for _, verdict := range r.Verdicts {
		if verdict.Broken() {
			count++
		}
	}
	return count
}

// Pending counts the laws whose violations wait on a decision.
func (r LawRun) Pending() int {
	count := 0
	for _, verdict := range r.Verdicts {
		if verdict.IsPending() {
			count++
		}
	}
	return count
}

func visibleViolations(verdict machine.Verdict) []string {
	if verdict.IsPending() && len(verdict.Violations) == 1 {
		if verdict.Violations[0] == verdict.Law.Pending {
			return nil // The report already shows the reason as a pending gap.
		}
	}
	return verdict.Violations
}

// Totals counts what a set of runs holds.
type Totals struct {
	Machines int // runs that checked laws
	Checked  int // machine laws checked
	Checks   int // gate checks completed
	Broken   int
	Pending  int
	Failed   int // runs that are infrastructure failures
}

// Sum counts the runs.
func Sum(runs []LawRun) Totals {
	var totals Totals
	for _, run := range runs {
		totals.add(run)
	}
	return totals
}

func (t *Totals) add(run LawRun) {
	if run.failure() != "" {
		t.Failed++
		return
	}
	if run.GateChecks {
		t.Checks += len(run.Verdicts)
	} else {
		t.Machines++
		t.Checked += len(run.Verdicts)
	}
	t.Broken += run.Broken()
	t.Pending += run.Pending()
}

// Outcome is the verdict over every run.
type Outcome string

const (
	OutcomeHolds   Outcome = "holds"   // every law holds
	OutcomePending Outcome = "pending" // every law holds or waits on a decision
	OutcomeBroken  Outcome = "broken"  // a law broke
	OutcomeFailed  Outcome = "failed"  // something could not be checked, or nothing was
)

// Outcome says how the runs fared. An infrastructure failure, or a set of
// runs that checked no law at all, is OutcomeFailed: never green.
func (t Totals) Outcome() Outcome {
	switch {
	case t.Failed > 0, t.Checked+t.Checks == 0:
		return OutcomeFailed
	case t.Broken > 0:
		return OutcomeBroken
	case t.Pending > 0:
		return OutcomePending
	}
	return OutcomeHolds
}

// Refuses reports whether the outcome refuses the change.
func (o Outcome) Refuses() bool {
	return o == OutcomeFailed || o == OutcomeBroken
}

// moduleMachine names a run for a reader: module/machine, or the machine.
func moduleMachine(run LawRun, separator string) string {
	if run.Module == "" {
		return run.Machine
	}
	return run.Module + separator + run.Machine
}
