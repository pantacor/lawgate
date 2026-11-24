package machine

import (
	"errors"
	"fmt"

	"github.com/pantacor/lawgate/bite"
)

// A Law is an invariant over a declared machine. Machines are small finite
// graphs, so a law is checked over every state and every path, not sampled.
//
// A law holds for the declaration. Package resolve ties the declaration to
// the code's tagged sites; a Go branch with no tagged site is invisible to
// both (see README.md, "What a law over a machine does not see").
type Law struct {
	Name string
	Doc  string
	// Check returns one line per violation; none means the law holds.
	Check func(m *Machine) []string
	// Witness changes an isolated copy of the machine so Check reports a
	// violation without changing the machine name or entry state.
	Witness func(m *Machine)
	// Pending is set while a known violation waits on a decision: the law is
	// checked and its violations are reported, but they do not fail the
	// gate. Setting or clearing it is a rule change, reviewed like any other
	// change to a law. It says why.
	Pending string
}

// Verdict is how one law fared on one machine.
type Verdict struct {
	Law        Law
	Violations []string
}

// Holds is true when the law found nothing.
func (v Verdict) Holds() bool { return len(v.Violations) == 0 }

// Broken is true when the law found violations and is not pending.
func (v Verdict) Broken() bool { return !v.Holds() && v.Law.Pending == "" }

// IsPending is true when the law found violations that wait on a decision.
func (v Verdict) IsPending() bool { return !v.Holds() && v.Law.Pending != "" }

// ErrNoLaws is returned by CheckLaws when it is given no law: a run that
// checks nothing proves nothing.
var ErrNoLaws = errors.New("no laws to check")

// CheckLaws runs every law over m. Each law must have a unique name, a check
// and a witness. A law that holds must break on its witness. A law that
// already reports violations keeps its original verdict without a witness run.
func CheckLaws(m *Machine, laws []Law) ([]Verdict, error) {
	if err := validateLaws(laws); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("no machine to check")
	}
	verdicts := make([]Verdict, 0, len(laws))
	for _, law := range laws {
		verdict, err := checkLaw(m, law)
		if err != nil {
			return nil, err
		}
		verdicts = append(verdicts, verdict)
	}
	return verdicts, nil
}

func checkLaw(m *Machine, law Law) (Verdict, error) {
	if bite.Off("lawgate/laws-bite") {
		return Verdict{Law: law, Violations: law.Check(m)}, nil
	}
	if law.Witness == nil {
		return Verdict{}, fmt.Errorf("law %s has no Witness", law.Name)
	}
	verdict := Verdict{Law: law, Violations: law.Check(m)}
	if !verdict.Holds() {
		return verdict, nil
	}
	return verdict, checkWitness(m, law)
}

func validateLaws(laws []Law) error {
	if len(laws) == 0 {
		return ErrNoLaws
	}
	seen := map[string]bool{}
	for i, law := range laws {
		if law.Name == "" {
			return fmt.Errorf("law %d has no name", i)
		}
		if law.Check == nil {
			return fmt.Errorf("law %s has no Check", law.Name)
		}
		if seen[law.Name] {
			return fmt.Errorf("law %s is listed twice", law.Name)
		}
		seen[law.Name] = true
	}
	return nil
}

// Siteline names a site for a violation: its tag (or match) and resolved line.
func Siteline(st *State, step *Step, site *Site) string {
	id := site.Tag
	if id == "" {
		id = fmt.Sprintf("%q", site.Match)
	}
	if site.Line > 0 {
		return fmt.Sprintf("%s (line %d) in %s/%s", id, site.Line, st.ID, step.ID)
	}
	return fmt.Sprintf("%s in %s/%s", id, st.ID, step.ID)
}
