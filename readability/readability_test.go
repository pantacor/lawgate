package readability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCoreFollowsTheReadabilityRule enforces readable-laws-code over the whole
// lawgate repository.
func TestCoreFollowsTheReadabilityRule(t *testing.T) {
	// The checker must still see a known violation, or a repository with
	// zero findings proves nothing (laws-bite).
	if len(mustCheck(t, "nesting_fail.go", nestingFailSrc())) == 0 {
		t.Fatal("the readability checker reports nothing on a known violation")
	}
	findings, err := CheckDir("..", nil)
	if err != nil {
		t.Fatalf("CheckDir failed: %v", err)
	}
	for _, f := range findings {
		t.Errorf("self-check violation [%s] %s:%d (%s): %s", f.Rule, f.Path, f.Line, f.Func, f.Message)
	}
}

func TestNestingRule(t *testing.T) {
	passFindings := mustCheck(t, "nesting_pass.go", nestingPassSrc())
	if len(passFindings) != 0 {
		t.Fatalf("expected 0 findings for passing nesting, got %v", passFindings)
	}

	failFindings := mustCheck(t, "nesting_fail.go", nestingFailSrc())
	if len(failFindings) != 3 {
		t.Fatalf("expected 3 nesting findings, got %d: %v", len(failFindings), failFindings)
	}
	assertAllRule(t, failFindings, RuleNesting)
	assertContains(t, failFindings[0].Message, "nested 3 levels deep")
	assertContains(t, failFindings[1].Message, "<func literal>")
	assertContains(t, failFindings[2].Message, "secondGuard")
}

func nestingPassSrc() string {
	return `package sample

type flag bool

func okDepth(xs []int, a, b flag) int {
	if a {
		for _, x := range xs {
			if x == 0 {
				continue
			}
			_ = x
		}
	} else if b {
		for i := 0; i < len(xs); i++ {
			if xs[i] < 0 {
				break
			}
			_ = i
		}
	} else {
		fn := func(ok flag) int {
			if ok {
				return 1
			}
			return 0
		}
		return fn(b)
	}
	return 0
}
`
}

func nestingFailSrc() string {
	return `package sample

type flag bool
func tooDeep(a, b, c flag) int {
	if a {
		if b {
			if c {
				return 1
			}
		}
	}
	return 0
}

func deepLit(a, b, c flag) {
	fn := func() {
		if a {
			for i := 0; i < 1; i++ {
				if c {
					_ = i
				}
			}
		}
	}
	fn()
}

func secondGuard(xs []int, ok flag) {
	if ok {
		for _, x := range xs {
			y := x + 1
			if y == 0 {
				continue
			}
		}
	}
}
`
}

func TestInlineNestingRule(t *testing.T) {
	passFindings := mustCheck(t, "inline_pass.go", inlinePassSrc())
	if len(passFindings) != 0 {
		t.Fatalf("expected 0 findings for passing inline literals, got %v", passFindings)
	}

	failFindings := mustCheck(t, "inline_fail.go", inlineFailSrc())
	if len(failFindings) != 4 {
		t.Fatalf("expected 4 inline-nesting findings, got %d: %v", len(failFindings), failFindings)
	}
	assertAllRule(t, failFindings, RuleInlineNesting)
	assertContains(t, failFindings[0].Message, "nested inside composite literal")
}

func inlinePassSrc() string {
	return `package sample

type Site struct{ Tag string }
type Point struct{ X int }
type Handler struct{ Fn func() }
type StringLists map[string][]string

func okLiterals() {
	_ = []Site{{Tag: "x"}}
	_ = []string{"a"}
	_ = Point{X: 1}
	_ = map[string][]string{"a": {"x", "y"}}
	_ = StringLists{"a": {"x"}}
	_ = Handler{Fn: func() { _ = Point{X: 2} }}
}
`
}

func inlineFailSrc() string {
	return `package sample

type State struct{ ID string }
type Machine struct{ States []State }
type Config struct{ Port int }
type Inner struct{ X int }
type Outer struct{ Inner *Inner }

func badLiterals() {
	_ = Machine{States: []State{{ID: "a"}}}
	_ = map[string]Config{"x": {Port: 1}}
	_ = Outer{Inner: &Inner{X: 1}}
	_ = map[string]map[string]int{"a": {"b": 1}}
}
`
}

