package machine

import "fmt"

// EntryEdgeName marks the start of a pass in its trace and is reserved
// from use as a declared edge name.
const EntryEdgeName = "enter"

// Validate checks that the declaration is a well-formed graph and returns one
// error per problem: a machine with no states, a duplicate state id, a step
// id declared twice in one state (it may repeat in other states: see
// StepRef), an entry, edge or catch that names an unknown state (a catch
// with an empty target carries on), a carry-on catch with a limit, a
// terminal state with edges, a working state with no way out, an edge with
// a negative limit, two edges of one state with one name, an edge with the
// reserved entry name, a catch with a negative limit, a catch back into a
// state that holds its own site kind without a limit, and a state no path
// from the entry state reaches. Laws assume a valid machine; package
// resolve runs Validate.
func Validate(m *Machine) []error {
	if len(m.States) == 0 {
		return []error{fmt.Errorf("machine %s has no states", m.Name)}
	}
	var errs []error
	errs = append(errs, duplicateIDs(m)...)
	errs = append(errs, unknownTargets(m)...)
	errs = append(errs, catchLimits(m)...)
	for i := range m.States {
		errs = append(errs, stateShape(&m.States[i])...)
	}
	return append(errs, unreachableStates(m)...)
}

func duplicateIDs(m *Machine) []error {
	var errs []error
	states := map[string]bool{}
	for _, st := range m.States {
		if states[st.ID] {
			errs = append(errs, fmt.Errorf("state %s is declared twice", st.ID))
		}
		states[st.ID] = true
		errs = append(errs, duplicateSteps(st)...)
	}
	return errs
}

// duplicateSteps reports each step id st declares more than once. Only the
// state is checked: products name steps after the work they do (Fleet's come
// from the Hub API) and the same work runs in several states, so a step id
// repeats across states and must not be renamed to look unique. The state
// and the step id together identify a step.
func duplicateSteps(st State) []error {
	var errs []error
	seen := map[string]bool{}
	for _, step := range st.Steps {
		if seen[step.ID] {
			errs = append(errs, fmt.Errorf("state %s declares step %s twice", st.ID, step.ID))
		}
		seen[step.ID] = true
	}
	return errs
}

func unknownTargets(m *Machine) []error {
	var errs []error
	if m.State(m.EntryID()) == nil {
		errs = append(errs, fmt.Errorf("entry state %s is not declared", m.EntryID()))
	}
	for _, catch := range m.Catches {
		unknown := !catch.CarriesOn() && m.State(catch.To) == nil
		if unknown {
			errs = append(errs, fmt.Errorf("catch on %s goes to unknown state %s", catch.On, catch.To))
		}
	}
	for _, st := range m.States {
		errs = append(errs, unknownEdgeTargets(m, st)...)
	}
	return errs
}

func unknownEdgeTargets(m *Machine, st State) []error {
	var errs []error
	for _, edge := range st.Edges {
		if m.State(edge.To) == nil {
			errs = append(errs, fmt.Errorf("state %s: edge %q goes to unknown state %s", st.ID, edge.On, edge.To))
		}
	}
	return errs
}

// catchLimits checks each catch rule's limit. A catch whose target holds the
// caught kind is a loop made only of catch moves: no edge of the declaration
// bounds it, so it must say how many times it may be taken.
func catchLimits(m *Machine) []error {
	var errs []error
	for _, catch := range m.Catches {
		errs = append(errs, catchShape(m, catch)...)
	}
	return errs
}

// catchShape checks one catch rule's limit.
func catchShape(m *Machine, catch Catch) []error {
	if catch.Limit < 0 {
		return []error{fmt.Errorf("catch on %s has a negative limit %d", catch.On, catch.Limit)}
	}
	if catch.CarriesOn() && catch.Limit > 0 {
		return []error{fmt.Errorf("catch on %s carries on, so its limit %d bounds nothing", catch.On, catch.Limit)}
	}
	if catch.Limit == 0 && catchesBackIntoTarget(m, catch) {
		return []error{fmt.Errorf("catch on %s goes back to %s, which holds a %s site: it needs a limit", catch.On, catch.To, catch.On)}
	}
	return nil
}

// catchesBackIntoTarget reports whether a fault in the catch's target state
// is caught back into that state.
func catchesBackIntoTarget(m *Machine, catch Catch) bool {
	target := m.State(catch.To)
	return target != nil && target.Holds(catch.On)
}

// stateShape checks one state's own edges.
func stateShape(st *State) []error {
	var errs []error
	if st.Terminal && len(st.Edges) > 0 {
		errs = append(errs, fmt.Errorf("terminal state %s has edges", st.ID))
	}
	endsPass := st.Terminal || st.Wait
	if !endsPass && len(st.Edges) == 0 {
		errs = append(errs, fmt.Errorf("state %s has no way out", st.ID))
	}
	names := map[string]bool{}
	for _, edge := range st.Edges {
		errs = append(errs, edgeShape(st.ID, edge, names)...)
	}
	return errs
}

func edgeShape(stateID string, edge Edge, names map[string]bool) []error {
	var errs []error
	if edge.Limit < 0 {
		errs = append(errs, fmt.Errorf("state %s: edge %q has a negative limit %d", stateID, edge.On, edge.Limit))
	}
	if edge.Name == "" {
		return errs
	}
	if edge.Name == EntryEdgeName {
		errs = append(errs, fmt.Errorf("state %s: edge name %q is reserved for the start of a pass", stateID, edge.Name))
	}
	if names[edge.Name] {
		errs = append(errs, fmt.Errorf("state %s: two edges are named %s", stateID, edge.Name))
	}
	names[edge.Name] = true
	return errs
}

// unreachableStates reports, in declaration order, every state no path from
// the entry state reaches.
func unreachableStates(m *Machine) []error {
	reached := m.Reachable()
	var errs []error
	for _, st := range m.States {
		if !reached[st.ID] {
			errs = append(errs, fmt.Errorf("state %s is never reached from the entry state %s", st.ID, m.EntryID()))
		}
	}
	return errs
}
