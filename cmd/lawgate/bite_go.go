package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/mutate"
)

// A Go law's bite is gate-chosen mutation of its subject: lawgate breaks
// each guard condition of the function the law names, one mutant at a time
// through `go test -overlay`, and the law's enforcing tests must fail on
// every one. No hook in production code takes part, so moving or rewriting
// a guard cannot keep the law green.

const subjectGapPrefix = "subject gap: "

// goLawCase is the mutation check of one subject of one law.
type goLawCase struct {
	doc     string
	law     string
	subject string
}

func (item goLawCase) name() string {
	subject := "no subject"
	if item.subject != "" {
		subject = "subject " + item.subject
	}
	return fmt.Sprintf("%s (%s: %s, %s)", biteLawName, item.law, item.doc, subject)
}

func (phase bitePhase) checkGoLaw(doc string, law lawsdoc.LawPlan) {
	item := goLawCase{doc: doc, law: law.Name}
	switch {
	case law.BiteGap != "":
		phase.add(verdict{Name: item.name(), Status: pending, Detail: biteGapPrefix + law.BiteGap})
	case law.SubjectGap != "":
		phase.add(verdict{Name: item.name(), Status: pending, Detail: subjectGapPrefix + law.SubjectGap})
	case len(law.Subjects) == 0:
		detail := "names no subject: add \"- Subject: <import path>.<Func>\" for the function the law protects, or \"- Subject: not yet. <why>\""
		phase.add(verdict{Name: item.name(), Status: failed, Detail: detail})
	default:
		phase.mutateLaw(item, law)
	}
}

func (phase bitePhase) mutateLaw(item goLawCase, law lawsdoc.LawPlan) {
	proof, err := phase.goProof(item.law, law)
	if errors.Is(err, errDiscovery) {
		return // The one discovery verdict already fails the module.
	}
	for _, subject := range law.Subjects {
		item.subject = subject.String()
		if err != nil {
			phase.add(verdict{Name: item.name(), Status: failed, Detail: err.Error()})
			continue
		}
		phase.mutateSubject(item, subject, proof)
	}
}

// goProof is the law's enforcing Go tests and the fingerprint of their source.
type goProof struct {
	inputs      *goBiteInputs
	tests       []lawsdoc.TestFunc
	fingerprint string
}

// errDiscovery stops a law whose tests could not be listed; the discovery
// verdict already says why.
var errDiscovery = errors.New("test discovery failed")

func (phase bitePhase) goProof(name string, law lawsdoc.LawPlan) (goProof, error) {
	inputs, err := phase.goTests()
	if err != nil && !hasDiscoveryFailure(phase.result.Verdicts) {
		phase.result.recordError(testDiscovery, err)
	}
	if inputs == nil {
		return goProof{}, errDiscovery
	}
	proof := goProof{inputs: inputs}
	for _, enforcer := range law.Enforcers {
		if enforcer.Kind != lawsdoc.Test {
			continue
		}
		test, err := phase.passingTest(inputs, name, enforcer.Name)
		if err != nil {
			return goProof{}, err
		}
		proof.tests = append(proof.tests, test)
	}
	proof.fingerprint, err = proofFingerprint(proof.tests)
	return proof, err
}

// passingTest resolves an enforcing test that passed in the normal run: a
// mutant can only be killed by a test that holds on the real code.
func (phase bitePhase) passingTest(inputs *goBiteInputs, law, reference string) (lawsdoc.TestFunc, error) {
	test, err := inputs.tests.EnforcingTest(law, reference)
	if err != nil {
		return test, err
	}
	if err := phase.result.TestRuns.require(test.QualifiedName()); err != nil {
		return test, err
	}
	if !phase.passedNormally(test.QualifiedName()) {
		return test, fmt.Errorf("test %s must pass before its subject is mutated", test.QualifiedName())
	}
	return test, nil
}

// proofFingerprint covers each enforcing test and every test file of its
// package, where the helpers it calls live.
func proofFingerprint(tests []lawsdoc.TestFunc) (string, error) {
	parts := make([]string, 0, len(tests))
	for _, test := range tests {
		files, err := filepath.Glob(filepath.Join(filepath.Dir(test.File), "*_test.go"))
		if err != nil {
			return "", err
		}
		sum, err := filesFingerprint(files)
		if err != nil {
			return "", err
		}
		parts = append(parts, test.QualifiedName()+" "+sum)
	}
	return mutate.Key(parts...), nil
}

