package laws

import (
	"testing"

	"github.com/pantacor/lawgate/machine"
)

func TestNothingSilent(t *testing.T) {
	holds(t, NothingSilent(), worker())
	m := worker()
	work := m.State("work")
	work.Steps[0].Sites = append(work.Steps[0].Sites, site("5", machine.SiteSilent, "logged"))
	breaks(t, NothingSilent(), m, `5 in work/run is silent`)
}

func TestEveryFailureSaysWhy(t *testing.T) {
	holds(t, EveryFailureSaysWhy(), worker())
	m := worker()
	m.State("work").Steps[0].Sites[1].Effect = " "
	breaks(t, EveryFailureSaysWhy(), m, `3 in work/run does not say what the user is told`)
}

func TestEveryFailureSaysWhyChecksOnlyTheKindsItIsGiven(t *testing.T) {
	m := worker()
	m.State("start").Steps[0].Sites[0].Effect = ""
	holds(t, EveryFailureSaysWhy(), m)
	law := EveryFailureSaysWhy(machine.SiteWrite)
	breaks(t, law, m, `1 in start/store-running does not say`)
	holds(t, law, worker())
}

func TestReachesOneOf(t *testing.T) {
	law := ReachesOneOf("no-stuck-deployment", "", "done")
	holds(t, law, worker())
	m := worker()
	m.State("finish").Edges[0] = edgeTo("stored", "start", machine.EdgeLoop)
	breaks(t, law, m, `state "finish" never reaches done`)
}

func TestReachesOneOfNeverHoldsByDefault(t *testing.T) {
	breaks(t, ReachesOneOf("reaches", ""), worker(), "no target states are named")
	breaks(t, ReachesOneOf("reaches", "", "done", "canceled"), worker(), `target state "canceled" is not declared`)
}

// A wait with no way out: ReachesOneOf leaves waits to a later pass,
// NoStuck reports them.
func TestNoStuckChecksWaitsReachesOneOfDoesNot(t *testing.T) {
	m := worker()
	m.State("retry").Edges = nil
	mustBeValid(t, m)
	if violations := ReachesOneOf("reaches", "", "done", "failed").Check(m); len(violations) > 0 {
		t.Fatalf("ReachesOneOf must leave waits out, got %v", violations)
	}
	breaks(t, NoStuck("no-stuck-job", ""), m, `state "retry" never reaches a terminal state`)
}

func TestNoStuck(t *testing.T) {
	holds(t, NoStuck("no-stuck-action", ""), worker())
	holds(t, NoStuck("no-stuck-job", "", "done", "failed"), worker())
	breaks(t, NoStuck("no-stuck-job", "", "done"), worker(), `terminal state "failed" shows "failed", not done`)
	breaks(t, NoStuck("no-stuck-job", ""), &machine.Machine{Name: "empty"}, "the machine has no states")
}

func TestWaitsEnd(t *testing.T) {
	law := WaitsEnd("every-wait-ends", "")
	holds(t, law, worker())
	m := worker()
	m.State("retry").WokenBy = ""
	holds(t, law, m)
	work := m.State("work")
	work.Edges = append(work.Edges, machine.Edge{On: "busy", To: "retry", Kind: machine.EdgeExit, Guard: "No limit"})
	breaks(t, law, m, `state "work" waits in "retry" on "busy" with no limit`)
	if violations := WaitsEnd("every-wait-ends", "", "work→retry").Check(m); len(violations) > 0 {
		t.Fatalf("an allowed wait is not reported, got %v", violations)
	}
}

func TestDoneMeansStored(t *testing.T) {
	law := DoneMeansStored("done", storesDoneLabel)
	holds(t, law, worker())
	m := worker()
	m.State("finish").Steps[0].Sites[0].Labels = nil
	breaks(t, law, m, `state "finish" goes to "done" without a write labelled "stores-done"`)
}

func TestDoneMeansStoredCountsCatchesAndTheEntry(t *testing.T) {
	law := DoneMeansStored("done", storesDoneLabel)
	m := worker()
	work := m.State("work")
	work.Steps[0].Sites = append(work.Steps[0].Sites, site("6", machine.SiteWarn, "partly done"))
	m.Catches = append(m.Catches, catch(machine.SiteWarn, "done"))
	breaks(t, law, m, `state "work" goes to "done"`)
	m = worker()
	m.Entry = "done"
	breaks(t, law, m, `the machine starts in "done"`)
	breaks(t, DoneMeansStored("finished", storesDoneLabel), worker(), `done state "finished" is not declared`)
}

