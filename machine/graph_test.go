package machine

import (
	"slices"
	"testing"
)

// approvalMachine has states start, approving (step approve), installing
// (step install) and done; startEdges decide whether start may skip approving.
func approvalMachine(startEdges ...Edge) *Machine {
	approve := withSteps(working("approving", next("installing")), step("approve"))
	install := withSteps(working("installing", next("done")), step("install"))
	return newMachine(working("start", startEdges...), approve, install, terminal("done"))
}

// Regression: "approve precedes install" was checked with the
// existential helper, which accepts a path to install that skips approve.
func TestAlwaysBeforeNeedsEveryPath(t *testing.T) {
	bypass := approvalMachine(next("approving"), next("installing"))
	if !bypass.SomePathBefore("approve", "install") {
		t.Fatal("some path runs approve before install")
	}
	if bypass.AlwaysBefore("approve", "install") {
		t.Fatal("a path skips approve, so approve does not always run before install")
	}
	strict := approvalMachine(next("approving"))
	if !strict.AlwaysBefore("approve", "install") {
		t.Fatal("every path to install runs approve first")
	}
}

// A catch that leaves a state before its step ran is a path that skips the
// step, even though it passes through the step's state.
func TestAlwaysBeforeFollowsCatchesFromTheirStep(t *testing.T) {
	checking := withSteps(working("checking", next("using")), step("risky", failSite("1")), step("check"))
	using := withSteps(working("using", next("done")), step("use"))
	retry := working("retry", next("using"))
	m := newMachine(checking, using, retry, terminal("done"))
	m.Catches = []Catch{catchTo(SiteFail, "retry")}
	if m.AlwaysBefore("check", "use") {
		t.Fatal("risky fails before check runs and the catch leads on to use")
	}
	m.Catches = nil
	if !m.AlwaysBefore("check", "use") {
		t.Fatal("without the catch, every path to use runs check first")
	}
}

func TestAlwaysBeforeInOneState(t *testing.T) {
	work := withSteps(working("start", next("done")), step("first"), step("second"))
	m := newMachine(work, terminal("done"))
	if !m.AlwaysBefore("first", "second") {
		t.Fatal("steps of one state run in order")
	}
	if m.AlwaysBefore("second", "first") {
		t.Fatal("second runs after first")
	}
}

func TestAlwaysBeforeRefusesUnknownOrEqualSteps(t *testing.T) {
	m := approvalMachine(next("approving"))
	if m.AlwaysBefore("approve", "ghost") || m.AlwaysBefore("ghost", "install") {
		t.Fatal("an undeclared step never satisfies a precedence law")
	}
	if m.AlwaysBefore("install", "install") {
		t.Fatal("a step does not precede itself")
	}
}

func TestSomePathBeforeInOneStateNeedsALoop(t *testing.T) {
	work := withSteps(working("start", next("done")), step("first"), step("second"))
	m := newMachine(work, terminal("done"))
	if m.SomePathBefore("second", "first") {
		t.Fatal("without a loop, second never runs before first")
	}
	m.States[0].Edges = append(m.States[0].Edges, next("start"))
	if !m.SomePathBefore("second", "first") {
		t.Fatal("the loop back to start runs second before first")
	}
}

func TestReachableStartsAtEntry(t *testing.T) {
	m := newMachine(working("start", next("done")), working("orphan", next("done")), terminal("done"))
	reached := m.Reachable()
	reachesTheLiveStates := reached["start"] && reached["done"]
	if !reachesTheLiveStates || reached["orphan"] {
		t.Fatalf("reached %v", reached)
	}
}

// branchesMachine runs step close in two states: left checks first, right
// does not; both go on to store, which saves.
func branchesMachine() *Machine {
	start := working("start", next("left"), next("right"))
	left := withSteps(working("left", next("store")), step("check"), step("close"))
	right := withSteps(working("right", next("store")), step("close"))
	store := withSteps(working("store", next("done")), step("save"))
	return newMachine(start, left, right, store, terminal("done"))
}

// Regression: AlwaysBefore looked a step up by id and took its first
// occurrence, so a law over "close" only saw left/close and missed the
// path through right/close that skips check.
func TestAlwaysBeforeCoversEveryOccurrenceOfTheLaterStep(t *testing.T) {
	m := branchesMachine()
	if m.AlwaysBefore("check", "close") {
		t.Fatal("right/close runs without check, so check does not always run before close")
	}
	leftClose := StepRef{State: "left", Step: "close"}
	if !m.AlwaysBeforeRef(StepRef{Step: "check"}, leftClose) {
		t.Fatal("check runs before left/close on every path")
	}
}

