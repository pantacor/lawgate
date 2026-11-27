package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pantacor/lawgate/bite"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/mutate"
)

const (
	biteLawName   = "laws-bite"
	biteGapPrefix = "bite gap: "
	testDiscovery = "test discovery"
)

type bitePhase struct {
	root    string
	module  inventory.Module
	result  *moduleResult
	goTests func() (*goBiteInputs, error)
	cache   mutate.Cache
	counter *biteCounter
}

// mutationCache is where passing mutation runs are remembered.
var mutationCache = mutate.UserCache

func checkBites(root string, module inventory.Module, result *moduleResult) {
	if !result.Ready {
		return
	}
	primary := lawDocument{Path: module.Laws, Plans: result.Plans}
	docs := append([]lawDocument{primary}, result.SharedDocs...)
	counter := &biteCounter{total: agreedLaws(docs)}
	phase := bitePhase{root: root, module: module, result: result, cache: mutationCache(), counter: counter}
	phase.goTests = syncGoBiteInputs(root, module, result)
	for _, doc := range docs {
		phase.checkDocument(doc)
	}
}

// biteCounter numbers the laws a module's bite phase checks, for progress.
type biteCounter struct {
	done, total int
}

// next reports the start of the next law's bite and returns its end.
func (counter *biteCounter) next(law string) func() {
	if counter == nil {
		return func() {}
	}
	counter.done++
	return progress.timed("bite %d/%d: %s", counter.done, counter.total, law)
}

func agreedLaws(docs []lawDocument) int {
	count := 0
	for _, doc := range docs {
		count += agreedPlans(doc.Plans)
	}
	return count
}

func agreedPlans(plans []lawsdoc.LawPlan) int {
	count := 0
	for _, law := range plans {
		if law.Agreed() {
			count++
		}
	}
	return count
}

// syncGoBiteInputs discovers tests once, only if an enforcer needs them.
func syncGoBiteInputs(root string, module inventory.Module, result *moduleResult) func() (*goBiteInputs, error) {
	return sync.OnceValues(func() (*goBiteInputs, error) {
		if hasDiscoveryFailure(result.Verdicts) {
			return nil, nil
		}
		dir := filepath.Join(root, module.Path)
		return discoverGoBiteInputs(dir, module.TestArgs(), module.Timeout())
	})
}

func hasDiscoveryFailure(items []verdict) bool {
	return slices.ContainsFunc(items, func(item verdict) bool {
		return item.Name == testDiscovery && item.Status == infrastructure
	})
}

type goBiteInputs struct {
	build inventory.GoCommand
	tests lawsdoc.Tests
}

func discoverGoBiteInputs(dir string, command []string, timeout time.Duration) (*goBiteInputs, error) {
	build, err := inventory.ParseGoCommand(dir, command)
	if err != nil {
		return nil, err
	}
	if build.Subcommand != "test" {
		return nil, fmt.Errorf("Go mutation requires a direct go test command; use env NAME=value go test for Go build settings")
	}
	tests, err := lawsdoc.DiscoverCommandTests(dir, command, timeout)
	if err != nil {
		return nil, err
	}
	return &goBiteInputs{build: build, tests: tests}, nil
}

func (phase bitePhase) checkDocument(doc lawDocument) {
	for _, law := range doc.Plans {
		if law.Agreed() {
			done := phase.counter.next(law.Name)
			phase.checkLaw(doc.Path, law)
			done()
		}
	}
}

// checkLaw mutates the subject of a Go-test law and reruns every other test
// or script enforcer with LAWGATE_BITE.
func (phase bitePhase) checkLaw(doc string, law lawsdoc.LawPlan) {
	goModule := phase.module.Language == "go"
	for _, enforcer := range law.Enforcers {
		mutated := goModule && enforcer.Kind == lawsdoc.Test
		if !enforcer.CanBite() || mutated {
			continue
		}
		phase.checkEnforcer(doc, law, enforcer)
	}
	if goModule && slices.ContainsFunc(law.Enforcers, isTestEnforcer) {
		phase.checkGoLaw(doc, law)
	}
}

func isTestEnforcer(enforcer lawsdoc.Enforcer) bool {
	return enforcer.Kind == lawsdoc.Test
}

type biteCase struct {
	doc      string
	law      string
	enforcer string
}

func (item biteCase) name() string {
	return fmt.Sprintf("%s (%s: %s, %s)", biteLawName, item.law, item.doc, item.enforcer)
}

func (phase bitePhase) checkEnforcer(doc string, law lawsdoc.LawPlan, enforcer lawsdoc.Enforcer) {
	item := biteCase{doc: doc, law: law.Name, enforcer: enforcer.Name}
	if law.BiteGap != "" {
		phase.add(verdict{Name: item.name(), Status: pending, Detail: biteGapPrefix + law.BiteGap})
		return
	}
	switch enforcer.Kind {
	case lawsdoc.Test:
		phase.checkTest(item)
	case lawsdoc.Script:
		phase.checkScript(item)
	}
}

