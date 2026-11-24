package machine

import (
	"strings"
	"testing"
)

// TestCheckLawsRequiresABreakingWitness enforces laws-bite by checking
// that an absent witness or one that keeps the law cannot pass.
func TestCheckLawsRequiresABreakingWitness(t *testing.T) {
	law := approvalLaw()
	law.Witness = nil
	checkInvalidWitness(t, "has no Witness", law)
	law.Witness = func(m *Machine) {
		m.Labels = append(m.Labels, "unrelated")
	}
	checkInvalidWitness(t, "witness did not break", law)
	law.Witness = func(m *Machine) {}
	checkInvalidWitness(t, "witness did not change", law)
}

func checkInvalidWitness(t *testing.T, want string, law Law) {
	t.Helper()
	m := approvalMachine(next("approving"))
	_, err := CheckLaws(m, []Law{law})
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("want an error with %q, got %v", want, err)
	}
}

// TestCheckLawsChecksTheUnmutatedMachine enforces laws-bite without allowing
// a witness to change the machine used for the real verdicts.
func TestCheckLawsChecksTheUnmutatedMachine(t *testing.T) {
	m := approvalMachine(next("approving"))
	verdicts, err := CheckLaws(m, []Law{approvalLaw()})
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 || !verdicts[0].Holds() {
		t.Fatalf("the real machine must pass, got %+v", verdicts)
	}
	if m.States[0].Edges[0].To != "approving" {
		t.Fatal("the witness changed the original machine")
	}
}

// This uses the bypass from TestAlwaysBeforeNeedsEveryPath as a witness.
func approvalLaw() Law {
	return Law{Name: "approval-before-install", Check: checkApproval, Witness: bypassApproval}
}

func checkApproval(m *Machine) []string {
	if !m.AlwaysBefore("approve", "install") {
		return []string{"install is reachable without approval"}
	}
	return nil
}

func bypassApproval(m *Machine) {
	start := &m.States[0]
	approval := start.Edges[0]
	start.Edges[0] = next("installing")
	start.Edges = append(start.Edges, approval)
}

func TestCheckLawsRejectsAMetadataOnlyWitness(t *testing.T) {
	law := Law{
		Name: "has-declaration",
		Check: func(m *Machine) []string {
			if m.Doc == "" {
				return []string{"documentation was removed"}
			}
			return nil
		},
		Witness: func(m *Machine) {
			m.Doc = ""
			m.States[0].Doc = "metadata only"
			m.Findings = []Finding{{Text: "metadata only"}}
		},
	}
	m := approvalMachine(next("approving"))
	m.Doc = "approval machine"
	_, err := CheckLaws(m, []Law{law})
	if err == nil || !strings.Contains(err.Error(), "witness did not change the machine") {
		t.Fatalf("a metadata-only witness must be rejected, got %v", err)
	}
}

func TestCheckLawsRejectsWitnessThatRenamesMachine(t *testing.T) {
	law := approvalLaw()
	law.Witness = func(m *Machine) {
		m.Name = "replacement"
		bypassApproval(m)
	}
	checkInvalidWitness(t, "witness changed the machine name", law)
}

func TestCheckLawsRejectsWitnessThatChangesEntry(t *testing.T) {
	law := approvalLaw()
	law.Witness = func(m *Machine) {
		m.Entry = "installing"
	}
	checkInvalidWitness(t, "witness changed the entry state", law)
}

func TestCheckLawsRejectsWitnessThatRemovesExplicitEntry(t *testing.T) {
	m := approvalMachine(next("approving"))
	m.Entry = m.EntryID()
	law := approvalLaw()
	law.Check = func(candidate *Machine) []string {
		if candidate.State(candidate.EntryID()) == nil {
			return []string{"entry is missing"}
		}
		return nil
	}
	law.Witness = func(candidate *Machine) {
		candidate.States = candidate.States[1:]
	}
	_, err := CheckLaws(m, []Law{law})
	if err == nil || !strings.Contains(err.Error(), "witness changed the entry state") {
		t.Fatalf("the explicit entry must still exist, got %v", err)
	}
}

func TestCheckLawsDoesNotCountWitnessWhenOriginalAlreadyBreaksLaw(t *testing.T) {
	law := approvalLaw()
	law.Check = alwaysBroken
	law.Witness = func(m *Machine) {
		t.Error("an already broken law must not count a witness")
		bypassApproval(m)
	}
	m := approvalMachine(next("approving"))
	verdicts, err := CheckLaws(m, []Law{law})
	if err != nil || len(verdicts) != 1 {
		t.Fatalf("want one verdict for the unmutated machine, got %v, %v", verdicts, err)
	}
	if !verdicts[0].Broken() {
		t.Fatalf("the already broken law must fail, got %+v", verdicts[0])
	}
}

func TestCheckLawsRejectsWitnessThatWipesMachine(t *testing.T) {
	law := approvalLaw()
	law.Check = func(m *Machine) []string {
		if m.Name == "" {
			return []string{"machine has no name"}
		}
		return nil
	}
	law.Witness = func(m *Machine) {
		*m = Machine{}
	}
	checkInvalidWitness(t, "witness changed the machine name", law)
}
