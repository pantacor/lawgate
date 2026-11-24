package machine

// The graph helpers come in two strengths, and a law must use the one its
// words need:
//
//   - existential ("some path"): Reaches, SomePathBefore. Use them for "can
//     reach", "is possible".
//   - universal ("every path"): AlwaysBefore. Use it for "precedes", "only
//     after", "never without".
//
// Every walk starts from the entry state (EntryID), never from an arbitrary
// state, unless a helper takes a start state by name.

// Next lists the states st can go to: its edges, and the catch of each kind
// of site it holds. Unknown targets are left out here; Validate reports them.
func (m *Machine) Next(st *State) []string {
	var next []string
	seen := map[string]bool{}
	add := func(id string) {
		if seen[id] || m.State(id) == nil {
			return
		}
		seen[id] = true
		next = append(next, id)
	}
	for _, edge := range st.Edges {
		add(edge.To)
	}
	st.sites(func(_ *State, _ *Step, site *Site) {
		for _, target := range m.catchTargets(site.Kind) {
			add(target)
		}
	})
	return next
}

// catchTargets lists where a site of kind leads.
func (m *Machine) catchTargets(kind SiteKind) []string {
	var targets []string
	for _, catch := range m.Catches {
		if catch.On == kind {
			targets = append(targets, catch.To)
		}
	}
	return targets
}

// Reachable is the set of states some path from the entry state reaches,
// the entry state included.
func (m *Machine) Reachable() map[string]bool {
	seen := map[string]bool{}
	m.walk(m.EntryID(), func(st *State) bool {
		seen[st.ID] = true
		return false
	})
	return seen
}

// Reaches reports whether some path from state from ends in a state for
// which goal is true (from itself included).
func (m *Machine) Reaches(from string, goal func(*State) bool) bool {
	return m.walk(from, goal)
}

// walk visits the states reachable from start, breadth first, and stops at
// the first one for which stop is true. It reports whether it stopped.
func (m *Machine) walk(start string, stop func(*State) bool) bool {
	seen := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		st := m.State(id)
		if seen[id] || st == nil {
			continue
		}
		seen[id] = true
		if stop(st) {
			return true
		}
		queue = append(queue, m.Next(st)...)
	}
	return false
}

// SomePathBefore reports whether step a can run before step b on SOME path:
// both in one state with a first, or a's state reaches b's state. It is the
// existential helper; a law that says "a precedes b" needs AlwaysBefore.
func (m *Machine) SomePathBefore(a, b string) bool {
	posA, okA := m.stepPosition(a)
	posB, okB := m.stepPosition(b)
	if !okA || !okB {
		return false
	}
	if posA.state == posB.state && posA.step < posB.step {
		return true
	}
	// Start from the states after a's own, so that a step after b in one
	// state counts only when a loop comes back to that state.
	target := m.States[posB.state].ID
	isTarget := func(st *State) bool { return st.ID == target }
	for _, next := range m.Next(&m.States[posA.state]) {
		if m.walk(next, isTarget) {
			return true
		}
	}
	return false
}
