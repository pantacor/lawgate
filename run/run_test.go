package run

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

func edge(name, to string) machine.Edge {
	return machine.Edge{On: name, To: to, Kind: machine.EdgeNext, Name: name}
}

// passMachine: start --Ok--> work, work --Retry (limit 2)--> work,
// work --Finish--> done.
func passMachine() *machine.Machine {
	retry := edge("Retry", "work")
	retry.Limit = 2
	startEdges := []machine.Edge{edge("Ok", "work")}
	workEdges := []machine.Edge{retry, edge("Finish", "done")}
	start := machine.State{ID: "start", Edges: startEdges}
	work := machine.State{ID: "work", Edges: workEdges}
	done := machine.State{ID: "done", Terminal: true}
	states := []machine.State{start, work, done}
	return &machine.Machine{Name: "m", States: states}
}

// quietRun records drift without logging it.
func quietRun(m *machine.Machine) *Run {
	r := New(m)
	r.OnDrift = func(string) {}
	return r
}

func TestADeclaredPassHasNoDrift(t *testing.T) {
	r := quietRun(passMachine())
	r.Enter("start")
	r.Go("start", "Ok", "work")
	r.Go("work", "Finish", "done")
	if len(r.Drift) != 0 {
		t.Fatalf("unexpected drift %v", r.Drift)
	}
	if err := IsPath(r.Machine, r.Trace); err != nil {
		t.Fatal(err)
	}
}

// TestGoChecksContinuity enforces no-silent-drift: Run.Go refuses a declared
// edge from a state the pass is not in.
func TestGoChecksContinuity(t *testing.T) {
	r := quietRun(passMachine())
	r.Enter("start")
	if err := r.Go("work", "Finish", "done"); err == nil {
		t.Fatal("a move from the wrong state must return an error")
	}
	if r.At() != "start" || len(r.Trace) != 1 {
		t.Fatalf("a rejected move changed the run: state %q, trace %v", r.At(), r.Trace)
	}
	if len(r.Drift) != 1 {
		t.Fatalf("a move from a state the pass is not in is drift, got %v", r.Drift)
	}
}

func TestGoBeforeEnterIsDrift(t *testing.T) {
	r := quietRun(passMachine())
	if err := r.Go("start", "Ok", "work"); err == nil {
		t.Fatal("a move before Enter must return an error")
	}
	if len(r.Drift) != 1 {
		t.Fatalf("a move before the pass entered a state is drift, got %v", r.Drift)
	}
}

// Regression: a pass could start anywhere and enter
// again in the middle.
func TestEnterOnlyAtTheStartOfAPass(t *testing.T) {
	r := quietRun(passMachine())
	if err := r.Enter("work"); err == nil {
		t.Fatal("an invalid pass entry must return an error")
	}
	if r.At() != "" || len(r.Trace) != 0 {
		t.Fatal("an invalid entry must leave the pass unstarted")
	}
	if len(r.Drift) != 1 {
		t.Fatalf("work is not a pass entry, got %v", r.Drift)
	}
	m := passMachine()
	m.States[1].PassEntry = true
	resumed := quietRun(m)
	resumed.Enter("work")
	if err := resumed.Enter("start"); err == nil {
		t.Fatal("a second entry must return an error")
	}
	if resumed.At() != "work" || len(resumed.Trace) != 1 {
		t.Fatal("a second entry must leave the pass at its first state")
	}
	if len(resumed.Drift) != 1 {
		t.Fatalf("a pass entry is allowed once, a second enter is drift, got %v", resumed.Drift)
	}
}

// Regression: a limited edge was a comment; nothing refused it
// past its limit.
func TestGoRefusesAtTheLimit(t *testing.T) {
	r := quietRun(passMachine())
	r.Enter("start")
	r.Go("start", "Ok", "work")
	for count := range 2 {
		if err := r.Go("work", "Retry", "work"); err != nil {
			t.Fatalf("retry %d is within the limit: %v", count, err)
		}
	}
	err := r.Go("work", "Retry", "work")
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("the third retry must be refused, got %v", err)
	}
	if len(r.Trace) != 4 || len(r.Drift) != 1 {
		t.Fatalf("a refused move is drift and not in the trace: trace %v drift %v", r.Trace, r.Drift)
	}
}

// TestIsPathRefusesBrokenTraces enforces no-silent-drift: a trace that is not
// a path of the declared machine is refused.
func TestIsPathRefusesBrokenTraces(t *testing.T) {
	m := passMachine()
	enterStart := Transition{Edge: EnterEdge, To: "start"}
	enterWork := Transition{Edge: EnterEdge, To: "work"}
	ok := Transition{From: "start", Edge: "Ok", To: "work"}
	finish := Transition{From: "work", Edge: "Finish", To: "done"}
	back := Transition{From: "work", Edge: "Back", To: "start"}
	// A guard that refused every move would pass the refusals below.
	if err := IsPath(m, []Transition{enterStart, ok, finish}); err != nil {
		t.Errorf("the declared pass must stay a path: %v", err)
	}
	checkNotPath(t, m, "empty")
	checkNotPath(t, m, "no enter", ok)
	checkNotPath(t, m, "starts in a state that is not an entry", enterWork, finish)
	err := IsPath(m, []Transition{enterStart, ok, enterWork})
	if err == nil || !strings.Contains(err.Error(), "in the middle of the pass") {
		t.Errorf("second entry should be refused as a mid-pass entry, got %v", err)
	}
	checkNotPath(t, m, "jumps", enterStart, finish)
	checkNotPath(t, m, "undeclared", enterStart, ok, back)
}

