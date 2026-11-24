package machine

import (
	"errors"
	"strings"
	"testing"
)

func noViolations(*Machine) []string { return nil }

func alwaysBroken(*Machine) []string { return []string{"broken"} }

func TestCheckLawsRefusesAnEmptyList(t *testing.T) {
	_, err := CheckLaws(newMachine(terminal("done")), nil)
	if !errors.Is(err, ErrNoLaws) {
		t.Fatalf("zero laws must be an error, got %v", err)
	}
}

func TestCheckLawsRefusesMalformedLaws(t *testing.T) {
	nameless := Law{Check: noViolations}
	checkless := Law{Name: "a"}
	lawA := Law{Name: "a", Check: noViolations}
	checkMalformedLaws(t, "has no name", nameless)
	checkMalformedLaws(t, "has no Check", checkless)
	checkMalformedLaws(t, "listed twice", lawA, lawA)
}

func checkMalformedLaws(t *testing.T, want string, laws ...Law) {
	t.Helper()
	_, err := CheckLaws(newMachine(terminal("done")), laws)
	if err == nil {
		t.Errorf("want an error with %q, got none", want)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("want an error with %q, got %v", want, err)
	}
}

// needsDone holds while the machine has a state named done.
func needsDone(m *Machine) []string {
	if m.State("done") == nil {
		return []string{"no done state"}
	}
	return nil
}

// renameDone keeps the entry but removes the done state that needsDone checks.
func renameDone(m *Machine) {
	m.States[1].ID = "finished"
	m.States[0].Edges[0].To = "finished"
}

func TestVerdictsSeparateBrokenFromPending(t *testing.T) {
	broken := Law{Name: "broken", Check: alwaysBroken, Witness: renameDone}
	pending := Law{Name: "pending", Check: alwaysBroken, Witness: renameDone, Pending: "waits on the owner"}
	holds := Law{Name: "holds", Check: needsDone, Witness: renameDone}
	m := newMachine(working("start", next("done")), terminal("done"))
	verdicts, err := CheckLaws(m, []Law{broken, pending, holds})
	if err != nil {
		t.Fatal(err)
	}
	if !verdicts[0].Broken() || verdicts[0].IsPending() {
		t.Error("a violation without Pending is broken")
	}
	if verdicts[1].Broken() || !verdicts[1].IsPending() {
		t.Error("a violation with Pending is pending, not broken")
	}
	if !verdicts[2].Holds() {
		t.Error("no violation holds")
	}
}
