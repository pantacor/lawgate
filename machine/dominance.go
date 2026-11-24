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
// the entry state, where a and b are step ids and each means every
// occurrence of that id (see StepRef): every occurrence of b is preceded,
// on every path from the entry state to it, by some occurrence of a. It is
// AlwaysBeforeRef with no state named.
func (m *Machine) AlwaysBefore(a, b string) bool {
	return m.AlwaysBeforeRef(StepRef{Step: a}, StepRef{Step: b})
}

// AlwaysBeforeRef reports whether some step a names runs before each step b
// names, on every path from the entry state. It is false when either names
// no declared step, when a and b are the same reference, and when some path
// reaches an occurrence of b without passing an occurrence of a first. An
// occurrence of b that a also names does not precede itself: some other
// occurrence of a must run before it.
func (m *Machine) AlwaysBeforeRef(a, b StepRef) bool {
	before, after := m.positions(a), m.positions(b)
	declared := len(before) > 0 && len(after) > 0
	if !declared || a == b {
		return false
	}
	start, ok := m.entryPosition()
	if !ok {
		return false
	}
	starts := []position{start}
	return !m.reachesAvoiding(starts, positionSet(after), positionSet(before))
}

// reachesAvoiding reports whether some path from one of starts reaches a
// goal without passing through a position in avoid. A goal is checked
// before avoid, so reaching a goal that is also in avoid still counts:
// nothing in avoid ran before it.
func (m *Machine) reachesAvoiding(starts []position, goals, avoid map[position]bool) bool {
	seen := map[position]bool{}
	queue := starts
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		if goals[at] {
			return true
		}
		if seen[at] || avoid[at] {
			continue
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