func checkNotPath(t *testing.T, m *machine.Machine, name string, trace ...Transition) {
	t.Helper()
	if IsPath(m, trace) == nil {
		t.Errorf("%s: IsPath must refuse %v", name, trace)
	}
}

func TestIsPathRefusesMovesPastTheLimit(t *testing.T) {
	enter := Transition{Edge: EnterEdge, To: "start"}
	ok := Transition{From: "start", Edge: "Ok", To: "work"}
	retry := Transition{From: "work", Edge: "Retry", To: "work"}
	trace := []Transition{enter, ok, retry, retry, retry}
	if err := IsPath(passMachine(), trace); !errors.Is(err, ErrLimit) {
		t.Fatalf("third consecutive retry must be refused, got %v", err)
	}
}

func TestGoTracksLimitsPerEdge(t *testing.T) {
	m := passMachine()
	m.States[1].Edges = append(m.States[1].Edges, edge("Yield", "work"))
	r := quietRun(m)
	requireMove(t, r.Enter("start"))
	requireMove(t, r.Go("start", "Ok", "work"))
	requireMove(t, r.Go("work", "Retry", "work"))
	requireMove(t, r.Go("work", "Retry", "work"))
	if err := r.Go("start", "Ok", "work"); err == nil {
		t.Fatal("Go accepted a move from the wrong state")
	}
	if err := r.Go("work", "Retry", "work"); !errors.Is(err, ErrLimit) {
		t.Fatalf("the third retry must reach the edge limit, got %v", err)
	}
	if r.At() != "work" || len(r.Trace) != 4 {
		t.Fatalf("rejected moves changed the run: state %q, trace %v", r.At(), r.Trace)
	}
	requireMove(t, r.Go("work", "Yield", "work"))
	if err := r.Go("work", "Retry", "work"); !errors.Is(err, ErrLimit) {
		t.Fatalf("taking another edge must preserve the retry count, got %v", err)
	}
	requireMove(t, IsPath(m, r.Trace))
}

func TestCatchIsRecordedAndReplayedFromAHoldingState(t *testing.T) {
	m := passMachine()
	site := machine.Site{Kind: machine.SiteFail}
	sites := []machine.Site{site}
	step := machine.Step{ID: "task", Sites: sites}
	m.States[1].Steps = []machine.Step{step}
	catch := machine.Catch{On: machine.SiteFail, To: "done"}
	m.Catches = []machine.Catch{catch}
	r := quietRun(m)
	requireMove(t, r.Enter("start"))
	if err := r.Catch("start", machine.SiteFail, "done"); err == nil {
		t.Fatal("a catch from a state without its site must be refused")
	}
	requireMove(t, r.Go("start", "Ok", "work"))
	if err := r.Catch("work", machine.SiteFail, "start"); err == nil {
		t.Fatal("a catch to an undeclared destination must be refused")
	}
	requireMove(t, r.Catch("work", machine.SiteFail, "done"))
	requireMove(t, IsPath(m, r.Trace))
	if r.At() != "done" || len(r.Trace) != 3 {
		t.Fatalf("want one accepted catch, got state %q and trace %v", r.At(), r.Trace)
	}
	move := Transition{From: "start", Catch: machine.SiteFail, To: "done"}
	checkNotPath(t, m, "catch without a matching site", r.Trace[0], move)
}

