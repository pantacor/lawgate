package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
)

// TestMain keeps the gate's progress out of the test log.
func TestMain(m *testing.M) {
	progressOutput = io.Discard
	os.Exit(m.Run())
}

// check says what it is doing on stderr while it runs, and stdout still
// carries only the report directory scripts capture.
func TestCheckReportsProgressOnStderrOnly(t *testing.T) {
	var lines bytes.Buffer
	progressOutput = &lines
	t.Cleanup(func() { progressOutput = io.Discard; progress.begin(io.Discard) })
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, "go.mod", "module example.test/fixture\n\ngo 1.25\n")
	writeFixture(t, root, module.Laws, testLaw)
	writeFixture(t, root, "law_test.go", lawTestSource)
	directory, _ := runCheck(t, root)
	if strings.Contains(directory, "\n") || !strings.Contains(directory, "run-") {
		t.Fatalf("stdout must hold only the report directory, got %q", directory)
	}
	for _, want := range []string{"module(s) in lawgate.json", "tests of . ...", "tests of . done", "bite 1/1: report-required", "finished: "} {
		if !strings.Contains(lines.String(), want) {
			t.Errorf("progress lacks %q:\n%s", want, lines.String())
		}
	}
}
