package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/mutate"
)

// The wallet is the architecture council's fixture: one law,
// never-spend-more-than-you-have, guarded in Pay.
const spendSource = `package shop

import "errors"

var ErrNotEnoughMoney = errors.New("not enough money")

func Pay(balance, price int) (int, error) {
	if price > balance {
		return balance, ErrNotEnoughMoney
	}
	return balance - price, nil
}
`

// The honest proof checks both sides of the boundary.
const spendTestSource = `package shop

import "testing"

func TestPay(t *testing.T) {
	if left, err := Pay(36, 900); err == nil {
		t.Fatalf("spend-law: paid 900 with 36 in the wallet: balance is now %d", left)
	}
	if left, err := Pay(36, 36); err != nil || left != 0 {
		t.Fatalf("spend-law: paying the whole balance: %d, %v", left, err)
	}
}
`

// The council's base proof: one point, far from the boundary.
const spendOnePointTest = `package shop

import "testing"

func TestPay(t *testing.T) {
	if left, err := Pay(36, 900); err == nil {
		t.Fatalf("spend-law: paid 900 with 36 in the wallet: balance is now %d", left)
	}
}
`

const spendSubject = "example.test/shop.Pay"

func writeSpendModule(t *testing.T, root, source, test string) {
	t.Helper()
	writeFixture(t, root, "go.mod", "module example.test/shop\n\ngo 1.25\n")
	writeFixture(t, root, "wallet.go", source)
	writeFixture(t, root, "wallet_test.go", test)
}

// isolateMutationCache keeps each test's mutation runs out of the user's cache.
func isolateMutationCache(t *testing.T) mutate.Cache {
	t.Helper()
	cache := mutate.Cache{Dir: t.TempDir()}
	original := mutationCache
	t.Cleanup(func() { mutationCache = original })
	mutationCache = func() mutate.Cache { return cache }
	return cache
}

func goLaw(name, test string, subjects ...string) string {
	doc := biteLaw(name, "test", test)
	for _, subject := range subjects {
		doc += "- Subject: `" + subject + "`\n"
	}
	return doc
}

func spendModule(command ...string) inventory.Module {
	if len(command) == 0 {
		command = []string{"go", "test", "-json", "."}
	}
	return inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
}

func mutateSpend(t *testing.T, source, test, doc string) moduleResult {
	t.Helper()
	isolateMutationCache(t)
	root := t.TempDir()
	writeSpendModule(t, root, source, test)
	return runTestBite(root, spendModule(), doc)
}

// TestBiteRunsAgreedEnforcers enforces laws-bite: a Go law's subject is
// mutated and its tests must kill every mutant; a script must reject
// LAWGATE_BITE; a gap stays pending with its reason.
func TestBiteRunsAgreedEnforcers(t *testing.T) {
	isolateMutationCache(t)
	root := t.TempDir()
	writeSpendModule(t, root, spendSource, spendTestSource)
	weak := strings.ReplaceAll(spendOnePointTest, "TestPay", "TestWeak")
	weak = strings.Replace(weak, "spend-law:", "blind-law gap-law unnamed-law later-law:", 1)
	writeFixture(t, root, "weak_test.go", weak)
	writeExecutable(t, root, "rules/bites.sh", "#!/bin/sh\ntest -f go.mod || exit 2\n"+rejectScriptLaw("script-law"))
	writeExecutable(t, root, "rules/blind.sh", "#!/bin/sh\nexit 0\n")
	doc := goLaw("spend-law", "TestPay", spendSubject) + goLaw("blind-law", "TestWeak", spendSubject)
	doc += goLaw("gap-law", "TestWeak", spendSubject) + "- Bites: not yet. owner recorded a gap\n"
	doc += goLaw("unnamed-law", "TestWeak") + goLaw("later-law", "TestWeak") + "- Subject: not yet. Needs a kernel.\n"
	scripts := biteLaw("script-law", "script", "bites.sh") + biteLaw("blind-script", "script", "blind.sh")
	shared := lawDocument{Path: "rules/laws.md", Plans: lawsdoc.Plan(scripts)}
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(doc), SharedDocs: []lawDocument{shared}}
	checkTests(root, spendModule(), &result)
	checkScripts(root, shared, &result)
	checkBites(root, spendModule(), &result)
	expectBite(t, result, "spend-law", passed, "killed every mutant of "+spendSubject+" (3)")
	expectBite(t, result, "blind-law", failed, "wallet.go:8 changed > to >= in `price > balance`")
	expectBite(t, result, "gap-law", pending, "bite gap: owner recorded a gap")
	expectBite(t, result, "unnamed-law", failed, "names no subject", "no subject")
	expectBite(t, result, "later-law", pending, "subject gap: Needs a kernel.", "no subject")
	expectBite(t, result, "script-law", passed, "")
	expectBite(t, result, "blind-script", failed, "passed")
}

