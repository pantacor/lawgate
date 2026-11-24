package machine

// A step id is not unique in a machine. Products name steps after the work
// they do, and the same work runs in several states (Fleet's appliance
// deployer has "close-open-children" in eleven states, named by the Hub
// API, which must not be renamed). Validate refuses a step id only when one
// state declares it twice: the state and the step id together identify a
// step. Every helper that takes a step therefore means every occurrence of
// its id, unless a StepRef names the state.

// StepRef names steps of a machine: the occurrence of step id Step in state
// State, or, with State empty, every occurrence of Step in any state.
type StepRef struct {
	State string
	Step  string
}

// String is "state/step", or the step id alone for every occurrence.
func (ref StepRef) String() string {
	if ref.State == "" {
		return ref.Step
	}
	return ref.State + "/" + ref.Step
}

// StepIn looks the step with id up in state; nil when that state does not
// declare it.
func (m *Machine) StepIn(state, id string) *Step {
	st := m.State(state)
	if st == nil {
		return nil
	}
	index := st.stepIndex(id)
	if index < 0 {
		return nil
	}
	return &st.Steps[index]
}

// StatesWithStep lists, in declaration order, the states that declare a
// step with id.
func (m *Machine) StatesWithStep(id string) []string {
	var states []string
	for i := range m.States {
		if m.States[i].stepIndex(id) >= 0 {
			states = append(states, m.States[i].ID)
		}
	}
	return states
}

// positions lists the positions of every step ref names, in declaration
// order; none when the step is not declared (in the named state).
func (m *Machine) positions(ref StepRef) []position {
	var found []position
	for i := range m.States {
		st := &m.States[i]
		named := ref.State == "" || ref.State == st.ID
		index := st.stepIndex(ref.Step)
		if named && index >= 0 {
			found = append(found, position{state: i, step: index})
		}
	}
	return found
}

func (st *State) stepIndex(id string) int {
	for j := range st.Steps {
		if st.Steps[j].ID == id {
			return j
		}
	}
	return -1
}

func positionSet(list []position) map[position]bool {
	set := make(map[position]bool, len(list))
	for _, at := range list {
		set[at] = true
	}
	return set
}
