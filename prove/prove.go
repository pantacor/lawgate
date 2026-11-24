// Package prove checks a kernel's laws on every input inside declared bounds
// (layer 4 of the proof gate).
//
// A kernel is a small pure function whose bug class matters: revision
// numbering, an access filter, a token check. Its inputs are bounded: enums,
// booleans, counters, small integer ranges. All enumerates the whole bounded
// input space and runs every law on every input, so within the bounds a pass
// is a proof, not a sample. The bounds are declared next to the kernel, where
// a reader sees what is covered; Go's fuzzing searches beyond them.
package prove

import (
	"errors"
	"fmt"
	"iter"
	"testing"
)

// Law is one property a kernel must keep. Holds returns nil when the input
// keeps it, or an error saying how it is broken.
type Law[T any] struct {
	Name  string
	Doc   string
	Holds func(T) error
}

// ExamplesPerLaw caps the violations each law lists. Every violation is
// still counted, per law, so a law with many violations never hides another.
const ExamplesPerLaw = 10

// LawResult is how one law fared over the whole space.
type LawResult struct {
	Name     string
	Broken   int      // every input that broke the law
	Examples []string // "input: why", at most ExamplesPerLaw
}

// Result is what All found.
type Result struct {
	Checked int         // inputs checked
	Laws    []LawResult // one per law, in the order given
}

// Broken counts every violation of every law.
func (r Result) Broken() int {
	total := 0
	for _, law := range r.Laws {
		total += law.Broken
	}
	return total
}

// BrokenBy counts the violations of the law named name.
func (r Result) BrokenBy(name string) int {
	for _, law := range r.Laws {
		if law.Name == name {
			return law.Broken
		}
	}
	return 0
}

// ErrNoLaws is returned when All is given no law: proving nothing is not a
// pass.
var ErrNoLaws = errors.New("prove: no laws to check")

// All runs every law on every input. It never stops early, so Checked is the
// size of the space and each law's Broken counts all its violations. It
// refuses an empty list, a law without a name or a Holds, and two laws of one
// name.
func All[T any](inputs iter.Seq[T], laws ...Law[T]) (Result, error) {
	if err := validate(laws); err != nil {
		return Result{}, err
	}
	result := Result{Laws: make([]LawResult, len(laws))}
	for i, law := range laws {
		result.Laws[i].Name = law.Name
	}
	for input := range inputs {
		result.Checked++
		checkInput(result.Laws, laws, input)
	}
	return result, nil
}

func checkInput[T any](results []LawResult, laws []Law[T], input T) {
	for i, law := range laws {
		err := law.Holds(input)
		if err == nil {
			continue
		}
		results[i].Broken++
		if len(results[i].Examples) < ExamplesPerLaw {
			results[i].Examples = append(results[i].Examples, fmt.Sprintf("%+v: %s", input, err))
		}
	}
}

func validate[T any](laws []Law[T]) error {
	if len(laws) == 0 {
		return ErrNoLaws
	}
	seen := map[string]bool{}
	for i, law := range laws {
		if law.Name == "" {
			return fmt.Errorf("prove: law %d has no name", i)
		}
		if law.Holds == nil {
			return fmt.Errorf("prove: law %s has no Holds", law.Name)
		}
		if seen[law.Name] {
			return fmt.Errorf("prove: law %s is listed twice", law.Name)
		}
		seen[law.Name] = true
	}
	return nil
}

// Check runs All in a test. It fails the test on a malformed law list, on an
// empty space (a domain that yields nothing proves nothing) and on any
// violation, and says the laws held only when they did.
func Check[T any](t testing.TB, inputs iter.Seq[T], laws ...Law[T]) Result {
	t.Helper()
	result, err := All(inputs, laws...)
	if err != nil {
		t.Fatal(err)
	}
	if result.Checked == 0 {
		t.Fatal("prove: the input space is empty, so nothing was checked")
	}
	for _, law := range result.Laws {
		reportLaw(t, law)
	}
	if result.Broken() == 0 {
		t.Logf("prove: %d laws held on all %d inputs", len(laws), result.Checked)
	}
	return result
}

func reportLaw(t testing.TB, law LawResult) {
	t.Helper()
	for _, example := range law.Examples {
		t.Errorf("%s: %s", law.Name, example)
	}
	if hidden := law.Broken - len(law.Examples); hidden > 0 {
		t.Errorf("%s: %d more violations not listed", law.Name, hidden)
	}
}