func requireMove(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// A load fault enters retry, whose limited edge returns to load.
func catchRetryMachine() *machine.Machine {
	sites := []machine.Site{{Kind: machine.SiteFault}}
	steps := []machine.Step{{ID: "load-job", Sites: sites}}
	load := machine.State{ID: "load", Steps: steps}
	edges := []machine.Edge{{On: "again", To: "load", Kind: machine.EdgeLoop, Name: "Retry", Limit: 3}}
	retry := machine.State{ID: "retry", Edges: edges}
	catches := []machine.Catch{{On: machine.SiteFault, To: "retry"}}
	return &machine.Machine{Name: "example-retry", States: []machine.State{load, retry}, Catches: catches}
}

func TestCatchRetryLimitIsSharedByLiveAndReplay(t *testing.T) {
	m := catchRetryMachine()
	r := quietRun(m)
	requireMove(t, r.Enter("load"))
	for range 3 {
		requireMove(t, r.Catch("load", machine.SiteFault, "retry"))
		requireMove(t, r.Go("retry", "Retry", "load"))
	}
	requireMove(t, r.Catch("load", machine.SiteFault, "retry"))
	requireMove(t, IsPath(m, r.Trace))
	retry := Transition{From: "retry", Edge: "Retry", To: "load"}
	overLimit := append(slices.Clone(r.Trace), retry)
	if err := r.Go("retry", "Retry", "load"); !errors.Is(err, ErrLimit) {
		t.Errorf("live run accepted a fourth retry: %v", err)
	}
	if r.At() != "retry" || len(r.Trace) != 8 {
		t.Errorf("rejected retry changed state or trace: %q, %v", r.At(), r.Trace)
	}
	if err := IsPath(m, overLimit); !errors.Is(err, ErrLimit) {
		t.Errorf("replay accepted a fourth retry: %v", err)
	}
	fresh := quietRun(m)
	requireMove(t, fresh.Enter("load"))
	requireMove(t, fresh.Catch("load", machine.SiteFault, "retry"))
	requireMove(t, fresh.Go("retry", "Retry", "load"))
}

// selfCatchMachine: work holds a fault site and catches it back into work
// (limit 2); work --Finish--> done.
func selfCatchMachine() *machine.Machine {
	sites := []machine.Site{{Kind: machine.SiteFault}}
	steps := []machine.Step{{ID: "task", Kind: "task", Sites: sites}}
	work := machine.State{ID: "work", Steps: steps, Edges: []machine.Edge{edge("Finish", "done")}}
	done := machine.State{ID: "done", Terminal: true}
	catch := machine.Catch{On: machine.SiteFault, To: "work", Limit: 2}
	states := []machine.State{work, done}
	return &machine.Machine{Name: "self-catch", States: states, Catches: []machine.Catch{catch}}
}

// Regression: a catch move was never counted against a limit,
// so a state that caught its own fault back into itself was an unbounded pass.
func TestASelfCatchStopsAtItsLimit(t *testing.T) {
	m := selfCatchMachine()
	r := quietRun(m)
	requireMove(t, r.Enter("work"))
	for range m.Catches[0].Limit {
		requireMove(t, r.Catch("work", machine.SiteFault, "work"))
	}
	if err := r.Catch("work", machine.SiteFault, "work"); !errors.Is(err, ErrLimit) {
		t.Fatalf("the catch past its limit must be refused, got %v", err)
	}
	if r.At() != "work" || len(r.Trace) != 3 {
		t.Fatalf("a refused catch changed the run: state %q, trace %v", r.At(), r.Trace)
	}
	if len(r.Drift) != 1 {
		t.Fatalf("a refused catch is drift, got %v", r.Drift)
	}
	requireMove(t, IsPath(m, r.Trace))
	over := append(slices.Clone(r.Trace), r.Trace[1])
	if err := IsPath(m, over); !errors.Is(err, ErrLimit) {
		t.Fatalf("replay accepted the catch past its limit, got %v", err)
	}
}

// A catch limit counts per source state, like an edge limit: the same site
// kind caught from another state has its own count.
func TestACatchLimitCountsPerSourceState(t *testing.T) {
	m := selfCatchMachine()
	late := machine.State{ID: "late", Steps: m.States[0].Steps, Edges: []machine.Edge{edge("Finish", "done")}}
	m.States = append(m.States, late)
	m.States[0].Edges = append(m.States[0].Edges, edge("Later", "late"))
	ownCount := machine.Catch{On: machine.SiteFault, To: "late", Limit: 1}
	m.Catches = append(m.Catches, ownCount)
	r := quietRun(m)
	requireMove(t, r.Enter("work"))
	requireMove(t, r.Catch("work", machine.SiteFault, "work"))
	requireMove(t, r.Catch("work", machine.SiteFault, "work"))
	requireMove(t, r.Go("work", "Later", "late"))
	requireMove(t, r.Catch("late", machine.SiteFault, "late"))
	if err := r.Catch("late", machine.SiteFault, "late"); !errors.Is(err, ErrLimit) {
		t.Fatalf("late has its own count of one, got %v", err)
	}
	requireMove(t, IsPath(m, r.Trace))
}

// TestGoRefusesAnEmptyEdgeName enforces no-silent-drift: a move that names no
// edge is refused (regression: where an empty name matched an
// edge declared without one and left a trace move that named nothing).
func TestGoRefusesAnEmptyEdgeName(t *testing.T) {
	m := passMachine()
	quiet := machine.Edge{On: "quietly", To: "work", Kind: machine.EdgeNext}
	m.States[0].Edges = append(m.States[0].Edges, quiet)
	r := quietRun(m)
	requireMove(t, r.Enter("start"))
	if err := r.Go("start", "", "work"); err == nil {
		t.Fatal("an empty edge name must be refused even where an unnamed edge leads")
	}
	if r.At() != "start" || len(r.Trace) != 1 {
		t.Fatalf("a refused move changed the run: state %q, trace %v", r.At(), r.Trace)
	}
	if len(r.Drift) != 1 {
		t.Fatalf("an empty edge name is drift, got %v", r.Drift)
	}
	unnamed := Transition{From: "start", To: "work"}
	checkNotPath(t, m, "a move that names no edge", Transition{Edge: EnterEdge, To: "start"}, unnamed)
}