func TestFuncLengthRule(t *testing.T) {
	passFindings := mustCheck(t, "len_pass.go", buildFuncWithBodyLines("exact40", "", 40))
	if len(passFindings) != 0 {
		t.Fatalf("expected 0 findings for 40-line body, got %v", passFindings)
	}

	failSrc := buildFuncWithBodyLines("over41", "", 41) + "\n" + buildMethodWithBodyLines("svc", "run", 42)
	failFindings := mustCheck(t, "len_fail.go", failSrc)
	if len(failFindings) != 2 {
		t.Fatalf("expected 2 func-length findings, got %d: %v", len(failFindings), failFindings)
	}
	assertAllRule(t, failFindings, RuleFuncLength)
	if failFindings[0].Func != "over41" {
		t.Errorf("expected func over41, got %q", failFindings[0].Func)
	}
	if failFindings[1].Func != "(*svc).run" {
		t.Errorf("expected method (*svc).run, got %q", failFindings[1].Func)
	}
}

func buildFuncWithBodyLines(name, extra string, totalLines int) string {
	var sb strings.Builder
	sb.WriteString("package sample\n\n")
	if extra != "" {
		sb.WriteString(extra)
	}
	fmt.Fprintf(&sb, "func %s() {\n", name)
	for i := 0; i < totalLines-2; i++ {
		sb.WriteString("\t_ = 1\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

func buildMethodWithBodyLines(recv, name string, totalLines int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "type %s struct{}\n", recv)
	fmt.Fprintf(&sb, "func (*%s) %s() {\n", recv, name)
	for i := 0; i < totalLines-2; i++ {
		sb.WriteString("\t_ = 1\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

func TestConditionRule(t *testing.T) {
	passFindings := mustCheck(t, "cond_pass.go", conditionPassSrc())
	if len(passFindings) != 0 {
		t.Fatalf("expected 0 findings for passing conditions, got %v", passFindings)
	}

	failFindings := mustCheck(t, "cond_fail.go", conditionFailSrc())
	if len(failFindings) != 4 {
		t.Fatalf("expected 4 condition findings, got %d: %v", len(failFindings), failFindings)
	}
	assertAllRule(t, failFindings, RuleCondition)
}

func TestTaglessSwitchConditions(t *testing.T) {
	src := `package sample

type flag bool

func cases(a, b, c flag) {
	both := a && b
	switch {
	case both || c:
	case a || (b && c), a && !(b || c):
	default:
	}
	switch a {
	case a || (b && c):
	}
}
`
	findings := mustCheck(t, "switch.go", src)
	if len(findings) != 2 {
		t.Fatalf("expected 2 tagless case findings, got %v", findings)
	}
	assertAllRule(t, findings, RuleCondition)
	for _, finding := range findings {
		assertContains(t, finding.Message, "case condition has 3 boolean operands")
	}
}

func conditionPassSrc() string {
	return `package sample

type flag bool

func helper(a, b flag) flag { return a && b }

func okConds(a, b, c flag) flag {
	both := a && b
	if both && c {
		return true
	}
	if helper(a, b) || c {
		return true
	}
	for a && b {
		break
	}
	return a || b
}
`
}

func conditionFailSrc() string {
	return `package sample

type flag bool

func badConds(a, b, c flag) flag {
	if a && b && c {
		return true
	}
	if !(a || b) && c {
		return false
	}
	for a && b || c {
		break
	}
	return a || b || c
}
`
}

func TestCheckFileFromDisk(t *testing.T) {
	root := t.TempDir()
	src := "package p\ntype flag bool\nfunc f(a, b, c flag) bool { return a && b && c }\n"
	writeTestFile(t, root, "disk.go", src)

	findings, err := CheckFile(filepath.Join(root, "disk.go"), nil)
	if err != nil {
		t.Fatalf("CheckFile(nil) failed: %v", err)
	}
	if len(findings) != 1 || findings[0].Rule != RuleCondition {
		t.Fatalf("expected 1 condition finding, got %v", findings)
	}
}

func TestCheckDirSkipsAndIncludes(t *testing.T) {
	root := t.TempDir()
	badSrc := "package p\ntype flag bool\nfunc f(a, b, c flag) bool { return a && b && c }\n"
	okSrc := "package p\nfunc g() {}\n"

	writeTestFile(t, root, "ok.go", okSrc)
	writeTestFile(t, root, "bad_test.go", badSrc)
	writeTestFile(t, root, "vendor/v.go", badSrc)
	writeTestFile(t, root, "testdata/t.go", badSrc)
	writeTestFile(t, root, ".hidden/h.go", badSrc)
	writeTestFile(t, root, "submod/go.mod", "module example.com/sub\n\ngo 1.25\n")
	writeTestFile(t, root, "submod/s.go", badSrc)
	writeTestFile(t, root, "sub/included.go", badSrc)

	all, err := CheckDir(root, nil)
	if err != nil {
		t.Fatalf("CheckDir failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 findings (bad_test.go and sub/included.go), got %d: %v", len(all), all)
	}

	onlySub, err := CheckDir(root, func(p string) bool {
		return strings.HasPrefix(p, "sub/")
	})
	if err != nil {
		t.Fatalf("CheckDir with include failed: %v", err)
	}
	if len(onlySub) != 1 || onlySub[0].Path != "sub/included.go" {
		t.Fatalf("expected only sub/included.go, got %v", onlySub)
	}
}

func TestBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.txt")
	want := Baseline{
		"z/last.go":  3,
		"a/first.go": 1,
		"zero.go":    0,
	}
	if err := WriteBaseline(path, want); err != nil {
		t.Fatalf("WriteBaseline failed: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	expectedText := "1 a/first.go\n3 z/last.go\n"
	if string(raw) != expectedText {
		t.Fatalf("baseline file mismatch:\ngot:\n%s\nwant:\n%s", string(raw), expectedText)
	}

	got, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 baseline entries, got %v", got)
	}
	if got["a/first.go"] != 1 || got["z/last.go"] != 3 {
		t.Fatalf("loaded baseline mismatch: %v", got)
	}
}

func TestRatchet(t *testing.T) {
	base := Baseline{
		"same.go":   1,
		"grew.go":   1,
		"shrank.go": 3,
		"clean.go":  2,
	}
	current := []Finding{
		{Path: "same.go", Rule: RuleCondition},
		{Path: "grew.go", Rule: RuleCondition},
		{Path: "grew.go", Rule: RuleNesting},
		{Path: "shrank.go", Rule: RuleCondition},
		{Path: "new.go", Rule: RuleFuncLength},
	}

	problems := Ratchet(current, base)
	if len(problems) != 2 {
		t.Fatalf("expected 2 ratchet problems, got %d: %v", len(problems), problems)
	}
	assertContains(t, problems[0], "grew.go")
	assertContains(t, problems[1], "new.go")

	notes := Shrinkable(current, base)
	if len(notes) != 2 {
		t.Fatalf("expected 2 shrink notes, got %d: %v", len(notes), notes)
	}
	assertContains(t, notes[0], "clean.go: baseline can shrink to 0")
	assertContains(t, notes[1], "shrank.go: baseline can shrink to 1")
}

func mustCheck(t *testing.T, path, src string) []Finding {
	t.Helper()
	findings, err := CheckFile(path, []byte(src))
	if err != nil {
		t.Fatalf("CheckFile(%s) failed: %v", path, err)
	}
	return findings
}

func assertAllRule(t *testing.T, findings []Finding, rule string) {
	t.Helper()
	for _, f := range findings {
		if f.Rule != rule {
			t.Errorf("expected rule %q, got %q (%s)", rule, f.Rule, f.Message)
		}
	}
}

func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("expected %q to contain %q", s, sub)
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func TestBoolParameterRule(t *testing.T) {
	bad := `package example
func named(flag bool) {}
func (receiver Thing) method(bool) {}
var closure = func(flag bool) {}
`
	clean := `package example
func okay(flag *bool, value string) bool { return true }
`
	findings, err := CheckFile("bad.go", []byte(bad))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 {
		t.Fatalf("want three bool parameters, got %v", findings)
	}
	assertAllRule(t, findings, "bool-param")
	findings, err = CheckFile("clean.go", []byte(clean))
	if err != nil || len(findings) != 0 {
		t.Fatalf("unexpected findings %v or error %v", findings, err)
	}
}

func TestFuncLitLengthRule(t *testing.T) {
	passSrc := buildTopLevelFuncLitWithBodyLines(40)
	if findings := mustCheck(t, "lit_len_pass.go", passSrc); len(findings) != 0 {
		t.Fatalf("expected 0 findings for 40-line func literal, got %v", findings)
	}

	failTopLevel := buildTopLevelFuncLitWithBodyLines(41)
	topFindings := mustCheck(t, "lit_top_fail.go", failTopLevel)
	if len(topFindings) != 1 {
		t.Fatalf("expected 1 finding for top-level 41-line func literal, got %d: %v", len(topFindings), topFindings)
	}
	assertAllRule(t, topFindings, RuleFuncLength)
	if topFindings[0].Func != "<func literal>" {
		t.Errorf("expected func <func literal>, got %q", topFindings[0].Func)
	}

	failInFunc := buildFuncLitWithBodyLines("holder", 41)
	inFuncFindings := mustCheck(t, "lit_len_fail.go", failInFunc)
	if len(inFuncFindings) != 2 {
		t.Fatalf("expected 2 findings (holder and holder.<func literal>), got %d: %v", len(inFuncFindings), inFuncFindings)
	}
	assertAllRule(t, inFuncFindings, RuleFuncLength)
	if inFuncFindings[1].Func != "holder.<func literal>" {
		t.Errorf("expected func holder.<func literal>, got %q", inFuncFindings[1].Func)
	}
}

func buildFuncLitWithBodyLines(parent string, totalLines int) string {
	var sb strings.Builder
	sb.WriteString("package sample\n\n")
	fmt.Fprintf(&sb, "func %s() {\n", parent)
	sb.WriteString("\t_ = func() {\n")
	for i := 0; i < totalLines-2; i++ {
		sb.WriteString("\t\t_ = 1\n")
	}
	sb.WriteString("\t}\n")
	sb.WriteString("}\n")
	return sb.String()
}

func buildTopLevelFuncLitWithBodyLines(totalLines int) string {
	var sb strings.Builder
	sb.WriteString("package sample\n\n")
	sb.WriteString("var top = func() {\n")
	for i := 0; i < totalLines-2; i++ {
		sb.WriteString("\t_ = 1\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

func TestInlineNestedSliceRule(t *testing.T) {
	findings := mustCheck(t, "nested_slices_fail.go", nestedSlicesFailSrc())
	if len(findings) != 7 {
		t.Fatalf("expected 7 inline-nesting findings for nested slices/arrays, got %d: %v", len(findings), findings)
	}
	assertAllRule(t, findings, RuleInlineNesting)

	if passFindings := mustCheck(t, "flat_slices_pass.go", flatSlicesPassSrc()); len(passFindings) != 0 {
		t.Fatalf("expected 0 findings for flat slices, got %v", passFindings)
	}
}

func nestedSlicesFailSrc() string {
	return `package sample

type Grid [][]int
type Row []int

func nestedSlices() {
	_ = [][]int{{1}}
	_ = [2][2]int{{1, 2}, {3, 4}}
	_ = Grid{{1}}
	_ = []Row{{1}}
	_ = []any{[]int{1}}
	_ = map[string][][]int{"a": {{1}}}
}
`
}

func flatSlicesPassSrc() string {
	return `package sample

type Box struct{ Tags []string }

func flatSlices() {
	_ = Box{Tags: []string{"a", "b"}}
	_ = []Box{{Tags: []string{"a"}}}
}
`
}

func TestClosureNestingContinuesEnclosingDepth(t *testing.T) {
	findings := mustCheck(t, "closure_depth_fail.go", closureDepthFailSrc())
	if len(findings) != 1 {
		t.Fatalf("expected 1 nesting finding inside closure, got %d: %v", len(findings), findings)
	}
	assertAllRule(t, findings, RuleNesting)
	assertContains(t, findings[0].Message, "nested 3 levels deep")
	assertContains(t, findings[0].Func, "closureInLoop.<func literal>")

	if passFindings := mustCheck(t, "closure_depth_pass.go", closureDepthPassSrc()); len(passFindings) != 0 {
		t.Fatalf("expected 0 findings when enclosing+inner depth <= 2, got %v", passFindings)
	}
}

func closureDepthFailSrc() string {
	return `package sample

type flag bool

func closureInLoop(a, b flag) {
	for i := 0; i < 1; i++ {
		if a {
			func() {
				if b {
					_ = i
				}
			}()
		}
	}
}
`
}

func closureDepthPassSrc() string {
	return `package sample

type flag bool

func closureAtDepthOne(a, b flag) {
	if a {
		fn := func() {
			if b {
				_ = 1
			}
		}
		fn()
	}
}
`
}