// Attack C: the guard is deleted and the proof rewritten to expect the
// payment. Nothing in Pay can be broken, so nothing proves the law.
func TestMutationRefusesTheMovedGuardAttack(t *testing.T) {
	source := strings.Replace(spendSource, "if price > balance {\n\t\treturn balance, ErrNotEnoughMoney\n\t}\n", "", 1)
	test := strings.Replace(spendOnePointTest, "err == nil", "err != nil", 1)
	result := mutateSpend(t, source, test, goLaw("spend-law", "TestPay", spendSubject))
	expectBite(t, result, "spend-law", failed, "has no guard condition to mutate")
}

// Attack D: the guard tests the wrong condition and the proof is moved to
// the one input that condition catches. The boundary mutant survives.
func TestMutationRefusesTheWrongConditionAttack(t *testing.T) {
	source := strings.Replace(spendSource, "price > balance", "price < 0", 1)
	test := strings.Replace(spendOnePointTest, "Pay(36, 900)", "Pay(36, -900)", 1)
	result := mutateSpend(t, source, test, goLaw("spend-law", "TestPay", spendSubject))
	expectBite(t, result, "spend-law", failed, "1 of 3 mutants of "+spendSubject+" survived")
	expectBite(t, result, "spend-law", failed, "wallet.go:8 changed < to <= in `price < 0`")
}

// The council's shifted threshold: a proof far from the boundary lets a
// moved boundary through.
func TestMutationRefusesAShiftedThreshold(t *testing.T) {
	source := strings.Replace(spendSource, "price > balance", "price > balance+100", 1)
	result := mutateSpend(t, source, spendTestSource, goLaw("spend-law", "TestPay", spendSubject))
	expectBite(t, result, "spend-law", failed, "")
	honest := mutateSpend(t, spendSource, spendTestSource, goLaw("spend-law", "TestPay", spendSubject))
	expectBite(t, honest, "spend-law", passed, "")
}

const refundSource = `
func Refund(balance, amount int) int {
	return balance + amount
}
`

// Every subject of a law is mutated on its own: the unguarded one fails
// while the guarded one passes.
func TestMutationChecksEachSubject(t *testing.T) {
	doc := goLaw("spend-law", "TestPay", spendSubject, "example.test/shop.Refund")
	result := mutateSpend(t, spendSource+refundSource, spendTestSource, doc)
	expectBiteFor(t, result, "spend-law", "subject "+spendSubject, passed, "killed every mutant")
	expectBiteFor(t, result, "spend-law", "subject example.test/shop.Refund", failed, "has no guard condition")
}

// expectBiteFor checks the one verdict of a law for one subject.
func expectBiteFor(t *testing.T, result moduleResult, law, subject, status, detail string) {
	t.Helper()
	name := "laws-bite (" + law + ": LAWS.md, " + subject + ")"
	for _, item := range result.Verdicts {
		matches := item.Name == name && item.Status == status
		if matches && strings.Contains(item.Detail, detail) {
			return
		}
	}
	t.Fatalf("no %s verdict %q with %q: %+v", status, name, detail, result.Verdicts)
}

func TestMutationNamesASubjectItCannotFind(t *testing.T) {
	doc := goLaw("spend-law", "TestPay", "example.test/shop.Charge") + goLaw("std-law", "TestPay", "strings.Contains")
	test := strings.Replace(spendTestSource, "spend-law:", "spend-law std-law:", 1)
	result := mutateSpend(t, spendSource, test, doc)
	expectBite(t, result, "spend-law", failed, "subject example.test/shop.Charge is not declared")
	expectBite(t, result, "std-law", failed, "subject strings.Contains is not in a package of this repository's build")
}

// A subject may live in another module of the repository that the law's
// module builds against (the example's generated-is-fresh is guarded in
// lawgate's gen package).
func TestMutationReachesASubjectInAnotherModuleOfTheRepository(t *testing.T) {
	isolateMutationCache(t)
	root := t.TempDir()
	writeSpendModule(t, filepath.Join(root, "shop"), spendSource, "package shop\n")
	writeFixture(t, root, "app/go.mod", "module example.test/app\n\ngo 1.25\n\nrequire example.test/shop v0.0.0\n\nreplace example.test/shop => ../shop\n")
	test := strings.Replace(spendTestSource, "package shop\n\nimport \"testing\"", "package app\n\nimport (\"testing\"; . \"example.test/shop\")", 1)
	writeFixture(t, root, "app/app_test.go", test)
	module := inventory.Module{Path: "app", Language: "go", Laws: "LAWS.md", TestCommand: []string{"go", "test", "-json", "."}}
	result := runTestBite(root, module, goLaw("spend-law", "TestPay", spendSubject))
	expectBite(t, result, "spend-law", passed, "killed every mutant")
}

