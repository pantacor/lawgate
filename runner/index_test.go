package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/review/index"
)

// declarationSource declares the demo machine's site, so the index can point
// from the site to its Tag literal.
const declarationSource = `package demo

var sites = []Site{
	{Tag: "1", Kind: "fault"},
}
`

func TestIndexWritesStatesSitesVerdictsAndDeclarations(t *testing.T) {
	dir := writeWorkerSource(t, validWorkerSource)
	writeFile(t, filepath.Join(dir, "machine.go"), declarationSource)
	target := filepath.Join(dir, "index.json")
	entry := sampleEntry(sampleMachine(noSilentLaw()), []machine.Law{brokenLaw("owner review")})

	var stdout, stderr bytes.Buffer
	if code := runIn(dir, []string{"-index", target}, []Entry{entry}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	document := readIndex(t, target)
	if len(document.Machines) != 1 {
		t.Fatalf("machines = %+v", document.Machines)
	}
	demo := document.Machines[0]
	clean := demo.Failure == "" && len(demo.Problems) == 0
	named := demo.Name == "demo" && demo.Source == "worker.go"
	if !named || !clean {
		t.Fatalf("machine = %+v", demo)
	}
	site := demo.Sites[0]
	placed := site.State == "work" && site.Step == "load"
	resolved := site.Line == 6 && site.Kind == "fault"
	if !placed || !resolved {
		t.Fatalf("site = %+v", site)
	}
	if site.DeclFile != "machine.go" || site.DeclLine != 4 {
		t.Fatalf("site declaration = %s:%d, want machine.go:4", site.DeclFile, site.DeclLine)
	}
	if got := strings.Join(demo.States[0].Laws, ","); got != "no-silent,forbid-fault" {
		t.Fatalf("state laws = %s", got)
	}
	wantStatus := map[string]string{"no-silent": index.StatusHolds, "forbid-fault": index.StatusPending}
	for _, law := range demo.Laws {
		if law.Status != wantStatus[law.Name] {
			t.Errorf("law %s status = %s, want %s", law.Name, law.Status, wantStatus[law.Name])
		}
	}
}

// TestIndexKeepsDriftAndStillChecksLaws: the -laws run stops at drift, the
// index records it on its line and still reports the laws, and exits 0.
func TestIndexKeepsDriftAndStillChecksLaws(t *testing.T) {
	drifted := validWorkerSource + "\nfunc extra() { fault(\"DEMO_JOB 99 unclaimed\") }\n"
	dir := writeWorkerSource(t, drifted)
	entry := sampleEntry(sampleMachine(noSilentLaw()), nil)

	var stdout, stderr bytes.Buffer
	if code := runIn(dir, []string{"-index", "-"}, []Entry{entry}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	document, err := index.Decode(&stdout)
	if err != nil {
		t.Fatal(err)
	}
	problems := document.Machines[0].Problems
	onLine := len(problems) == 1 && problems[0].Line == 9
	if !onLine || !strings.Contains(problems[0].Message, "99") {
		t.Fatalf("problems = %+v, want the unclaimed tag on line 9", problems)
	}
	demo := document.Machines[0]
	if len(demo.Laws) != 1 || demo.Laws[0].Status != index.StatusHolds {
		t.Fatalf("laws = %+v", demo.Laws)
	}
}

func TestIndexFlagsAreExclusive(t *testing.T) {
	dir := writeWorkerSource(t, validWorkerSource)
	entry := sampleEntry(sampleMachine(noSilentLaw()), nil)
	both := []string{"-index", "-", "-laws"}
	junit := []string{"-index", "-", "-junit", "x.xml"}
	unwritable := []string{"-index", filepath.Join(dir, "missing", "index.json")}
	for _, args := range [][]string{both, junit, unwritable} {
		var stdout, stderr bytes.Buffer
		if code := runIn(dir, args, []Entry{entry}, &stdout, &stderr); code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
	}
}

func TestIndexRefusesAnotherVersion(t *testing.T) {
	if _, err := index.Decode(strings.NewReader(`{"version": 2, "machines": []}`)); err == nil {
		t.Fatal("a version 2 document was accepted")
	}
}

func readIndex(t *testing.T, name string) index.Document {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	document, err := index.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func writeFile(t *testing.T, name, text string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