func filesFingerprint(files []string) (string, error) {
	parts := make([]string, 0, 2*len(files))
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			return "", err
		}
		parts = append(parts, filepath.Base(name), string(data))
	}
	return mutate.Key(parts...), nil
}

func (phase bitePhase) mutateSubject(item goLawCase, subject mutate.Subject, proof goProof) {
	target, err := phase.findSubject(proof.inputs, subject)
	if err != nil {
		phase.add(verdict{Name: item.name(), Status: failed, Detail: err.Error()})
		return
	}
	mutants, total := target.Mutants(mutate.MaxMutants)
	propagations := target.ErrorPropagations()
	if total == 0 {
		where := phase.location(target.File, target.Line())
		detail := fmt.Sprintf("subject %s (%s) has no guard condition to mutate%s: no if, for or switch case, named or returned condition its law's tests could depend on", subject, where, unmutatedNote(propagations, " (%s)"))
		phase.add(verdict{Name: item.name(), Status: failed, Detail: detail})
		return
	}
	run := mutationRun{item: item, target: target, proof: proof, mutants: mutants, total: total, propagations: propagations}
	phase.runMutation(run)
}

// findSubject lists the subject's package in the module's build and finds
// the function among its production files.
func (phase bitePhase) findSubject(inputs *goBiteInputs, subject mutate.Subject) (mutate.Target, error) {
	dir := filepath.Join(phase.root, phase.module.Path)
	command := inputs.build.SelectTest("", "", []string{subject.ImportPath})
	packages, err := inventory.GoPackages(dir, command, phase.module.Timeout())
	if err != nil {
		return mutate.Target{}, fmt.Errorf("list the package of subject %s: %w", subject, err)
	}
	for _, pkg := range packages {
		listed := pkg.ImportPath == subject.ImportPath && pkg.ForTest == "" && pkg.Error == nil
		if listed && phase.inRepository(pkg.Dir) {
			return mutate.Find(pkg.Sources(), subject)
		}
	}
	return mutate.Target{}, fmt.Errorf("subject %s is not in a package of this repository's build", subject)
}

func (phase bitePhase) inRepository(dir string) bool {
	_, inside := phase.repositoryPath(dir)
	return inside
}

func (phase bitePhase) location(file string, line int) string {
	rel, inside := phase.repositoryPath(file)
	if !inside {
		rel = file
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), line)
}

// repositoryPath is path relative to the repository root, and whether it is
// inside it. Both sides are made absolute and free of symlinks: the root may
// be "." and go list reports resolved directories.
func (phase bitePhase) repositoryPath(path string) (string, bool) {
	root, err := gosource.CanonicalRoot(phase.root)
	if err != nil {
		return "", false
	}
	resolved, err := gosource.CanonicalRoot(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, resolved)
	return rel, err == nil && filepath.IsLocal(rel)
}

// mutationRun is every mutant of one subject against one law's proof.
type mutationRun struct {
	item    goLawCase
	target  mutate.Target
	proof   goProof
	mutants []mutate.Mutant
	total   int
	// propagations counts the guards that only pass an error up: they are
	// not mutated, and every verdict says how many.
	propagations int
}

// unmutatedNote formats the error-propagation guards left unmutated into
// layout, or is empty when there are none.
func unmutatedNote(propagations int, layout string) string {
	if propagations == 0 {
		return ""
	}
	noun := "guards"
	if propagations == 1 {
		noun = "guard"
	}
	return fmt.Sprintf(layout, fmt.Sprintf("%d error-propagation %s not mutated", propagations, noun))
}

type mutantOutcome struct {
	mutant mutate.Mutant
	killed bool
	err    error
}

func (phase bitePhase) runMutation(run mutationRun) {
	key, err := mutationKey(run)
	if err != nil {
		phase.result.recordError(run.item.name(), err)
		return
	}
	if phase.cache.Killed(key) {
		phase.add(verdict{Name: run.item.name(), Status: passed, Detail: run.summary() + " (cached)"})
		return
	}
	outcomes := phase.runMutants(run)
	if phase.judge(run, outcomes) {
		_ = phase.cache.RecordKilled(key) // A cache that cannot be written only costs time.
	}
}

// mutationKey changes whenever the subject or the proof does.
func mutationKey(run mutationRun) (string, error) {
	subject, err := run.target.Fingerprint()
	if err != nil {
		return "", err
	}
	return mutate.Key(run.target.Subject.String(), subject, run.proof.fingerprint), nil
}

