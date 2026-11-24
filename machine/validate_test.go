package machine

import (
	"strings"
	"testing"
)

func errorsText(errs []error) string {
	lines := make([]string, 0, len(errs))
	for _, err := range errs {
		lines = append(lines, err.Error())
	}
	return strings.Join(lines, "\n")
}

func TestValidateAcceptsAWellFormedMachine(t *testing.T) {
	work := withSteps(working("work", next("done")), step("compute", failSite("1")))
	m := newMachine(working("start", next("work")), work, terminal("done"), terminal("failed"))
	m.Catches = []Catch{catchTo(SiteFail, "failed")}
	if errs := Validate(m); len(errs) != 0 {
		t.Fatalf("a well-formed machine must pass:\n%s", errorsText(errs))
	}
}

// Regression: two states that point at each other
// both counted as reached although no path from the entry state gets there.
func TestValidateRejectsAnUnreachableCluster(t *testing.T) {
	orphanA := working("orphan-a", next("orphan-b"))
	orphanB := working("orphan-b", next("orphan-a"), next("done"))
	m := newMachine(working("start", next("done")), orphanA, orphanB, terminal("done"))
	got := errorsText(Validate(m))
	for _, want := range []string{"state orphan-a is never reached", "state orphan-b is never reached"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q, got:\n%s", want, got)
		}
	}
}

func TestValidateCountsACatchAsAWayIn(t *testing.T) {
	work := withSteps(working("start", next("done")), step("compute", failSite("1")))
	m := newMachine(work, terminal("done"), terminal("failed"))
	m.Catches = []Catch{catchTo(SiteFail, "failed")}
	if errs := Validate(m); len(errs) != 0 {
		t.Fatalf("a state reached through a catch is reached:\n%s", errorsText(errs))
	}
}

func TestValidateStartsAtTheDeclaredEntry(t *testing.T) {
	m := newMachine(working("first", next("done")), working("second", next("done")), terminal("done"))
	m.Entry = "second"
	if !strings.Contains(errorsText(Validate(m)), "state first is never reached") {
		t.Fatal("with Entry set, the first state is not reached unless a path leads to it")
	}
}

func TestValidateRejectsMalformedMachines(t *testing.T) {
	for _, tc := range malformedCases() {
		t.Run(tc.name, func(t *testing.T) { checkMalformed(t, tc) })
	}
}

func checkMalformed(t *testing.T, tc malformedCase) {
	got := errorsText(Validate(tc.machine))
	if !strings.Contains(got, tc.want) {
		t.Fatalf("want %q, got:\n%s", tc.want, got)
	}
}

type malformedCase struct {
	name    string
	machine *Machine
	want    string
}

func malformed(name string, m *Machine, want string) malformedCase {
	return malformedCase{name: name, machine: m, want: want}
}

func malformedCases() []malformedCase {
	return []malformedCase{
		malformed("empty", newMachine(), "has no states"),
		malformed("duplicate state", duplicateStateMachine(), "state start is declared twice"),
		malformed("duplicate step", duplicateStepMachine(), "state start declares step work twice"),
		malformed("catch to unknown", unknownCatchMachine(), "catch on fail goes to unknown state nowhere"),
		malformed("edge to unknown", newMachine(working("start", next("nowhere"))), "goes to unknown state nowhere"),
		malformed("unknown entry", unknownEntryMachine(), "entry state ghost is not declared"),
		malformed("terminal with edges", terminalWithEdgeMachine(), "terminal state done has edges"),
		malformed("no way out", newMachine(working("start")), "state start has no way out"),
		malformed("negative limit", negativeLimitMachine(), "negative limit"),
		malformed("duplicate edge name", duplicateEdgeNameMachine(), "two edges are named Go"),
		malformed("negative catch limit", negativeCatchLimitMachine(), "catch on fail has a negative limit -1"),
	}
}

func duplicateStateMachine() *Machine {
	return newMachine(working("start", next("done")), working("start", next("done")), terminal("done"))
}

func duplicateStepMachine() *Machine {
	start := withSteps(working("start", next("done")), step("work"), step("work"))
	return newMachine(start, terminal("done"))
}

