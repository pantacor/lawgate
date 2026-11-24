package runner

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

const closingWorkerSource = `package demo

func fault(msg string) {}

func run() {
	fault("DEMO_JOB 1 left failed")
	fault("DEMO_JOB 2 right failed")
}
`

const closingDeclarations = `package demo

var left = []Site{{Tag: "1"}}

var right = []Site{{Tag: "2"}}

var again = []Site{{Match: "fault(\"DEMO"}}

var twice = []Site{{Match: "fault(\"DEMO"}}
`

// closingMachine runs step close in left and in right, each with its own
// tagged site and an untagged one that shares its match with the other.
func closingMachine() *machine.Machine {
	m := sampleMachine()
	left := closingState("left", "1", "right")
	right := closingState("right", "2", "done")
	m.Entry = "left"
	// The sample's work state would claim tag 1; left and right replace it.
	m.States = append([]machine.State{left, right}, m.States[1:]...)
	return m
}

func closingState(id, tag, to string) machine.State {
	tagged := machine.Site{Tag: tag, Kind: machine.SiteFault, Effect: "retried"}
	untagged := machine.Site{Match: `fault("DEMO`, Kind: machine.SiteFault, Effect: "retried"}
	sites := []machine.Site{tagged, untagged}
	steps := []machine.Step{{ID: "close", Kind: "io", Sites: sites}}
	edges := []machine.Edge{{On: "ok", To: to, Kind: machine.EdgeNext}}
	return machine.State{ID: id, Steps: steps, Edges: edges}
}

// The index lists each occurrence of a repeated step id under its own
// state, and two untagged sites that share a match point at their own
// declarations instead of both at the first.
func TestIndexKeepsEachOccurrenceOfAStepID(t *testing.T) {
	dir := writeWorkerSource(t, closingWorkerSource)
	writeFile(t, filepath.Join(dir, "machine.go"), closingDeclarations)
	m := closingMachine()
	target := filepath.Join(dir, "index.json")
	var stdout, stderr bytes.Buffer
	if code := runIn(dir, []string{"-index", target}, []Entry{sampleEntry(m, []machine.Law{noSilentLaw()})}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d; stderr:\n%s", code, stderr.String())
	}
	indexed := readIndex(t, target).Machines[0]
	if indexed.Failure != "" || len(indexed.Problems) != 0 {
		t.Fatalf("machine = %+v", indexed)
	}
	var where, declared []string
	for _, site := range indexed.Sites {
		where = append(where, site.State+"/"+site.Step)
		declared = append(declared, fmt.Sprintf("%s:%d", site.DeclFile, site.DeclLine))
	}
	if got := strings.Join(where, ","); got != "left/close,left/close,right/close,right/close" {
		t.Fatalf("sites = %s", got)
	}
	if got := strings.Join(declared, ","); got != "machine.go:3,machine.go:7,machine.go:5,machine.go:9" {
		t.Fatalf("declarations = %s", got)
	}
}

// The index says where each site's failure goes, and a catch that carries
// on as "carries on" with its note.
func TestIndexSaysWhereASiteIsCaught(t *testing.T) {
	m := sampleMachine()
	if got := caughtText(m, machine.SiteFault); got != "retry" {
		t.Fatalf("caught = %q, want retry", got)
	}
	logged := machine.Catch{On: machine.SiteWarn, Note: "the job log"}
	m.Catches = append(m.Catches, logged, machine.Catch{On: machine.SiteWarn})
	if got := caughtText(m, machine.SiteWarn); got != "carries on (the job log), carries on" {
		t.Fatalf("caught = %q", got)
	}
	if got := caughtText(m, machine.SiteFail); got != "" {
		t.Fatalf("a kind no catch covers is not caught, got %q", got)
	}
}
