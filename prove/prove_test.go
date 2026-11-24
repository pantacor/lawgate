package prove

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func even() Law[int] {
	return Law[int]{Name: "even", Holds: func(i int) error {
		if i%2 != 0 {
			return errors.New("odd")
		}
		return nil
	}}
}

func notSeven() Law[int] {
	return Law[int]{Name: "not-seven", Holds: func(i int) error {
		if i == 7 {
			return errors.New("seven")
		}
		return nil
	}}
}

// recorder is a testing.TB that keeps what Check said. Fatal ends the
// goroutine, as it does in a real test.
type recorder struct {
	testing.TB
	mu     sync.Mutex
	fatal  string
	errors []string
	logs   []string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatal(args ...any) {
	r.mu.Lock()
	r.fatal = fmt.Sprint(args...)
	r.mu.Unlock()
	runtime.Goexit()
}

func (r *recorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

// runCheck runs Check on a recorder in its own goroutine, so a Fatal ends
// only that goroutine.
func runCheck(laws ...Law[int]) *recorder {
	rec := &recorder{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Check(rec, Ints(1, 10), laws...)
	}()
	<-done
	return rec
}

// Regression: Check passed, logging "0 laws held",
// when it was given no law.
func TestCheckFailsWithNoLaws(t *testing.T) {
	rec := runCheck()
	if !strings.Contains(rec.fatal, "no laws") {
		t.Fatalf("zero laws must fail the test, got fatal %q", rec.fatal)
	}
}

func TestAllRefusesMalformedLaws(t *testing.T) {
	var holdsNothing Law[int]
	checkRefused(t, "has no name", namedLaw("", even()))
	checkRefused(t, "has no Holds", namedLaw("x", holdsNothing))
	checkRefused(t, "listed twice", even(), even())
}

func checkRefused(t *testing.T, want string, laws ...Law[int]) {
	t.Helper()
	_, err := All(Ints(1, 3), laws...)
	if err == nil {
		t.Errorf("want an error with %q, got none", want)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("want an error with %q, got %v", want, err)
	}
}

func namedLaw(name string, law Law[int]) Law[int] {
	law.Name = name
	return law
}

// Regression: "held" was logged even when a law broke.
func TestCheckSaysHeldOnlyWhenTheLawsHeld(t *testing.T) {
	rec := runCheck(even())
	if len(rec.errors) == 0 {
		t.Fatal("odd inputs break even")
	}
	for _, line := range rec.logs {
		if strings.Contains(line, "held") {
			t.Fatalf("a broken run must not say the laws held: %q", line)
		}
	}
	if held := runCheck(alwaysHolds()); len(held.logs) != 1 {
		t.Fatalf("a passing run says so once, got %v", held.logs)
	}
}

// alwaysHolds is a law no input breaks, for a passing run.
func alwaysHolds() Law[int] {
	return Law[int]{Name: "holds", Holds: func(int) error { return nil }}
}

// Regression: one global cap on listed violations let an early,
// noisy law hide every violation of a later one.
func TestEveryLawIsCountedOnItsOwn(t *testing.T) {
	result, err := All(Ints(1, 100), even(), notSeven())
	if err != nil {
		t.Fatal(err)
	}
	countsAreExact := result.BrokenBy("even") == 50 && result.BrokenBy("not-seven") == 1
	if result.Checked != 100 || !countsAreExact {
		t.Fatalf("got %+v", result)
	}
	if len(result.Laws[0].Examples) != ExamplesPerLaw || len(result.Laws[1].Examples) != 1 {
		t.Fatalf("each law lists its own examples, got %+v", result.Laws)
	}
	if result.Broken() != 51 {
		t.Fatalf("total broken %d, want 51", result.Broken())
	}
}

// count drains at most limit values of seq, so a wrapping iterator cannot
// hang the test.
func count(values func(func(int) bool), limit int) []int {
	var seen []int
	for value := range values {
		seen = append(seen, value)
		if len(seen) == limit {
			break
		}
	}
	return seen
}

// Regression: Ints(lo, math.MaxInt) incremented past MaxInt,
// wrapped to MinInt and never ended.
func TestIntsStopsAtTheUpperBound(t *testing.T) {
	if got := count(Ints(math.MaxInt, math.MaxInt), 3); len(got) != 1 {
		t.Fatalf("Ints(MaxInt, MaxInt) yielded %v", got)
	}
	if got := count(Ints(math.MaxInt-1, math.MaxInt), 3); len(got) != 2 {
		t.Fatalf("Ints(MaxInt-1, MaxInt) yielded %v", got)
	}
	if got := count(Ints(math.MinInt, math.MinInt+1), 3); len(got) != 2 {
		t.Fatalf("Ints(MinInt, MinInt+1) yielded %v", got)
	}
	if got := count(Ints(3, 1), 3); len(got) != 0 {
		t.Fatalf("an empty range yielded %v", got)
	}
}

func TestProductAndConcatCoverTheSpace(t *testing.T) {
	n := 0
	for range Product(Of("a", "b", "c"), Concat(Ints(0, 1), Of(7))) {
		n++
	}
	if n != 9 {
		t.Fatalf("product size %d, want 9", n)
	}
}

func TestSequencesStopWhenTheConsumerStops(t *testing.T) {
	for range Product(Of(1, 2), Of(1, 2)) {
		break
	}
	for range Concat(Of(1), Of(2)) {
		break
	}
}
