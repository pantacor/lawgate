package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
)

// recordGoCalls installs bin/go, which logs each call with its selector.
func recordGoCalls(t *testing.T, root string) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"bite=${LAWGATE_BITE-unset} $*\" >> %q\nexec go \"$@\"\n", log)
	writeExecutable(t, root, "bin/go", script)
	return log
}

func expectCalls(t *testing.T, calls string, want int, fragments ...string) {
	t.Helper()
	got := 0
	for line := range strings.SplitSeq(calls, "\n") {
		matches := !slices.ContainsFunc(fragments, func(fragment string) bool {
			return !strings.Contains(line, fragment)
		})
		if matches {
			got++
		}
	}
	if got != want {
		t.Fatalf("%d calls with %q, want %d:\n%s", got, fragments, want, calls)
	}
}

func expectNoEventStream(t *testing.T, result moduleResult) {
	t.Helper()
	for _, item := range result.Verdicts {
		if strings.Contains(item.Detail, `"Action"`) {
			t.Fatalf("detail quotes the go test -json stream: %+v", item)
		}
	}
}

func TestBiteExitCodeSeparatesStartFailuresAndSignals(t *testing.T) {
	cases := map[string]int{"exit 0": 0, "exit 1": 1, "exit 126": 126, "exit 127": 127, "kill -TERM $$": -1}
	for script, want := range cases {
		err := exec.Command("sh", "-c", script).Run()
		if got := biteExitCode(err); got != want {
			t.Errorf("%s: exit code %d, want %d", script, got, want)
		}
	}
	if got := biteExitCode(exec.Command(filepath.Join(t.TempDir(), "absent")).Run()); got != -1 {
		t.Errorf("a command that cannot start: exit code %d, want -1", got)
	}
}

func TestNodeBiteSelectsOnlyDirectNodeRunners(t *testing.T) {
	direct := []string{"/usr/bin/node", "--test", "--test-reporter=tap"}
	want := []string{"/usr/bin/node", `--test-name-pattern=first \(law\)`, "--test", "--test-reporter=tap"}
	if got := selectNodeTest(direct, "first (law)"); !slices.Equal(got, want) {
		t.Fatalf("selected %q, want %q", got, want)
	}
	for _, runner := range []string{"npm test", "sh test.sh", "node run-tests.js"} {
		command := strings.Fields(runner)
		if got := selectNodeTest(command, "first"); !slices.Equal(got, command) {
			t.Fatalf("%q must rerun unchanged, got %q", command, got)
		}
	}
}

// The reruns of a law must not repeat the rest of the suite.
func TestNodeBiteRerunsOnlyTheNamedTest(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required")
	}
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "other.log")
	writeFixture(t, root, "law.test.js", fmt.Sprintf(nodeSuiteSource, log))
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md"}
	doc := biteLaw("top-law", "test", "top level law") + biteLaw("nested-law", "test", "nested law")
	result := runTestBite(root, module, doc)
	expectBite(t, result, "top-law", passed, "")
	expectBite(t, result, "nested-law", passed, "")
	if runs := strings.Count(readFixture(t, log, ""), "ran"); runs != 1 {
		t.Fatalf("the unrelated test ran %d times, want once (the normal run): %+v", runs, result.Verdicts)
	}
}

const nodeSuiteSource = `const { test, describe, it } = require('node:test');
const fs = require('node:fs');
const selected = process.env.LAWGATE_BITE;
test('top level law', () => { if (selected === 'top-law') throw new Error('top-law'); });
describe('suite', () => {
	it('nested law', () => { if (selected === 'nested-law') throw new Error('nested-law'); });
});
test('unrelated', () => { fs.appendFileSync(%q, 'ran\n'); });
`

// Node skips a parent test whose name does not match the pattern, with every
// test() inside it: the rerun must still reach the nested enforcer.
func TestNodeBiteReachesATestNestedInAnotherTest(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required")
	}
	root := t.TempDir()
	writeFixture(t, root, "law.test.js", nodeNestedSource)
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md"}
	doc := biteLaw("sub-law", "test", "rejects empty payload") + biteLaw("blind-law", "test", "accepts a payload")
	result := runTestBite(root, module, doc)
	expectBite(t, result, "sub-law", passed, "")
	expectBite(t, result, "blind-law", failed, "test accepts a payload passed with LAWGATE_BITE=blind-law")
}