// Each mutant is one go test of the enforcing test with the overlay in
// GOFLAGS and no LAWGATE_BITE; a law whose subject and proof are unchanged
// is answered from the cache without running a mutant.
func TestMutationRunsOneTestPerMutantAndCachesAPass(t *testing.T) {
	isolateMutationCache(t)
	root := t.TempDir()
	writeSpendModule(t, root, spendSource, spendTestSource)
	log := recordGoCalls(t, root)
	module := spendModule("bin/go", "test", "-json", ".")
	doc := goLaw("spend-law", "TestPay", spendSubject)
	result := runTestBite(root, module, doc)
	expectBite(t, result, "spend-law", passed, "")
	calls := readFixture(t, log, "")
	expectCalls(t, calls, 3, "bite=unset", "test -json -count=1 -run ^TestPay$")
	expectCalls(t, calls, 0, "bite=unset", "test", "-tags=lawgate")
	again := runTestBite(root, module, doc)
	expectBite(t, again, "spend-law", passed, "(cached)")
	expectCalls(t, readFixture(t, log, ""), 3, "test -json -count=1 -run ^TestPay$")
}

// The mutant runs keep the configured build: tags, test flags and test
// binary arguments, and leave the configured profile to the normal run.
func TestMutationKeepsTheConfiguredBuild(t *testing.T) {
	isolateMutationCache(t)
	root := t.TempDir()
	test := "//go:build shop\n\n" + strings.Replace(spendTestSource, "import \"testing\"", "import (\"flag\"; \"testing\")\n\nvar strict = flag.Bool(\"strict\", false, \"\")", 1)
	test = strings.Replace(test, "if left, err := Pay(36, 900)", "if !*strict || !testing.Short() {\n\t\tt.Fatal(\"spend-law needs its flags\")\n\t}\n\tif left, err := Pay(36, 900)", 1)
	writeSpendModule(t, root, spendSource, test)
	profile := filepath.Join(t.TempDir(), "cover.out")
	module := spendModule("go", "test", "-json", "-tags", "shop", "-short", "-coverprofile="+profile, ".", "-args", "-strict")
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(goLaw("spend-law", "TestPay", spendSubject))}
	checkTests(root, module, &result)
	normal := readFixture(t, profile, "")
	checkBites(root, module, &result)
	expectBite(t, result, "spend-law", passed, "killed every mutant")
	if after := readFixture(t, profile, ""); after != normal {
		t.Fatalf("the mutant runs rewrote the configured profile")
	}
}

// For laws-bite, a mutant run that never reports the enforcing test is an
// infrastructure failure in words, not a kill and not the raw event stream.
func TestMutationReportsARunWithoutTheTestsResult(t *testing.T) {
	test := spendTestSource + `
func TestMain(m *testing.M) {
	if _, err := Pay(36, 36); err != nil {
		println("mutation broke setup")
		os.Exit(3)
	}
	os.Exit(m.Run())
}
`
	test = strings.Replace(test, "import \"testing\"", "import (\"os\"; \"testing\")", 1)
	result := mutateSpend(t, spendSource, test, goLaw("spend-law", "TestPay", spendSubject))
	expectBite(t, result, "spend-law", infrastructure, "reported no result against the mutant")
	expectBite(t, result, "spend-law", infrastructure, "mutation broke setup")
	expectNoEventStream(t, result)
}

// lawgate check runs with --root . by default: a relative root must still
// find the subject inside the repository.
func TestMutationAcceptsARelativeRoot(t *testing.T) {
	isolateMutationCache(t)
	root := t.TempDir()
	writeSpendModule(t, root, spendSource, spendTestSource)
	t.Chdir(root)
	result := runTestBite(".", spendModule(), goLaw("spend-law", "TestPay", spendSubject))
	expectBite(t, result, "spend-law", passed, "killed every mutant")
}

const propagatingSource = `
func checked(price int) error { return nil }

func Load(read func() error) error {
	if err := read(); err != nil {
		return err
	}
	return nil
}
`

// A guard that only passes an error up is not mutated, and the verdict says
// how many such guards it left, so the limit stays visible.
func TestMutationCountsErrorPropagationGuards(t *testing.T) {
	propagating := "if err := checked(price); err != nil {\n\t\treturn 0, err\n\t}\n\tif price > balance {"
	source := strings.Replace(spendSource, "if price > balance {", propagating, 1) + propagatingSource
	doc := goLaw("spend-law", "TestPay", spendSubject, "example.test/shop.Load")
	result := mutateSpend(t, source, spendTestSource, doc)
	expectBiteFor(t, result, "spend-law", "subject "+spendSubject, passed, "(3); 1 error-propagation guard not mutated")
	expectBiteFor(t, result, "spend-law", "subject example.test/shop.Load", failed, "has no guard condition to mutate (1 error-propagation guard not mutated)")
}
