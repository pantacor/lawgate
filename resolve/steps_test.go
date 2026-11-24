package resolve

import (
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

// closingMachine runs step close in two states, each with its own tagged
// site, the way Fleet repeats a Hub API step name across states.
func closingMachine(leftTag, rightTag string) *machine.Machine {
	left := closingState("left", leftTag, "right")
	right := closingState("right", rightTag, "done")
	end := machine.State{ID: "done", Terminal: true}
	states := []machine.State{left, right, end}
	return &machine.Machine{Name: "x", States: states}
}

func closingState(id, tag, to string) machine.State {
	sites := []machine.Site{tagSite(tag, machine.SiteFail)}
	steps := []machine.Step{{ID: "close", Kind: "store", Sites: sites}}
	edges := []machine.Edge{{On: "ok", To: to, Kind: machine.EdgeNext}}
	return machine.State{ID: id, Steps: steps, Edges: edges}
}

const closingSource = `package x

func left() {
	FailJob("X_JOB 1: left gone")
}

func right() {
	FailJob("X_JOB 2: right gone")
}
`

// Each occurrence of a repeated step id binds its own site and is named by
// its state in a disagreement.
func TestResolveBindsEachOccurrenceOfAStepID(t *testing.T) {
	m := closingMachine("1", "2")
	if got := resolveText(m, closingSource, testVocabulary()); got != "" {
		t.Fatalf("want no drift for one step id in two states, got:\n%s", got)
	}
	leftLine := m.StepIn("left", "close").Sites[0].Line
	rightLine := m.StepIn("right", "close").Sites[0].Line
	if leftLine != 4 || rightLine != 8 {
		t.Fatalf("left/close bound to line %d, right/close to %d; want 4 and 8", leftLine, rightLine)
	}
	got := resolveText(closingMachine("1", "3"), closingSource, testVocabulary())
	if !strings.Contains(got, `right/close: tag "3"`) {
		t.Fatalf("the missing site must be named by its state, got:\n%s", got)
	}
}
