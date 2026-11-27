package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// progressLog reports what check is doing while it runs, one line per step,
// so a long gate (a CI job, a slow test suite) is never silent. It writes to
// stderr: stdout carries only the report directory, which scripts capture.
type progressLog struct {
	mu    sync.Mutex
	out   io.Writer
	start time.Time
}

// progress is silent until checkCommand points it at stderr, so tests that
// call check directly print nothing.
var progress = &progressLog{out: io.Discard}

// progressOutput is where checkCommand sends progress.
var progressOutput io.Writer = os.Stderr

func (log *progressLog) begin(out io.Writer) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.out = out
	log.start = time.Now()
}

// step prints one line, prefixed with the time since the check began.
func (log *progressLog) step(format string, args ...any) {
	log.mu.Lock()
	defer log.mu.Unlock()
	elapsed := time.Duration(0)
	if !log.start.IsZero() {
		elapsed = time.Since(log.start).Truncate(time.Second)
	}
	fmt.Fprintf(log.out, "lawgate [%s] %s\n", elapsed, fmt.Sprintf(format, args...))
}

// timed prints a step and returns a function that prints its end and how
// long it took.
func (log *progressLog) timed(format string, args ...any) func() {
	label := fmt.Sprintf(format, args...)
	log.step("%s ...", label)
	began := time.Now()
	return func() {
		log.step("%s done (%s)", label, time.Since(began).Truncate(time.Second))
	}
}

// moduleLabel names a module in progress lines; the root module is ".".
func moduleLabel(path string) string {
	if path == "" {
		return "."
	}
	return path
}

// verdictCounts counts the failed, pending and passed verdicts of results.
func verdictCounts(results []moduleResult) (failures, pendings, passes int) {
	counts := map[string]int{}
	for _, result := range results {
		countStatuses(result.Verdicts, counts)
	}
	return counts[failed] + counts[infrastructure], counts[pending], counts[passed]
}

func countStatuses(items []verdict, counts map[string]int) {
	for _, item := range items {
		counts[item.Status]++
	}
}
