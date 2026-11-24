package laws

import (
	"fmt"

	"github.com/pantacor/lawgate/machine"
)

// witnessID is the base of every id a witness adds. A machine that already
// uses it gets a numbered variant, so a witness never declares a state id
// twice, nor a step id twice in one state (machine.Validate refuses both; a
// step id may repeat across states).
const witnessID = "witness"

// witnessKind is the step kind of a step a witness adds.
const witnessKind machine.StepKind = "witness"

// uniqueStateID returns base, or base with the first number that makes it
// an id no state of m uses.
func uniqueStateID(m *machine.Machine, base string) string {
	id := base
	for n := 2; m.State(id) != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

// uniqueStepID returns base, or base with the first number that makes it
// an id no step of st uses. Step ids only need to be unique in their state.
func uniqueStepID(st *machine.State, base string) string {
	id := base
	for n := 2; hasStep(st, id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func hasStep(st *machine.State, id string) bool {
	for _, step := range st.Steps {
		if step.ID == id {
			return true
		}
	}
	return false
}

// addEntryStep appends a step holding site to the entry state: the first
// state every job runs, so the site is on every path.
func addEntryStep(m *machine.Machine, site machine.Site) {
	entry := m.State(m.EntryID())
	if entry == nil {
		return
	}
	step := machine.Step{ID: uniqueStepID(entry, witnessID), Kind: witnessKind}
	step.Sites = []machine.Site{site}
	entry.Steps = append(entry.Steps, step)
}

// witnessEdge is the edge a witness adds from the entry state to a state it
// declares; addStateAfterEntry fills in its target.
func witnessEdge() machine.Edge {
	return machine.Edge{On: witnessID, Kind: machine.EdgeNext}
}

// addStateAfterEntry declares state under a fresh id and adds edge from the
// entry state to it, so a pass can reach it.
func addStateAfterEntry(m *machine.Machine, state machine.State, edge machine.Edge) {
	state.ID = uniqueStateID(m, state.ID)
	m.States = append(m.States, state)
	// Appending may move the states, so the entry is looked up afterwards.
	entry := m.State(m.EntryID())
	if entry == nil {
		return
	}
	edge.To = state.ID
	entry.Edges = append(entry.Edges, edge)
}

// addEndlessLoop declares, after the entry state, a working state whose only
// edge comes back to itself. machine.Validate accepts it (it has a way out
// and the entry reaches it), yet a job that enters it never ends.
func addEndlessLoop(m *machine.Machine) {
	id := uniqueStateID(m, witnessID+"-loop")
	again := machine.Edge{On: "again", To: id, Kind: machine.EdgeLoop}
	loop := machine.State{ID: id}
	loop.Edges = []machine.Edge{again}
	addStateAfterEntry(m, loop, witnessEdge())
}
