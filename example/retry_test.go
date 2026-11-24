package worker

import (
	"errors"
	"testing"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/run"
)

func TestWorkerRetryLimitAppliesAcrossThePass(t *testing.T) {
	worker := Machine()
	pass := run.New(worker)
	pass.OnDrift = func(string) {}
	if _, err := enterWorkerLoad(pass); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if err := retryLoad(t, pass); err != nil {
			t.Fatalf("retry %d: %v", attempt, err)
		}
	}
	if err := retryLoad(t, pass); !errors.Is(err, run.ErrLimit) {
		t.Fatalf("fourth retry: got %v, want %v", err, run.ErrLimit)
	}
	if pass.At() != "retry" || len(pass.Trace) != 8 {
		t.Fatalf("rejected retry changed state or trace: %q, %v", pass.At(), pass.Trace)
	}
	if err := run.IsPath(worker, pass.Trace); err != nil {
		t.Fatalf("replay rejected accepted moves: %v", err)
	}
	excess := run.Transition{From: "retry", Edge: "Retry", To: "load"}
	if err := run.IsPath(worker, append(pass.Trace, excess)); !errors.Is(err, run.ErrLimit) {
		t.Fatalf("replay of fourth retry: got %v, want %v", err, run.ErrLimit)
	}
	fresh := run.New(worker)
	if _, err := enterWorkerLoad(fresh); err != nil {
		t.Fatal(err)
	}
	if err := retryLoad(t, fresh); err != nil {
		t.Fatalf("first retry in a fresh pass: %v", err)
	}
}

func retryLoad(t *testing.T, pass *run.Run) error {
	t.Helper()
	if err := pass.Catch("load", machine.SiteFault, "retry"); err != nil {
		t.Fatalf("load fault: %v", err)
	}
	retry := workerRetry{run: pass}
	_, err := retry.Retry()
	return err
}
