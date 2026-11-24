package machine

// AlwaysBefore works on positions, not states, because a state's steps run in
// order and a catch can leave a state from the middle: a path that enters a's
// state and leaves through the catch of an earlier step never ran a.
//
// A position is a step about to run, or the end of a state (step ==
// len(Steps)), where its edges leave. From position i a pass goes to i+1, or
// through the catch of a site of step i; from the end it takes an edge to the
// first position of the target state.

type position struct {
	state int // index in Machine.States
	step  int // index in State.Steps; len(Steps) is the state's end
}

// AlwaysBefore reports whether step a runs before step b on EVERY path from
// the entry state to b. It is false when either step is not declared, when a
// and b are the same step, and when some path reaches b without passing a.
func (m *Machine) AlwaysBefore(a, b string) bool {
	posA, okA := m.stepPosition(a)
	posB, okB := m.stepPosition(b)
	twoDistinctSteps := okA && okB && a != b
	if !twoDistinctSteps {
		return false
	}
	start, ok := m.entryPosition()
	if !ok {
		return false
	}
	return !m.reachesAvoiding(start, posB, posA)
}

// reachesAvoiding reports whether some path from start reaches goal without
// passing through avoid.
func (m *Machine) reachesAvoiding(start, goal, avoid position) bool {
	seen := map[position]bool{avoid: true}
	queue := []position{start}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		if seen[at] {
			continue
		}
		if at == goal {
			return true
		}
		seen[at] = true
		queue = append(queue, m.positionsAfter(at)...)
	}
	return false
}

// positionsAfter lists the positions a pass can move to from at.
func (m *Machine) positionsAfter(at position) []position {
	st := &m.States[at.state]
	if at.step == len(st.Steps) {
		return m.firstPositions(edgeTargets(st))
	}
	var targets []string
	for _, site := range st.Steps[at.step].Sites {
		targets = append(targets, m.catchTargets(site.Kind)...)
	}
	next := position{state: at.state, step: at.step + 1}
	return append(m.firstPositions(targets), next)
}

func edgeTargets(st *State) []string {
	targets := make([]string, 0, len(st.Edges))
	for _, edge := range st.Edges {
		targets = append(targets, edge.To)
	}
	return targets
}

// firstPositions maps state ids to their first positions, dropping unknown
// states (Validate reports those).
func (m *Machine) firstPositions(ids []string) []position {
	var out []position
	for _, id := range ids {
		if index := m.stateIndex(id); index >= 0 {
			out = append(out, position{state: index})
		}
	}
	return out
}

func (m *Machine) entryPosition() (position, bool) {
	index := m.stateIndex(m.EntryID())
	return position{state: index}, index >= 0
}

func (m *Machine) stateIndex(id string) int {
	for i := range m.States {
		if m.States[i].ID == id {
			return i
		}
	}
	return -1
}

// stepPosition finds the step with id. Step ids are unique in a valid
// machine (Validate); the first one wins otherwise.
func (m *Machine) stepPosition(id string) (position, bool) {
	for i := range m.States {
		if index := m.States[i].stepIndex(id); index >= 0 {
			return position{state: i, step: index}, true
		}
	}
	return position{}, false
}

func (st *State) stepIndex(id string) int {
	for j := range st.Steps {
		if st.Steps[j].ID == id {
			return j
		}
	}
	return -1
}
