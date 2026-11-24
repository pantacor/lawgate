package machine

import (
	"fmt"
	"reflect"
	"slices"
)

func checkWitness(m *Machine, law Law) error {
	isolated := cloneMachine(m)
	law.Witness(isolated)
	if isolated.Name != m.Name {
		return fmt.Errorf("law %s witness changed the machine name", law.Name)
	}
	entry := m.EntryID()
	if isolated.EntryID() != entry || isolated.State(entry) == nil {
		return fmt.Errorf("law %s witness changed the entry state", law.Name)
	}
	if sameDeclaration(m, isolated) {
		return fmt.Errorf("law %s witness did not change the machine", law.Name)
	}
	if len(law.Check(isolated)) == 0 {
		return fmt.Errorf("law %s witness did not break the law", law.Name)
	}
	return nil
}

// A witness must change the declaration itself. Report metadata and
// callback identity do not change what the machine can do.
func sameDeclaration(a, b *Machine) bool {
	return reflect.DeepEqual(declaration(a), declaration(b))
}

func declaration(m *Machine) *Machine {
	declared := cloneMachine(m)
	declared.Doc = ""
	declared.Laws = nil
	declared.Findings = nil
	for i := range declared.States {
		state := &declared.States[i]
		state.Doc = ""
		for j := range state.Steps {
			state.Steps[j].Doc = ""
		}
	}
	declared.Sites(func(_ *State, _ *Step, site *Site) { site.Line = 0 })
	return declared
}

// Witnesses can edit the whole declaration, so no mutable slice may share
// storage with the machine passed to the real law checks.
func cloneMachine(m *Machine) *Machine {
	cloned := *m
	cloned.Labels = slices.Clone(m.Labels)
	cloned.Laws = slices.Clone(m.Laws)
	cloned.Catches = slices.Clone(m.Catches)
	cloned.States = slices.Clone(m.States)
	cloned.Findings = slices.Clone(m.Findings)
	for i := range cloned.States {
		cloneStateSlices(&cloned.States[i])
	}
	return &cloned
}

func cloneStateSlices(state *State) {
	state.Edges = slices.Clone(state.Edges)
	state.Steps = slices.Clone(state.Steps)
	for i := range state.Steps {
		step := &state.Steps[i]
		step.Sites = slices.Clone(step.Sites)
		for j := range step.Sites {
			site := &step.Sites[j]
			site.Labels = slices.Clone(site.Labels)
		}
	}
}