// A step id names the work, not the place: Fleet's step names come from
// the Hub API and repeat across states. Validate used to refuse a step id
// declared in two states.
func TestValidateAcceptsAStepIDInSeveralStates(t *testing.T) {
	start := withSteps(working("start", next("other")), step("work"))
	other := withSteps(working("other", next("done")), step("work"))
	m := newMachine(start, other, terminal("done"))
	if errs := Validate(m); len(errs) != 0 {
		t.Fatalf("one step id in two states is well formed:\n%s", errorsText(errs))
	}
}

func unknownCatchMachine() *Machine {
	m := newMachine(working("start", next("done")), terminal("done"))
	m.Catches = []Catch{catchTo(SiteFail, "nowhere")}
	return m
}

func unknownEntryMachine() *Machine {
	m := newMachine(working("start", next("done")), terminal("done"))
	m.Entry = "ghost"
	return m
}

func terminalWithEdgeMachine() *Machine {
	done := terminal("done")
	done.Edges = []Edge{next("start")}
	return newMachine(working("start", next("done")), done)
}

func negativeLimitMachine() *Machine {
	loop := next("start")
	loop.Limit = -1
	return newMachine(working("start", loop, next("done")), terminal("done"))
}

func duplicateEdgeNameMachine() *Machine {
	start := working("start", named(next("done"), "Go"), named(next("failed"), "Go"))
	return newMachine(start, terminal("done"), terminal("failed"))
}

func negativeCatchLimitMachine() *Machine {
	catch := catchTo(SiteFail, "failed")
	catch.Limit = -1
	m := newMachine(working("start", next("done")), terminal("done"), terminal("failed"))
	m.Catches = []Catch{catch}
	return m
}

func TestValidateRejectsReservedEntryEdge(t *testing.T) {
	start := working("start", named(next("done"), "enter"))
	m := newMachine(start, terminal("done"))
	if got := errorsText(Validate(m)); !strings.Contains(got, `"enter" is reserved`) {
		t.Fatalf("a declared edge named enter must be rejected, got %q", got)
	}
}

// Regression: Validate accepted a catch leading back into a
// state that holds its own site kind, a retry loop with nothing bounding it.
func TestValidateRequiresALimitOnACatchBackIntoItsOwnState(t *testing.T) {
	work := withSteps(working("work", next("done")), step("compute", failSite("1")))
	m := newMachine(work, terminal("done"))
	m.Catches = []Catch{catchTo(SiteFail, "work")}
	want := "catch on fail goes back to work, which holds a fail site: it needs a limit"
	if got := errorsText(Validate(m)); !strings.Contains(got, want) {
		t.Fatalf("a catch back into its own state without a limit must be rejected, got %q", got)
	}
	m.Catches[0].Limit = 3
	if errs := Validate(m); len(errs) != 0 {
		t.Fatalf("a catch back into its own state with a limit is well formed:\n%s", errorsText(errs))
	}
}

// carryOnMachine is Fleet's shape: a warn and a silent site whose catches
// say where the message is logged and leave the pass where it is.
func carryOnMachine() *Machine {
	warn := Site{Tag: "1", Kind: SiteWarn, Effect: "noted"}
	silent := Site{Tag: "2", Kind: SiteSilent}
	work := withSteps(working("work", next("done")), step("check", warn, silent), step("store"))
	m := newMachine(work, terminal("done"))
	logged := Catch{On: SiteWarn, Note: "a warning in the action's log; the action carries on"}
	swallowed := Catch{On: SiteSilent, Note: "logged and swallowed; the pass carries on"}
	m.Catches = []Catch{logged, swallowed}
	return m
}

// Regression: a catch whose target describes where the message goes, not a
// state, could only be declared as an unknown state, which Validate refuses.
func TestValidateAcceptsACatchThatCarriesOn(t *testing.T) {
	m := carryOnMachine()
	if errs := Validate(m); len(errs) != 0 {
		t.Fatalf("a catch with no target carries on and is well formed:\n%s", errorsText(errs))
	}
	m.Catches[0].To = "log stream"
	if got := errorsText(Validate(m)); !strings.Contains(got, "catch on warn goes to unknown state log stream") {
		t.Fatalf("a catch to a named state that is not declared is refused, got %q", got)
	}
}

func TestValidateRefusesALimitOnACatchThatCarriesOn(t *testing.T) {
	m := carryOnMachine()
	m.Catches[0].Limit = 2
	want := "catch on warn carries on, so its limit 2 bounds nothing"
	if got := errorsText(Validate(m)); !strings.Contains(got, want) {
		t.Fatalf("want %q, got %q", want, got)
	}
}