// Regression: with only the first occurrence of the earlier step counted,
// the path through right/close looked like a path that skips close.
func TestAlwaysBeforeAcceptsAnyOccurrenceOfTheEarlierStep(t *testing.T) {
	m := branchesMachine()
	if !m.AlwaysBefore("close", "save") {
		t.Fatal("both branches run close before save")
	}
	if m.AlwaysBeforeRef(StepRef{State: "left", Step: "close"}, StepRef{Step: "save"}) {
		t.Fatal("the right branch reaches save without left/close")
	}
}

// One step id in a chain of states: an occurrence does not precede itself,
// and AlwaysBefore of an id with itself is false.
func TestAlwaysBeforeWithOneIDInTwoStates(t *testing.T) {
	first := withSteps(working("first", next("second")), step("x"))
	second := withSteps(working("second", next("done")), step("x"))
	m := newMachine(first, second, terminal("done"))
	if m.AlwaysBefore("x", "x") {
		t.Fatal("a step id does not precede itself")
	}
	if !m.AlwaysBeforeRef(StepRef{Step: "x"}, StepRef{State: "second", Step: "x"}) {
		t.Fatal("first/x runs before second/x on every path")
	}
	if m.AlwaysBeforeRef(StepRef{Step: "x"}, StepRef{State: "first", Step: "x"}) {
		t.Fatal("nothing runs before first/x")
	}
	if m.AlwaysBeforeRef(StepRef{State: "done", Step: "x"}, StepRef{Step: "x"}) {
		t.Fatal("a reference to an undeclared occurrence never satisfies a precedence law")
	}
}

// Regression: SomePathBefore started from the first occurrence of a only.
func TestSomePathBeforeStartsFromEveryOccurrence(t *testing.T) {
	m := branchesMachine()
	m.States[0].Edges = []Edge{next("right"), next("left")}
	m.State("left").Edges = []Edge{next("done")}
	if !m.SomePathBefore("close", "save") {
		t.Fatal("right/close goes on to save")
	}
	if m.SomePathBeforeRef(StepRef{State: "left", Step: "close"}, StepRef{Step: "save"}) {
		t.Fatal("left/close never goes on to save")
	}
}

// After step a has run, a catch of a step before it in the same state can
// no longer be taken: the walk starts from a's position, not its state.
func TestSomePathBeforeIgnoresCatchesOfEarlierSteps(t *testing.T) {
	work := withSteps(working("work", next("done")), step("risky", failSite("1")), step("a"))
	recovery := withSteps(working("recover", next("done")), step("b"))
	m := newMachine(work, recovery, terminal("done"))
	m.Catches = []Catch{catchTo(SiteFail, "recover")}
	if m.SomePathBefore("a", "b") {
		t.Fatal("only risky's failure leads to b, and risky runs before a")
	}
	if !m.SomePathBefore("risky", "b") {
		t.Fatal("risky's failure leads to b")
	}
}

func TestStepLookupsByStateAndID(t *testing.T) {
	m := branchesMachine()
	if got := m.StatesWithStep("close"); !slices.Equal(got, []string{"left", "right"}) {
		t.Fatalf("close is declared in left and right, got %v", got)
	}
	if m.StepIn("right", "close") != &m.State("right").Steps[0] {
		t.Fatal("StepIn finds the occurrence in the named state")
	}
	if m.StepIn("store", "close") != nil || m.StepIn("ghost", "close") != nil {
		t.Fatal("StepIn finds nothing in a state without the step, or an undeclared state")
	}
	ref := StepRef{State: "left", Step: "close"}
	if ref.String() != "left/close" || (StepRef{Step: "close"}).String() != "close" {
		t.Fatalf("StepRef strings: %q", ref.String())
	}
}

// A catch that carries on is no edge: Next leaves it out, and a pass that
// takes it goes on to the next step, so it skips nothing.
func TestCatchThatCarriesOnIsNoEdge(t *testing.T) {
	checking := withSteps(working("checking", next("using")), step("risky", failSite("1")), step("check"))
	using := withSteps(working("using", next("done")), step("use"))
	m := newMachine(checking, using, terminal("done"))
	m.Catches = []Catch{{On: SiteFail, Note: "logged; the pass carries on"}}
	if got := m.Next(&m.States[0]); !slices.Equal(got, []string{"using"}) {
		t.Fatalf("Next = %v, want only the edge to using", got)
	}
	if !m.AlwaysBefore("check", "use") {
		t.Fatal("risky's failure carries on into check, so check still runs before use")
	}
	if m.SomePathBefore("use", "risky") {
		t.Fatal("a carry-on catch leads nowhere back")
	}
}
