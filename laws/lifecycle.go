package laws

import (
	"fmt"
	"slices"

	"github.com/pantacor/lawgate/machine"
)

// DoneMeansStoredName is the name of the law DoneMeansStored returns.
const DoneMeansStoredName = "done-means-stored"

// OneShotFailsOnFaultName is the name of the law OneShotFailsOnFault returns.
const OneShotFailsOnFaultName = "one-shot-fails-on-fault"

// DoneMeansStored is the law "done-means-stored": every way into the state
// done (an edge or a catch) leaves a state with a machine.SiteWrite site
// labelled label (for example "stores-done"), so a job is only done once
// its completion is stored. A machine that starts in done, or that does not
// declare done, is a violation. Its witness adds a state with no write that
// goes to done after the entry state.
func DoneMeansStored(done, label string) machine.Law {
	return machine.Law{
		Name:    DoneMeansStoredName,
		Doc:     "every way into " + done + " leaves a state with a write labelled " + label,
		Check:   func(m *machine.Machine) []string { return unstoredDone(m, done, label) },
		Witness: func(m *machine.Machine) { addUnstoredDone(m, done) },
	}
}

func unstoredDone(m *machine.Machine, done, label string) []string {
	if m.State(done) == nil {
		return []string{fmt.Sprintf("done state %q is not declared", done)}
	}
	var violations []string
	if m.EntryID() == done {
		violations = append(violations, fmt.Sprintf("the machine starts in %q without a completion write", done))
	}
	for i := range m.States {
		st := &m.States[i]
		goesDone := slices.Contains(m.Next(st), done)
		if goesDone && !storesWithLabel(st, label) {
			violations = append(violations, fmt.Sprintf("state %q goes to %q without a write labelled %q", st.ID, done, label))
		}
	}
	return violations
}

// storesWithLabel reports whether st has a write site that carries label.
func storesWithLabel(st *machine.State, label string) bool {
	found := false
	for i := range st.Steps {
		found = found || stepStoresWithLabel(&st.Steps[i], label)
	}
	return found
}

func stepStoresWithLabel(step *machine.Step, label string) bool {
	for i := range step.Sites {
		site := &step.Sites[i]
		if site.Kind == machine.SiteWrite && site.HasLabel(label) {
			return true
		}
	}
	return false
}

func addUnstoredDone(m *machine.Machine, done string) {
	skip := machine.Edge{On: witnessID, To: done, Kind: machine.EdgeExit}
	shortcut := machine.State{ID: witnessID + "-skips-the-write"}
	shortcut.Edges = []machine.Edge{skip}
	addStateAfterEntry(m, shortcut, witnessEdge())
}

// OneShotFailsOnFault is the law "one-shot-fails-on-fault": on a machine
// labelled label (for example "one-shot"; an empty label means every
// machine), every catch of each of kinds goes to the state failed, which
// must be terminal, each of kinds has a catch, and no state is a wait. A
// one-shot job must not retry: a rerun after a partial run would duplicate
// its work. A catch of one of kinds that carries on (machine.Catch) breaks
// the law too: the job would go on after the error. With no kinds it checks
// machine.SiteFault; a product whose failed writes are faults adds
// machine.SiteWrite. Its witness labels the machine and sends the first
// kind's catches to a retry wait.
func OneShotFailsOnFault(label, failed string, kinds ...machine.SiteKind) machine.Law {
	checked := slices.Clone(kinds)
	if len(checked) == 0 {
		checked = []machine.SiteKind{machine.SiteFault}
	}
	return machine.Law{
		Name:    OneShotFailsOnFaultName,
		Doc:     "a fault fails a one-shot job at once: no " + joinKinds(checked) + " error leads to a retry",
		Check:   func(m *machine.Machine) []string { return oneShotRetries(m, label, failed, checked) },
		Witness: func(m *machine.Machine) { addOneShotRetry(m, label, checked[0]) },
	}
}

func oneShotRetries(m *machine.Machine, label, failed string, kinds []machine.SiteKind) []string {
	if label != "" && !m.HasLabel(label) {
		return nil
	}
	var violations []string
	target := m.State(failed)
	if target == nil || !target.Terminal {
		violations = append(violations, fmt.Sprintf("failure state %q is not a declared terminal state", failed))
	}
	for _, kind := range kinds {
		violations = append(violations, retryingCatches(m, kind, failed)...)
	}
	for _, st := range m.States {
		if st.Wait {
			violations = append(violations, fmt.Sprintf("a one-shot job can wait in %q", st.ID))
		}
	}
	return violations
}

func retryingCatches(m *machine.Machine, kind machine.SiteKind, failed string) []string {
	var violations []string
	caught := false
	for _, catch := range m.Catches {
		if catch.On != kind {
			continue
		}
		caught = true
		if catch.To != failed {
			violations = append(violations, retryViolation(catch, failed))
		}
	}
	if !caught {
		violations = append(violations, fmt.Sprintf("the one-shot machine has no catch for %s errors", kind))
	}
	return violations
}

// retryViolation says how a catch keeps a one-shot job going after an
// error: by moving it to another state, or by carrying on in the same pass.
func retryViolation(catch machine.Catch, failed string) string {
	if catch.CarriesOn() {
		return fmt.Sprintf("a %s error carries on instead of failing at once in %q", catch.On, failed)
	}
	return fmt.Sprintf("a %s error goes to %q instead of failing at once in %q", catch.On, catch.To, failed)
}

func addOneShotRetry(m *machine.Machine, label string, kind machine.SiteKind) {
	if label != "" && !m.HasLabel(label) {
		m.Labels = append(m.Labels, label)
	}
	retry := machine.State{ID: uniqueStateID(m, witnessID+"-retry"), Wait: true}
	m.States = append(m.States, retry)
	redirected := false
	for i := range m.Catches {
		if m.Catches[i].On == kind {
			m.Catches[i].To = retry.ID
			redirected = true
		}
	}
	if !redirected {
		m.Catches = append(m.Catches, machine.Catch{On: kind, To: retry.ID})
	}
}