func TestOneShotFailsOnFault(t *testing.T) {
	law := OneShotFailsOnFault(oneShotLabel, "failed", machine.SiteFault, machine.SiteWrite)
	holds(t, law, oneShot())
	holds(t, law, worker())
	m := worker()
	m.Labels = []string{oneShotLabel}
	breaks(t, law, m, `a fault error goes to "retry" instead of failing at once in "failed"`)
	breaks(t, law, m, `a one-shot job can wait in "retry"`)
}

func TestOneShotFailsOnFaultNeedsACatchAndATerminalFailure(t *testing.T) {
	law := OneShotFailsOnFault(oneShotLabel, "failed", machine.SiteFault, machine.SiteWrite)
	m := oneShot()
	m.Catches = m.Catches[:1]
	breaks(t, law, m, "the one-shot machine has no catch for write errors")
	breaks(t, OneShotFailsOnFault(oneShotLabel, "stopped"), oneShot(), `failure state "stopped" is not a declared terminal state`)
	breaks(t, OneShotFailsOnFault("", "failed"), worker(), `a fault error goes to "retry"`)
}

// Every witness breaks its law on both typical machines and leaves a
// declaration machine.Validate still accepts: it models a mistake a
// developer could make, not a malformed machine.
func TestWitnessesBreakTheirLawsOnValidMachines(t *testing.T) {
	builders := []func() *machine.Machine{worker, oneShot}
	for _, law := range everyLaw() {
		for _, build := range builders {
			witnessBreaks(t, law, build())
		}
	}
}

// A witness never declares a state id the machine uses, nor a step id the
// entry state uses; witnessBreaks validates the result.
func TestWitnessesKeepIDsUnique(t *testing.T) {
	for _, law := range everyLaw() {
		m := worker()
		taken := state(witnessID+"-loop", steps(step(witnessID)), edgeTo("on", "finish", machine.EdgeNext))
		m.States = append(m.States, taken)
		start := m.State("start")
		start.Edges = append(start.Edges, edgeTo("taken", taken.ID, machine.EdgeNext))
		start.Steps = append(start.Steps, step(witnessID))
		mustBeValid(t, m)
		witnessBreaks(t, law, m)
	}
}

// A step id repeats across states, so a witness step only has to be new in
// the entry state it is added to.
func TestWitnessStepIsNewInTheEntryState(t *testing.T) {
	m := worker()
	work := m.State("work")
	work.Steps = append(work.Steps, step(witnessID))
	NothingSilent().Witness(m)
	start := m.State("start")
	if added := start.Steps[len(start.Steps)-1].ID; added != witnessID {
		t.Fatalf("the witness step is %q, want %q: work's step does not clash", added, witnessID)
	}
	start.Steps = append(start.Steps, step(witnessID+"-2"))
	NothingSilent().Witness(m)
	if added := start.Steps[len(start.Steps)-1].ID; added != witnessID+"-3" {
		t.Fatalf("the witness step is %q, want %q", added, witnessID+"-3")
	}
}

func everyLaw() []machine.Law {
	return []machine.Law{
		NothingSilent(),
		EveryFailureSaysWhy(),
		ReachesOneOf("no-stuck-deployment", "", "done", "failed"),
		NoStuck("no-stuck-job", "", "done", "failed"),
		WaitsEnd("every-wait-ends", ""),
		DoneMeansStored("done", storesDoneLabel),
		OneShotFailsOnFault(oneShotLabel, "failed", machine.SiteFault, machine.SiteWrite),
	}
}

// Fleet's catches that carry on: a warn logged to the action's log and a
// silent site logged to the pod log only.
func withCarryOnCatches(m *machine.Machine) *machine.Machine {
	logged := machine.Catch{On: machine.SiteWarn, Note: "a warning in the action's log; the action carries on"}
	swallowed := machine.Catch{On: machine.SiteSilent, Note: "logged and swallowed; the pass carries on"}
	m.Catches = append(m.Catches, logged, swallowed)
	return m
}

// A catch that carries on moves nothing, so the lifecycle laws hold as
// before; it does not excuse a silent site.
func TestCatchesThatCarryOn(t *testing.T) {
	for _, law := range everyLaw() {
		holds(t, law, withCarryOnCatches(worker()))
	}
	m := withCarryOnCatches(worker())
	work := m.State("work")
	work.Steps[0].Sites = append(work.Steps[0].Sites, site("5", machine.SiteSilent, "pod log only"))
	breaks(t, NothingSilent(), m, `5 in work/run is silent`)
}

// A one-shot job whose fault carries on would go on after the fault.
func TestOneShotFailsOnFaultRefusesAFaultThatCarriesOn(t *testing.T) {
	m := oneShot()
	m.Catches[0] = machine.Catch{On: machine.SiteFault, Note: "logged"}
	law := OneShotFailsOnFault(oneShotLabel, "failed", machine.SiteFault, machine.SiteWrite)
	breaks(t, law, m, `a fault error carries on instead of failing at once in "failed"`)
}
