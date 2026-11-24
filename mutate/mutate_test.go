package mutate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseSubjectSplitsOnlyAfterTheLastSlash(t *testing.T) {
	pay := Subject{ImportPath: "example.org/shop", Func: "Pay"}
	method := Subject{ImportPath: "example.org/shop", Receiver: "Wallet", Func: "Pay"}
	check := Subject{ImportPath: "github.com/pantacor/lawgate/lawsdoc", Func: "Check"}
	lower := Subject{ImportPath: "shop", Func: "pay"}
	cases := map[string]Subject{
		"example.org/shop.Pay":                      pay,
		"`example.org/shop.Wallet.Pay`":             method,
		"github.com/pantacor/lawgate/lawsdoc.Check": check,
		"shop.pay": lower,
	}
	for reference, want := range cases {
		got, err := ParseSubject(reference)
		if err != nil || got != want {
			t.Errorf("ParseSubject(%q) = %+v, %v; want %+v", reference, got, err, want)
		}
	}
}

func TestParseSubjectRefusesWhatIsNotAFunction(t *testing.T) {
	for _, reference := range []string{"", "Pay", "example.org/shop", "example.org/shop.", "a/b.c.d.e", "a/b.1x", "a b/c.D"} {
		if got, err := ParseSubject(reference); err == nil {
			t.Errorf("ParseSubject(%q) = %+v, want an error", reference, got)
		}
	}
}

const wallet = `package shop

import "errors"

var ErrNotEnoughMoney = errors.New("not enough money")

type Wallet struct{ balance int }

func Pay(balance, price int) (int, error) {
	if price > balance {
		return balance, ErrNotEnoughMoney
	}
	return balance - price, nil
}

func (w *Wallet) Pay(price int) error {
	_, err := Pay(w.balance, price)
	return err
}
`