func (run mutationRun) summary() string {
	summary := fmt.Sprintf("the law's tests killed every mutant of %s (%d)", run.target.Subject, len(run.mutants))
	if len(run.mutants) < run.total {
		summary += fmt.Sprintf("; truncated: checked %d of %d mutants", len(run.mutants), run.total)
	}
	return summary + unmutatedNote(run.propagations, "; %s")
}

// runMutants checks the mutants in parallel: each is its own go test.
func (phase bitePhase) runMutants(run mutationRun) []mutantOutcome {
	outcomes := make([]mutantOutcome, len(run.mutants))
	slots := make(chan struct{}, mutationWorkers())
	var group sync.WaitGroup
	for index, mutant := range run.mutants {
		group.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			outcomes[index] = phase.runMutant(run, mutant)
		})
	}
	group.Wait()
	return outcomes
}

func mutationWorkers() int {
	return max(1, min(4, runtime.NumCPU()/2))
}

func (phase bitePhase) runMutant(run mutationRun, mutant mutate.Mutant) mutantOutcome {
	outcome := mutantOutcome{mutant: mutant}
	dir, err := os.MkdirTemp("", "lawgate-mutant-")
	if err != nil {
		outcome.err = err
		return outcome
	}
	defer os.RemoveAll(dir)
	overlay, err := mutate.WriteOverlay(dir, run.target.File, mutant)
	outcome.err = err
	for _, test := range run.proof.tests {
		if outcome.killed || outcome.err != nil {
			break
		}
		outcome.killed, outcome.err = phase.killsMutant(run.proof.inputs, test, overlay)
	}
	return outcome
}

// killsMutant runs one enforcing test against the mutant. A test that does
// not finish in the module's time limit is killed: the mutant changed what
// it observes.
func (phase bitePhase) killsMutant(inputs *goBiteInputs, test lawsdoc.TestFunc, overlay string) (bool, error) {
	dir := filepath.Join(phase.root, phase.module.Path)
	command := inputs.build.SelectTest(test.Name, "", goBiteOperands(inputs.build, test))
	overrides := []string{"GOFLAGS=-overlay=" + overlay}
	stdout, stderr, err := inventory.RunGateCommandWithEnv(dir, command, overrides, phase.module.Timeout())
	if errors.Is(err, context.DeadlineExceeded) {
		return true, nil
	}
	run := biteRun{err: err, code: biteExitCode(err), stdout: stdout, stderr: stderr}
	run.evidence, run.parseErr = readTestEvidence("go", stdout)
	name := test.QualifiedName()
	switch {
	case run.reports(name, failed):
		return true, nil
	case run.passedTest(name):
		return false, nil
	}
	return false, fmt.Errorf("test %s reported no result against the mutant: %w", name, phase.cause(run, name))
}

// judge records the run's verdict and reports whether every mutant died.
func (phase bitePhase) judge(run mutationRun, outcomes []mutantOutcome) bool {
	var survivors []string
	for _, outcome := range outcomes {
		where := phase.location(run.target.File, outcome.mutant.Line)
		if outcome.err != nil {
			phase.result.recordError(run.item.name(), fmt.Errorf("mutant at %s (%s): %w", where, outcome.mutant.Description, outcome.err))
			return false
		}
		if !outcome.killed {
			survivors = append(survivors, where+" "+outcome.mutant.Description)
		}
	}
	if len(survivors) == 0 {
		phase.add(verdict{Name: run.item.name(), Status: passed, Detail: run.summary()})
		return true
	}
	detail := fmt.Sprintf("%d of %d mutants of %s survived: the law's tests (%s) still pass with %s%s",
		len(survivors), len(run.mutants), run.target.Subject, testNames(run.proof.tests), strings.Join(survivors, "; "), unmutatedNote(run.propagations, " (%s)"))
	phase.add(verdict{Name: run.item.name(), Status: failed, Detail: detail})
	return false
}

func testNames(tests []lawsdoc.TestFunc) string {
	names := make([]string, 0, len(tests))
	for _, test := range tests {
		names = append(names, test.Name)
	}
	return strings.Join(names, ", ")
}

func goBiteOperands(build inventory.GoCommand, test lawsdoc.TestFunc) []string {
	if test.Package == "command-line-arguments" {
		return build.Operands
	}
	return []string{test.Package}
}
