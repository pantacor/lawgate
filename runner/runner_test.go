package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/report"
	"github.com/pantacor/lawgate/resolve"
)

func TestRunResolvesChecksAndWritesJUnit(t *testing.T) {
	dir := writeWorkerSource(t, validWorkerSource)
	junitPath := filepath.Join(dir, "laws.xml")
	entry := sampleEntry(sampleMachine(noSilentLaw()), nil)

	var stdout, stderr bytes.Buffer
	code := runIn(dir, []string{"-laws", "-junit", junitPath}, []Entry{entry}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	runs, err := report.ReadJUnit(junitPath)
	if err != nil {
		t.Fatalf("ReadJUnit: %v", err)
	}
	totals := report.Sum(runs)
	if totals.Outcome() != report.OutcomeHolds || totals.Checked != 1 {
		t.Fatalf("unexpected totals %+v from runs %+v", totals, runs)
	}
}

func TestRunFailsOnSourceDrift(t *testing.T) {
	driftedSource := validWorkerSource + "\nfunc extra() { fault(\"DEMO_JOB 99 unclaimed\") }\n"
	dir := writeWorkerSource(t, driftedSource)
	junitPath := filepath.Join(dir, "drift.xml")
	entry := sampleEntry(sampleMachine(noSilentLaw()), nil)

	var stdout, stderr bytes.Buffer
	code := runIn(dir, []string{"-laws", "-junit", junitPath}, []Entry{entry}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 on source drift", code)
	}
	runs, err := report.ReadJUnit(junitPath)
	if err != nil {
		t.Fatalf("ReadJUnit: %v", err)
	}
	if report.Sum(runs).Outcome() != report.OutcomeFailed {
		t.Fatalf("outcome = %s, want failed; runs = %+v", report.Sum(runs).Outcome(), runs)
	}
	if !strings.Contains(runs[0].Failure, "99") {
		t.Fatalf("expected drift failure mentioning tag 99, got %q", runs[0].Failure)
	}
}

func TestRunFailsOnBrokenLawAndAllowsPending(t *testing.T) {
	dir := writeWorkerSource(t, validWorkerSource)
	brokenPath := filepath.Join(dir, "broken.xml")
	brokenEntry := sampleEntry(sampleMachine(), []machine.Law{brokenLaw("")})

	var stdout, stderr bytes.Buffer
	code := runIn(dir, []string{"-laws", "-junit", brokenPath}, []Entry{brokenEntry}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 on broken law", code)
	}
	runs, err := report.ReadJUnit(brokenPath)
	if err != nil {
		t.Fatalf("ReadJUnit: %v", err)
	}
	if report.Sum(runs).Outcome() != report.OutcomeBroken {
		t.Fatalf("outcome = %s, want broken", report.Sum(runs).Outcome())
	}

	pendingPath := filepath.Join(dir, "pending.xml")
	pendingEntry := sampleEntry(sampleMachine(), []machine.Law{brokenLaw("owner review")})
	stdout.Reset()
	stderr.Reset()
	code = runIn(dir, []string{"-laws", "-junit", pendingPath}, []Entry{pendingEntry}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 on pending law; stderr:\n%s", code, stderr.String())
	}
}

func TestRunUsageAndIOErrorsExitTwo(t *testing.T) {
	dir := writeWorkerSource(t, validWorkerSource)
	entry := sampleEntry(sampleMachine(noSilentLaw()), nil)

	var stdout, stderr bytes.Buffer
	if code := runIn(dir, []string{"-laws", "-list"}, []Entry{entry}, &stdout, &stderr); code != 2 {
		t.Fatalf("two actions exit = %d, want 2", code)
	}
	if code := runIn(dir, []string{"-laws", "extra"}, []Entry{entry}, &stdout, &stderr); code != 2 {
		t.Fatalf("positional arg exit = %d, want 2", code)
	}
	badPath := filepath.Join(dir, "no-such-dir", "out.xml")
	if code := runIn(dir, []string{"-laws", "-junit", badPath}, []Entry{entry}, &stdout, &stderr); code != 2 {
		t.Fatalf("unwritable junit exit = %d, want 2", code)
	}
}

func TestCheckRejectsInvalidEntries(t *testing.T) {
	dir := writeWorkerSource(t, validWorkerSource)
	empty := CheckIn(dir, nil)
	if report.Sum(empty).Outcome() != report.OutcomeFailed {
		t.Fatalf("empty entries outcome = %s, want failed", report.Sum(empty).Outcome())
	}

	first := sampleEntry(sampleMachine(noSilentLaw()), nil)
	second := sampleEntry(sampleMachine(noSilentLaw()), nil)
	dup := CheckIn(dir, []Entry{first, second})
	if report.Sum(dup).Outcome() != report.OutcomeFailed {
		t.Fatalf("duplicate machine outcome = %s, want failed", report.Sum(dup).Outcome())
	}

	missingSrc := sampleEntry(sampleMachine(noSilentLaw()), nil)
	missingSrc.Source = "missing.go"
	missing := CheckIn(dir, []Entry{missingSrc})
	if report.Sum(missing).Outcome() != report.OutcomeFailed {
		t.Fatalf("missing source outcome = %s, want failed", report.Sum(missing).Outcome())
	}
}

func TestRunPrintsMarkdownWithoutJUnit(t *testing.T) {
	dir := writeWorkerSource(t, validWorkerSource)
	entry := sampleEntry(sampleMachine(noSilentLaw()), nil)

	var stdout, stderr bytes.Buffer
	code := runIn(dir, []string{"-laws"}, []Entry{entry}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "demo") {
		t.Fatalf("expected machine name in markdown stdout, got:\n%s", stdout.String())
	}
}

const validWorkerSource = `package demo

func fault(msg string) {}

func run() {
	fault("DEMO_JOB 1 load failed")
}
`

func writeWorkerSource(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "worker.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}

func sampleEntry(m *machine.Machine, extra []machine.Law) Entry {
	calls := map[string]machine.SiteKind{"fault": machine.SiteFault}
	vocab := resolve.Vocabulary{
		TagPattern: regexp.MustCompile(`^(DEMO_JOB \d+)`),
		Calls:      calls,
	}
	return Entry{
		Machine:    m,
		Vocabulary: vocab,
		Prefix:     "DEMO_JOB",
		Laws:       extra,
	}
}

func sampleMachine(laws ...machine.Law) *machine.Machine {
	sites := []machine.Site{{Tag: "1", Kind: machine.SiteFault}}
	steps := []machine.Step{{ID: "load", Kind: "io", Sites: sites}}
	edges := []machine.Edge{{On: "ok", To: "done", Kind: machine.EdgeExit}}
	work := machine.State{ID: "work", Steps: steps, Edges: edges}
	done := machine.State{ID: "done", Terminal: true}
	retry := machine.State{ID: "retry", Terminal: true}
	catches := []machine.Catch{{On: machine.SiteFault, To: "retry"}}
	return &machine.Machine{
		Name:    "demo",
		Source:  "worker.go",
		Entry:   "work",
		Laws:    laws,
		Catches: catches,
		States:  []machine.State{work, done, retry},
	}
}

func noSilentLaw() machine.Law {
	return machine.Law{
		Name: "no-silent",
		Doc:  "No site may swallow errors silently.",
		Check: func(m *machine.Machine) []string {
			var out []string
			m.Sites(func(st *machine.State, step *machine.Step, site *machine.Site) {
				if site.Kind == machine.SiteSilent {
					out = append(out, machine.Siteline(st, step, site))
				}
			})
			return out
		},
		Witness: func(m *machine.Machine) {
			m.States[0].Steps[0].Sites[0].Kind = machine.SiteSilent
		},
	}
}

func brokenLaw(pending string) machine.Law {
	return machine.Law{
		Name:    "forbid-fault",
		Doc:     "Test law that reports fault sites.",
		Pending: pending,
		Check: func(m *machine.Machine) []string {
			var out []string
			m.Sites(func(st *machine.State, step *machine.Step, site *machine.Site) {
				if site.Kind == machine.SiteFault {
					out = append(out, machine.Siteline(st, step, site))
				}
			})
			return out
		},
		Witness: func(m *machine.Machine) {
			extra := machine.Site{Tag: "witness", Kind: machine.SiteFault}
			m.States[0].Steps[0].Sites = append(m.States[0].Steps[0].Sites, extra)
		},
	}
}