func (phase bitePhase) checkScript(item biteCase) {
	baseline := scriptVerdictName(item.law, item.enforcer)
	if !phase.passedNormally(baseline) {
		phase.add(verdict{Name: item.name(), Status: failed, Detail: "script must pass before checking its bite"})
		return
	}
	path, err := lawsdoc.ScriptPath(phase.root, item.doc, item.enforcer)
	if err == nil {
		err = refuseScriptLink(phase.root, item.doc, item.enforcer, path)
	}
	if err != nil {
		phase.result.recordError(item.name(), err)
		return
	}
	command := []string{path}
	stdout, stderr, err := phase.executeBite(phase.root, command, unrelatedControl)
	if err != nil {
		phase.result.recordError(item.name(), fmt.Errorf("script control must pass: %w", commandError(err, stdout, stderr)))
		return
	}
	phase.mutateScript(item, command)
}

func (phase bitePhase) mutateScript(item biteCase, command []string) {
	stdout, stderr, err := phase.executeBite(phase.root, command, item.law)
	code := biteExitCode(err)
	marker := "lawgate: rejects " + item.law
	rejected := hasScriptMarker(string(stdout), marker) || hasScriptMarker(stderr, marker)
	switch {
	case err == nil:
		phase.add(verdict{Name: item.name(), Status: failed, Detail: "script passed with " + bite.Env + "=" + item.law})
	case code == 1 && rejected:
		phase.add(verdict{Name: item.name(), Status: passed})
	case code == 1:
		cause := commandError(err, stdout, stderr)
		phase.result.recordError(item.name(), fmt.Errorf("script exited without the required line %q: %w", marker, cause))
	default:
		phase.result.recordError(item.name(), commandError(err, stdout, stderr))
	}
}

func hasScriptMarker(output, marker string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		if strings.TrimSuffix(line, "\r") == marker {
			return true
		}
	}
	return false
}

func (phase bitePhase) passedNormally(name string) bool {
	return slices.ContainsFunc(phase.result.Verdicts, func(item verdict) bool {
		return item.Name == name && item.Status == passed
	})
}

func (phase bitePhase) add(item verdict) {
	phase.result.Verdicts = append(phase.result.Verdicts, item)
}

func (phase bitePhase) checkTest(item biteCase) {
	dir := filepath.Join(phase.root, phase.module.Path)
	whole := phase.module.TestArgs()
	target := biteTestTarget{
		name:     item.enforcer,
		dir:      dir,
		command:  selectNodeTest(whole, item.enforcer),
		selector: item.law,
		whole:    whole,
	}
	if err := phase.result.TestRuns.require(target.name); err != nil {
		phase.add(verdict{Name: item.name(), Status: failed, Detail: err.Error()})
		return
	}
	if !phase.passedNormally(target.name) {
		phase.add(verdict{Name: item.name(), Status: failed, Detail: "test must pass before checking its bite"})
		return
	}
	if checked, ok := phase.passesControl(item, target); ok {
		phase.runTest(item, checked)
	}
}

// selectNodeTest narrows a direct `node --test` command to the tests whose
// name contains the enforcer's, so a rerun does not repeat the whole suite.
// Any other runner takes no selection and reruns everything it runs.
//
// Node matches the pattern against each test's own name and skips a parent
// that does not match together with its subtests, so the selection can drop a
// test() nested in another test(): rerun falls back to the whole command.
func selectNodeTest(command []string, name string) []string {
	direct := len(command) > 1 && filepath.Base(command[0]) == "node" && slices.Contains(command[1:], "--test")
	if !direct {
		return command
	}
	pattern := "--test-name-pattern=" + regexp.QuoteMeta(name)
	return slices.Concat(command[:1], []string{pattern}, command[1:])
}

type biteTestTarget struct {
	name     string
	dir      string
	command  []string
	selector string
	// whole is the command without a test selection; it is set only where
	// command may narrow it.
	whole []string
}

// The unrelated selection catches an enforcer that rejects whenever
// LAWGATE_BITE is nonempty.
const unrelatedControl = "lawgate/control/no-such-law"

func (phase bitePhase) executeBite(dir string, command []string, selector string) ([]byte, string, error) {
	overrides := []string{bite.Env + "=" + selector}
	return inventory.RunGateCommandWithEnv(dir, command, overrides, phase.module.Timeout())
}

// biteRun is one rerun of an enforcing test and what it reported.
type biteRun struct {
	// widened marks a rerun whose selection skipped the named test and that
	// ran the whole command instead.
	widened  bool
	err      error
	code     int
	stdout   []byte
	stderr   string
	evidence testEvidenceLog
	parseErr error
}