const nodeNestedSource = `const { test } = require('node:test');
const assert = require('node:assert');
const accepts = (payload) => process.env.LAWGATE_BITE === 'sub-law' || payload !== '';
test('payloads', async () => {
	await test('rejects empty payload', () => { assert.equal(accepts(''), false); });
	await test('accepts a payload', () => { assert.equal(accepts('x'), true); });
});
`

// A control that exits 0 without the named test has a cause: it never ran.
func TestNodeBiteSaysWhenTheNamedTestDidNotRun(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required")
	}
	root := t.TempDir()
	writeFixture(t, root, "law.test.js", nodeVanishingSource)
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md"}
	result := runTestBite(root, module, biteLaw("gone-law", "test", "vanishing law"))
	want := "test vanishing law did not run in the rerun, with --test-name-pattern or with the whole test command"
	expectBite(t, result, "gone-law", failed, want)
}

const nodeVanishingSource = `const { test } = require('node:test');
test('always', () => {});
if (!process.env.LAWGATE_BITE) test('vanishing law', () => {});
`

// A failing Node control reports only the named test's TAP block, not the
// whole TAP stream (subtests, siblings, headers and footer).
func TestNodeBiteControlReportsOnlyNamedTAPBlock(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required")
	}
	root := t.TempDir()
	writeFixture(t, root, "law.test.js", nodeBrokenControlSource)
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md"}
	result := runTestBite(root, module, biteLaw("control-law", "test", "broken control"))
	expectBite(t, result, "control-law", failed, "not ok ")
	expectBite(t, result, "control-law", failed, "broken control")
	expectBite(t, result, "control-law", failed, "control broke here")
	expectBite(t, result, "control-law", failed, "location:")
	expectNotInBiteDetail(t, result, "TAP version 13", "# Subtest:", "sibling pass", "unrelated", "# tests ")
}

const nodeBrokenControlSource = `const { test } = require('node:test');
test('unrelated', () => {});
test('suite', async () => {
	await test('broken control', () => {
		if (process.env.LAWGATE_BITE) throw new Error('control broke here');
	});
	await test('sibling pass', () => {});
});
`

// A failing Node mutation also quotes only the named test's TAP block when
// the test printed a failure before the stream ended without a plan.
func TestNodeBiteMutationFailureReportsOnlyNamedTAPBlock(t *testing.T) {
	root := t.TempDir()
	script := `if [ "$LAWGATE_BITE" = mut-law ]; then
printf '%s
' 'TAP version 13' '# Subtest: unrelated' 'ok 1 - unrelated' 'not ok 2 - target law' '  ---' "  error: 'mutation rejected'" '  ...'
exit 1
fi
printf '%s
' 'TAP version 13' 'ok 1 - unrelated' 'ok 2 - target law' '1..2'
`
	command := []string{"sh", "-c", script}
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md", TestCommand: command}
	result := runTestBite(root, module, biteLaw("mut-law", "test", "target law"))
	expectBite(t, result, "mut-law", failed, "not ok 2 - target law")
	expectBite(t, result, "mut-law", failed, "error: 'mutation rejected'")
	expectBite(t, result, "mut-law", failed, "incomplete Node TAP report")
	expectNotInBiteDetail(t, result, "TAP version 13", "# Subtest:", "ok 1 - unrelated")
}

func expectNotInBiteDetail(t *testing.T, result moduleResult, forbidden ...string) {
	t.Helper()
	for _, item := range result.Verdicts {
		if !strings.Contains(item.Name, biteLawName) {
			continue
		}
		assertNoFragments(t, item, forbidden)
	}
}

func assertNoFragments(t *testing.T, item verdict, forbidden []string) {
	t.Helper()
	for _, fragment := range forbidden {
		if strings.Contains(item.Detail, fragment) {
			t.Fatalf("bite detail must not contain %q: %+v", fragment, item)
		}
	}
}