func writeSource(t *testing.T, source string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "wallet.go")
	if err := os.WriteFile(name, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

func findSubject(t *testing.T, source, reference string) Target {
	t.Helper()
	subject, err := ParseSubject(reference)
	if err != nil {
		t.Fatal(err)
	}
	target, err := Find([]string{writeSource(t, source)}, subject)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func TestMutantsBreakEveryGuardOfTheSubject(t *testing.T) {
	target := findSubject(t, wallet, "example.org/shop.Pay")
	mutants, total := target.Mutants(MaxMutants)
	want := []string{
		"negated the condition `price > balance`",
		"changed > to >= in `price > balance`",
		"replaced the condition `price > balance` of a guarded return by false",
	}
	if total != len(want) || !slices.Equal(descriptions(mutants), want) {
		t.Fatalf("mutants = %q (total %d), want %q", descriptions(mutants), total, want)
	}
	if !strings.Contains(string(mutants[1].Source), "if price >= balance {") || mutants[1].Line != 10 {
		t.Fatalf("flipped mutant at line %d:\n%s", mutants[1].Line, mutants[1].Source)
	}
}

func descriptions(mutants []Mutant) []string {
	var names []string
	for _, mutant := range mutants {
		names = append(names, mutant.Description)
	}
	return names
}

func TestMutantsFindTheMethodNotTheFunction(t *testing.T) {
	target := findSubject(t, wallet, "example.org/shop.Wallet.Pay")
	if mutants, total := target.Mutants(MaxMutants); total != 0 {
		t.Fatalf("the method has no guard, got %q", descriptions(mutants))
	}
	if target.Line() != 16 {
		t.Fatalf("method found at line %d, want 16", target.Line())
	}
}

// A subject whose guard was deleted (the moved-guard attack) has nothing for
// a test to depend on: it yields no mutant, which the gate refuses.
func TestMutantsOfAGuardlessSubjectAreNone(t *testing.T) {
	source := strings.Replace(wallet, "if price > balance {\n\t\treturn balance, ErrNotEnoughMoney\n\t}\n", "", 1)
	target := findSubject(t, source, "example.org/shop.Pay")
	if mutants, total := target.Mutants(MaxMutants); total != 0 {
		t.Fatalf("guardless subject produced %q", descriptions(mutants))
	}
}

const loops = `package shop

func Count(items []int, limit int) int {
	n := 0
	for i := 0; i < len(items); i++ {
		switch {
		case items[i] == limit:
			return -1
		case items[i] != 0:
			n++
		}
	}
	return n
}
`

func TestMutantsCoverLoopsAndSwitchCasesAndAreBounded(t *testing.T) {
	target := findSubject(t, loops, "shop.Count")
	all, total := target.Mutants(MaxMutants)
	// for: negate, < to <=; two cases: negate and flip each (the case
	// negation of == and the flip to != differ in text, not in meaning).
	if total != 6 || len(all) != 6 {
		t.Fatalf("mutants = %q", descriptions(all))
	}
	bounded, total := target.Mutants(2)
	if len(bounded) != 2 || total != 6 {
		t.Fatalf("bounded run = %d of %d, want 2 of 6", len(bounded), total)
	}
}

func TestDisabledReturnKeepsItsConditionCompiling(t *testing.T) {
	source := strings.Replace(wallet, "if price > balance {", "if over := price - balance; over > 0 {", 1)
	target := findSubject(t, source, "example.org/shop.Pay")
	mutants, _ := target.Mutants(MaxMutants)
	last := string(mutants[len(mutants)-1].Source)
	if !strings.Contains(last, "if over := price - balance; false && (over > 0) {") {
		t.Fatalf("disabled return mutant:\n%s", last)
	}
}

func TestFingerprintIgnoresCommentsAndLayoutButNotCode(t *testing.T) {
	base := fingerprint(t, wallet)
	commented := strings.Replace(wallet, "func Pay(", "// Pay pays.\nfunc Pay(", 1)
	commented = strings.Replace(commented, "if price > balance {", "if price > balance { // the law", 1)
	if fingerprint(t, commented) != base {
		t.Fatal("a comment changed the subject's fingerprint")
	}
	changed := strings.Replace(wallet, "price > balance", "price < 0", 1)
	if fingerprint(t, changed) == base {
		t.Fatal("a changed guard kept the subject's fingerprint")
	}
}

func fingerprint(t *testing.T, source string) string {
	t.Helper()
	sum, err := findSubject(t, source, "example.org/shop.Pay").Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func TestFindNamesAMissingSubject(t *testing.T) {
	subject, _ := ParseSubject("example.org/shop.Refund")
	_, err := Find([]string{writeSource(t, wallet)}, subject)
	if err == nil || !strings.Contains(err.Error(), "example.org/shop.Refund") {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteOverlayReplacesOnlyTheSubjectFile(t *testing.T) {
	dir := t.TempDir()
	mutant := Mutant{Source: []byte("package shop\n")}
	overlay, err := WriteOverlay(dir, "/repo/shop/wallet.go", mutant)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	var parsed overlayFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	replaced := parsed.Replace["/repo/shop/wallet.go"]
	written, err := os.ReadFile(replaced)
	onlyMutant := len(parsed.Replace) == 1 && string(written) == "package shop\n"
	if err != nil || !onlyMutant {
		t.Fatalf("overlay %s: %v", data, err)
	}
}

func TestCacheRemembersOnlyRecordedKeys(t *testing.T) {
	cache := Cache{Dir: filepath.Join(t.TempDir(), "mutation")}
	key := Key("subject", "proof")
	if cache.Killed(key) {
		t.Fatal("an empty cache reported a run")
	}
	if err := cache.RecordKilled(key); err != nil {
		t.Fatal(err)
	}
	if !cache.Killed(key) || cache.Killed(Key("subject", "changed proof")) {
		t.Fatal("the cache must answer for exactly the recorded key")
	}
	if (Cache{}).Killed(key) {
		t.Fatal("a disabled cache reported a run")
	}
}

// The readability rule names compound conditions, so a guard often lives in
// an assignment: it is mutated like the if that reads it.
const named = `package shop

func Allowed(balance, price int, frozen bool) bool {
	affordable := price <= balance && !frozen
	if !affordable {
		return false
	}
	return true
}
`

func TestMutantsReachANamedCondition(t *testing.T) {
	target := findSubject(t, named, "shop.Allowed")
	mutants, _ := target.Mutants(MaxMutants)
	got := descriptions(mutants)
	for _, want := range []string{
		"negated the condition `price <= balance && !frozen`",
		"changed <= to < in `price <= balance && !frozen`",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing mutant %q in %q", want, got)
		}
	}
}

// A predicate often returns its condition, or names it first: either is the
// guard its law depends on.
const predicates = `package shop

import "strings"

func HasPayload(payload string) bool {
	return strings.TrimSpace(payload) != ""
}

func Ready(count int) bool {
	var ready = count > 0
	return ready
}

func Closed(open bool) bool {
	return !open
}
`

func TestMutantsReachAReturnedCondition(t *testing.T) {
	cases := map[string][]string{
		"shop.HasPayload": {
			"negated the condition `strings.TrimSpace(payload) != \"\"`",
			"changed != to == in `strings.TrimSpace(payload) != \"\"`",
		},
		"shop.Ready": {
			"negated the condition `count > 0`",
			"changed > to >= in `count > 0`",
		},
		"shop.Closed": {"negated the condition `!open`"},
	}
	for reference, want := range cases {
		mutants, total := findSubject(t, predicates, reference).Mutants(MaxMutants)
		if total != len(want) || !slices.Equal(descriptions(mutants), want) {
			t.Errorf("%s: mutants = %q (total %d), want %q", reference, descriptions(mutants), total, want)
		}
	}
}

// Passing an error up unchanged is not law behaviour: those guards are
// counted, not mutated. Any other use of err in a guard is still mutated.
const propagation = `package shop

import (
	"errors"
	"fmt"
	"os"
)

var ErrClosed = errors.New("closed")

func Load(name string) ([]byte, int, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, 0, err
	}
	if _, err := os.Stat(name); err != nil {
		return nil, 0, fmt.Errorf("stat %s: %w", name, err)
	}
	if err := os.Chdir(name); err != nil {
		return nil, 0, errors.Join(ErrClosed, err)
	}
	if err = os.Remove(name); err != nil && !os.IsNotExist(err) {
		return nil, 0, err
	}
	if errors.Is(err, ErrClosed) {
		return nil, 1, err
	}
	if err != nil {
		return data, 0, err
	}
	return data, len(data), nil
}
`

func TestMutantsSkipErrorPropagation(t *testing.T) {
	target := findSubject(t, propagation, "shop.Load")
	if got := target.ErrorPropagations(); got != 3 {
		t.Errorf("error-propagation guards = %d, want 3", got)
	}
	mutants, _ := target.Mutants(MaxMutants)
	lines := map[int]bool{}
	for _, mutant := range mutants {
		lines[mutant.Line] = true
	}
	for _, line := range []int{13, 16, 19} {
		if lines[line] {
			t.Errorf("propagation guard at line %d was mutated: %q", line, descriptions(mutants))
		}
	}
	for _, line := range []int{22, 25, 28} {
		if !lines[line] {
			t.Errorf("guard at line %d was not mutated: %q", line, descriptions(mutants))
		}
	}
}
