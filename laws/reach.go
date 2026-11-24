package laws

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// ReachesOneOf builds a law named name: every working state (neither
// terminal nor a wait) can reach one of targets on some path. A target that
// is not declared, or no target at all, is a violation, so the law never
// holds by default. Waits are left out: a parked job resumes in a later
// pass; NoStuck is the law that also covers them. Its witness adds a working
// state that only loops back to itself after the entry state.
func ReachesOneOf(name, doc string, targets ...string) machine.Law {
	wanted := slices.Clone(targets)
	return machine.Law{
		Name:    name,
		Doc:     doc,
		Check:   func(m *machine.Machine) []string { return unreachedTargets(m, wanted) },
		Witness: addEndlessLoop,
	}
}

func unreachedTargets(m *machine.Machine, targets []string) []string {
	if len(targets) == 0 {
		return []string{"no target states are named"}
	}
	violations := undeclaredTargets(m, targets)
	isTarget := func(st *machine.State) bool { return slices.Contains(targets, st.ID) }
	for _, st := range m.States {
		if st.Terminal || st.Wait {
			continue
		}
		if !m.Reaches(st.ID, isTarget) {
			violations = append(violations, fmt.Sprintf("state %q never reaches %s", st.ID, strings.Join(targets, ", ")))
		}
	}
	return violations
}

func undeclaredTargets(m *machine.Machine, targets []string) []string {
	var violations []string
	for _, target := range targets {
		if m.State(target) == nil {
			violations = append(violations, fmt.Sprintf("target state %q is not declared", target))
		}
	}
	return violations
}

// NoStuck builds a law named name: every state that is not terminal, waits
// included, can reach a terminal state on some path, so no job stays active
// or parked forever. With statuses, the goal is a terminal state whose
// Status is one of them, and a terminal state showing another status is a
// violation too. A machine with no states is a violation. Its witness adds a
// working state that only loops back to itself after the entry state.
func NoStuck(name, doc string, statuses ...string) machine.Law {
	wanted := slices.Clone(statuses)
	return machine.Law{
		Name:    name,
		Doc:     doc,
		Check:   func(m *machine.Machine) []string { return stuckStates(m, wanted) },
		Witness: addEndlessLoop,
	}
}

func stuckStates(m *machine.Machine, statuses []string) []string {
	if len(m.States) == 0 {
		return []string{"the machine has no states"}
	}
	ends := terminalGoal(statuses)
	var violations []string
	for _, st := range m.States {
		if st.Terminal {
			violations = append(violations, unlistedTerminal(st, statuses)...)
			continue
		}
		if !m.Reaches(st.ID, ends) {
			violations = append(violations, fmt.Sprintf("state %q never reaches a terminal state%s", st.ID, showing(statuses)))
		}
	}
	return violations
}

// terminalGoal is true for a terminal state, and with statuses only for one
// that shows one of them.
func terminalGoal(statuses []string) func(*machine.State) bool {
	return func(st *machine.State) bool {
		listed := len(statuses) == 0 || slices.Contains(statuses, st.Status)
		return st.Terminal && listed
	}
}

func unlistedTerminal(st machine.State, statuses []string) []string {
	if len(statuses) == 0 || slices.Contains(statuses, st.Status) {
		return nil
	}
	return []string{fmt.Sprintf("terminal state %q shows %q, not %s", st.ID, st.Status, strings.Join(statuses, ", "))}
}

func showing(statuses []string) string {
	if len(statuses) == 0 {
		return ""
	}
	return " showing " + strings.Join(statuses, ", ")
}

// NoLimitGuard is the guard text WaitsEnd reads as "this wait has no limit
// of its own", compared without case.
const NoLimitGuard = "no limit"

// WaitsEnd builds a law named name: every wait ends. An edge into a wait
// state whose Guard says NoLimitGuard is allowed only when that wait
// declares what wakes it (State.WokenBy). allow lists, as "from→to", the
// waits that have neither yet and that the owner has not decided: each one
// is a known gap the law reports nothing for, not an exception it grants.
// Its witness adds an unlimited wait with no waker after the entry state.
func WaitsEnd(name, doc string, allow ...string) machine.Law {
	allowed := slices.Clone(allow)
	return machine.Law{
		Name:    name,
		Doc:     doc,
		Check:   func(m *machine.Machine) []string { return endlessWaits(m, allowed) },
		Witness: addEndlessWait,
	}
}

func endlessWaits(m *machine.Machine, allowed []string) []string {
	var violations []string
	for _, st := range m.States {
		for _, edge := range st.Edges {
			violations = append(violations, endlessWait(m, st.ID, edge, allowed)...)
		}
	}
	return violations
}

func endlessWait(m *machine.Machine, from string, edge machine.Edge, allowed []string) []string {
	to := m.State(edge.To)
	if to == nil || !to.Wait {
		return nil
	}
	unlimited := strings.Contains(strings.ToLower(edge.Guard), NoLimitGuard)
	woken := strings.TrimSpace(to.WokenBy) != ""
	known := slices.Contains(allowed, from+"→"+edge.To)
	endsOrIsKnown := !unlimited || woken || known
	if endsOrIsKnown {
		return nil
	}
	return []string{fmt.Sprintf("state %q waits in %q on %q with no limit and nothing that wakes it", from, edge.To, edge.On)}
}

func addEndlessWait(m *machine.Machine) {
	edge := witnessEdge()
	edge.Guard = NoLimitGuard
	addStateAfterEntry(m, machine.State{ID: witnessID + "-wait", Wait: true}, edge)
}
