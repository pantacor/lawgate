package machine

import "testing"

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
