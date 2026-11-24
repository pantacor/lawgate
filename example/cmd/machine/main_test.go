package main

import (
	"bytes"
	"testing"

	worker "github.com/pantacor/lawgate/example"
	"github.com/pantacor/lawgate/review/index"
	"github.com/pantacor/lawgate/runner"
)

// TestIndexDescribesTheWorker runs this machine command the way lawgate
// review does (-index, from the module root) and checks the worker machine's
// index: every site on its source line and its declaration, the laws' verdicts.
func TestIndexDescribesTheWorker(t *testing.T) {
	t.Chdir("../..")
	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"-index", "-"}, []runner.Entry{worker.Entry()}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stderr:\n%s", code, stderr.String())
	}
	document, err := index.Decode(&stdout)
	if err != nil {
		t.Fatal(err)
	}
	machine := document.Machines[0]
	if machine.Name != "worker" || len(machine.Problems) != 0 {
		t.Fatalf("machine = %+v", machine)
	}
	for _, site := range machine.Sites {
		resolved := site.File == "worker.go" && site.Line > 0
		if !resolved || site.DeclFile != "machine.go" {
			t.Errorf("site %s: %s:%d declared at %s:%d", site.Tag, site.File, site.Line, site.DeclFile, site.DeclLine)
		}
	}
	for _, law := range machine.Laws {
		if law.Status != index.StatusHolds {
			t.Errorf("law %s is %s", law.Name, law.Status)
		}
	}
	if len(machine.Laws) != 2 {
		t.Fatalf("laws = %+v, want no-silent-errors and validate-before-store", machine.Laws)
	}
}