func (phase bitePhase) rerun(target biteTestTarget, selector string) biteRun {
	run := phase.runCommand(target.dir, target.command, selector)
	if target.narrowed() && !run.ran(target.name) {
		run = phase.runCommand(target.dir, target.whole, selector)
		run.widened = true
	}
	return run
}

func (phase bitePhase) runCommand(dir string, command []string, selector string) biteRun {
	stdout, stderr, err := phase.executeBite(dir, command, selector)
	run := biteRun{err: err, code: biteExitCode(err), stdout: stdout, stderr: stderr}
	run.evidence, run.parseErr = readTestEvidence(phase.module.Language, stdout)
	return run
}

func (target biteTestTarget) narrowed() bool {
	return len(target.whole) != 0 && !slices.Equal(target.command, target.whole)
}

// ran reports whether the rerun has any result for the named test.
func (run biteRun) ran(name string) bool {
	return run.evidence.tests.ran[name]
}

// absence names a rerun that never reached the test: without it, a control
// that exits 0 would be reported as failing with no cause at all.
func (run biteRun) absence(name string) error {
	if run.ran(name) {
		return nil
	}
	if run.widened {
		return fmt.Errorf("test %s did not run in the rerun, with --test-name-pattern or with the whole test command", name)
	}
	return fmt.Errorf("test %s did not run in the rerun", name)
}

func (run biteRun) reports(name, status string) bool {
	return run.parseErr == nil && hasBiteTestVerdict(run.evidence, name, status)
}

func (run biteRun) passedTest(name string) bool {
	return run.err == nil && run.reports(name, passed)
}

func (phase bitePhase) runTest(item biteCase, target biteTestTarget) {
	run := phase.rerun(target, target.selector)
	startFailed := run.code == 126 || run.code == 127
	abnormal := run.code < 0 || startFailed
	rejected := run.code > 0 && !startFailed && run.reports(target.name, failed)
	switch {
	case rejected:
		phase.add(verdict{Name: item.name(), Status: passed})
	case abnormal:
		phase.result.recordError(item.name(), commandError(run.err, run.stdout, run.stderr))
	default:
		phase.add(verdict{Name: item.name(), Status: failed, Detail: phase.survivedMutation(run, target)})
	}
}

func (phase bitePhase) survivedMutation(run biteRun, target biteTestTarget) string {
	outcome := "did not fail"
	if run.code == 0 && run.reports(target.name, passed) {
		outcome = "passed"
	}
	detail := fmt.Sprintf("test %s %s with %s=%s", target.name, outcome, bite.Env, target.selector)
	return withCause(detail, phase.cause(run, target.name))
}

// passesControl reruns the test with a selector no enforcer answers to.
// It returns the target the mutation must use: the whole command, when the
// control's selection skipped the test.
func (phase bitePhase) passesControl(item biteCase, target biteTestTarget) (biteTestTarget, bool) {
	selector := unrelatedControl
	run := phase.rerun(target, selector)
	if run.widened {
		target.command = target.whole
	}
	if run.passedTest(target.name) {
		return target, true
	}
	detail := fmt.Sprintf("test %s must pass the control with %s=%q", target.name, bite.Env, selector)
	cause := phase.cause(run, target.name)
	phase.add(verdict{Name: item.name(), Status: failed, Detail: withCause(detail, cause)})
	return target, false
}

// cause says why a rerun failed in a reader's words: the named test's own
// output, else what the runner printed. A Go event stream is never quoted raw,
// and a failing Node test quotes only its own TAP block.
func (phase bitePhase) cause(run biteRun, name string) error {
	if run.err == nil {
		return errors.Join(run.absence(name), run.parseErr)
	}
	output := phase.failureText(run, name)
	return errors.Join(commandError(run.err, run.stdout, output), run.absence(name), run.parseErr)
}

func (phase bitePhase) failureText(run biteRun, name string) string {
	for _, item := range run.evidence.Verdicts {
		namedFailure := item.Name == name && item.Status == failed
		if namedFailure && item.Detail != "" {
			return item.Detail
		}
	}
	if text := strings.TrimSpace(run.stderr); text != "" {
		return text
	}
	if phase.module.Language == "go" {
		return goTestText(run.stdout)
	}
	return ""
}

func withCause(detail string, cause error) string {
	if cause != nil {
		return detail + ": " + cause.Error()
	}
	return detail
}

func biteExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

func hasBiteTestVerdict(evidence testEvidenceLog, name, status string) bool {
	if evidence.tests.require(name) != nil {
		return false
	}
	return slices.ContainsFunc(evidence.Verdicts, func(item verdict) bool {
		return item.Name == name && item.Status == status
	})
}
