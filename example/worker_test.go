package worker

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/readability"
	"github.com/pantacor/lawgate/report"
	"github.com/pantacor/lawgate/resolve"
	"github.com/pantacor/lawgate/runner"
)

// TestWorkerStatesAreFresh enforces generated-is-fresh.
func TestWorkerStatesAreFresh(t *testing.T) {
	if err := CheckWorkerStateTypes(io.Discard); err != nil {
		t.Fatal(err)
	}
	path := StateTypeTarget().Path
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	edited := append(source, []byte("\n// Hand edit.\n")...)
	if err := os.WriteFile(path, edited, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkerStateTypes(io.Discard); err == nil || !strings.Contains(err.Error(), path+": stale") {
		t.Fatalf("edited state types should be stale, got %v", err)
	}
}

type memoryStore struct {
	job    Job
	stored int
}

func (s *memoryStore) Load(string) (Job, error) {
	return s.job, nil
}

func (s *memoryStore) StoreDone(Job) error {
	s.stored++
	return nil
}

// TestRejectsEmptyPayload enforces rejects-empty-payload.
func TestRejectsEmptyPayload(t *testing.T) {
	job := Job{ID: "job-1", Payload: "   "}
	store := &memoryStore{job: job}
	if err := Process("job-1", store); err == nil {
		t.Fatal("expected error for empty payload, got nil")
	}
	if store.stored != 0 {
		t.Fatalf("StoreDone called %d times, want 0", store.stored)
	}
}

func TestWorkerMachineChecksCleanly(t *testing.T) {
	runs := runner.Check([]runner.Entry{Entry()})
	totals := report.Sum(runs)
	if totals.Outcome() != report.OutcomeHolds || totals.Checked != 2 {
		t.Fatalf("unexpected totals %+v from runs %+v", totals, runs)
	}
}

func TestWorkerMachineDetectsDrift(t *testing.T) {
	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	drifted := append(src, []byte("\nfunc stray() { _ = fault(\"WORKER_JOB 99 stray\", errInvalidJob) }\n")...)
	errs := resolve.Resolve(Machine(), drifted, Vocabulary(), TagPrefix)
	if len(errs) == 0 {
		t.Fatal("expected drift error for unclaimed tag WORKER_JOB 99, got none")
	}
}

func TestExampleLawsDocMatchesCode(t *testing.T) {
	doc, err := os.ReadFile("LAWS.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	runs := runner.Check([]runner.Entry{Entry()})
	machines := map[string][]string{}
	for _, r := range runs {
		for _, v := range r.Verdicts {
			machines[r.Machine] = append(machines[r.Machine], v.Law.Name)
		}
	}
	tests, err := lawsdoc.DiscoverTests(".")
	if err != nil {
		t.Fatalf("DiscoverTests: %v", err)
	}
	inputs := lawsdoc.Inputs{
		Required: machines,
		Machines: machines,
		Tests:    tests,
		RepoRoot: "..",
		DocPath:  "example/LAWS.md",
	}
	if problems := lawsdoc.Check(string(doc), inputs); len(problems) > 0 {
		t.Fatalf("LAWS.md problems: %v", problems)
	}
}

func TestExampleFollowsReadability(t *testing.T) {
	findings, err := readability.CheckDir(".", nil)
	if err != nil {
		t.Fatalf("CheckDir: %v", err)
	}
	for _, f := range findings {
		t.Errorf("readability [%s] %s:%d (%s): %s", f.Rule, f.Path, f.Line, f.Func, f.Message)
	}
}
