package runner

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

// twoEntries are the sample machine and a copy named second.
func twoEntries() []Entry {
	second := sampleMachine(noSilentLaw())
	second.Name = "second"
	return []Entry{sampleEntry(sampleMachine(noSilentLaw()), nil), sampleEntry(second, nil)}
}

// run runs the runner over entries in a module holding src and returns its
// exit code, standard output and standard error.
func run(t *testing.T, src string, entries []Entry, args ...string) (int, string, string) {
	t.Helper()
	dir := writeWorkerSource(t, src)
	var stdout, stderr bytes.Buffer
	code := runIn(dir, args, entries, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestListShowsStateAndLawCounts(t *testing.T) {
	code, stdout, _ := run(t, validWorkerSource, twoEntries(), "-list")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if code != 0 || len(lines) != 2 {
		t.Fatalf("exit %d, list:\n%s", code, stdout)
	}
	fields := strings.Fields(lines[1])
	want := []string{"second", "worker.go", "3", "states,", "1", "law"}
	if strings.Join(fields, " ") != strings.Join(want, " ") {
		t.Fatalf("list line = %q, want the fields %v", lines[1], want)
	}
}

func TestMachineSelectsOneMachine(t *testing.T) {
	code, stdout, _ := run(t, validWorkerSource, twoEntries(), "-list", "-machine", "second")
	onlySecond := !strings.Contains(stdout, "demo") && strings.Contains(stdout, "second")
	if code != 0 || !onlySecond {
		t.Fatalf("exit %d, list:\n%s", code, stdout)
	}
	code, _, stderr := run(t, validWorkerSource, twoEntries(), "-machine", "ghost")
	if code != 2 || !strings.Contains(stderr, `no machine "ghost" (see -list)`) {
		t.Fatalf("an unknown machine: exit %d, stderr %q", code, stderr)
	}
}

func TestOutlineIsTheDefaultView(t *testing.T) {
	code, stdout, stderr := run(t, validWorkerSource, twoEntries())
	shown := strings.Contains(stdout, "demo  worker.go") && strings.Contains(stdout, "second  worker.go")
	plain := shown && !strings.Contains(stdout, "\x1b[")
	if code != 0 || !plain {
		t.Fatalf("exit %d, stderr %q, outline:\n%s", code, stderr, stdout)
	}
	resolved := strings.Contains(stdout, "    ⚡ fault  1:6") && strings.Contains(stdout, "3 states, 0 fail, 1 fault")
	if !resolved {
		t.Fatalf("the outline shows each resolved site and the summary:\n%s", stdout)
	}
	_, coloured, _ := run(t, validWorkerSource, twoEntries(), "-color")
	if !strings.Contains(coloured, "\x1b[") {
		t.Fatalf("-color colours the outline:\n%s", coloured)
	}
}

// A view is still shown when the declaration drifted from its source, but
// the drift is on standard error and the exit status is 1.
func TestViewsReportDrift(t *testing.T) {
	drifted := strings.Replace(validWorkerSource, "DEMO_JOB 1", "DEMO_JOB 2", 1)
	code, stdout, stderr := run(t, drifted, twoEntries()[:1])
	shownWithDrift := strings.Contains(stdout, "demo  worker.go") && strings.Contains(stderr, "drift: demo: ")
	if code != 1 || !shownWithDrift {
		t.Fatalf("exit %d, stderr %q, outline:\n%s", code, stderr, stdout)
	}
	code, _, stderr = run(t, drifted, twoEntries()[:1], "-brief", "work")
	if code != 1 || !strings.Contains(stderr, "drift: demo: ") {
		t.Fatalf("brief over drift: exit %d, stderr %q", code, stderr)
	}
}

func TestJSONWritesTheMachineDriftLawsAndLines(t *testing.T) {
	code, stdout, stderr := run(t, validWorkerSource, twoEntries(), "-json", "-machine", "demo")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var view MachineView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	declared := view.Name == "demo" && len(view.States) == 3 && len(view.Catches) == 1
	if !declared || view.State("work").Steps[0].Sites[0].Line != 6 {
		t.Fatalf("view machine = %+v", view.Machine)
	}
	holds := len(view.Laws) == 1 && view.Laws[0].Status == "holds"
	clean := len(view.Drift) == 0 && view.Lines[5] == `	fault("DEMO_JOB 1 load failed")`
	if !holds || !clean {
		t.Fatalf("view laws %+v, drift %+v, lines %q", view.Laws, view.Drift, view.Lines)
	}
	if !strings.HasPrefix(view.Summary, "3 states") {
		t.Fatalf("summary = %q", view.Summary)
	}
}

func TestJSONPlacesDriftOnItsLine(t *testing.T) {
	extra := validWorkerSource + "\nfunc more() {\n\tfault(\"DEMO_JOB 9 more\")\n}\n"
	code, stdout, _ := run(t, extra, twoEntries()[:1], "-json")
	var view MachineView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	placed := len(view.Drift) == 1 && view.Drift[0].Line == 10 && view.Drift[0].File == "worker.go"
	if code != 1 || !placed {
		t.Fatalf("exit %d, drift %+v", code, view.Drift)
	}
}

func TestBriefShowsOneState(t *testing.T) {
	code, stdout, _ := run(t, validWorkerSource, twoEntries(), "-brief", "work", "-machine", "demo")
	if code != 0 || !strings.Contains(stdout, `# Change state "work" of demo`) {
		t.Fatalf("exit %d, brief:\n%s", code, stdout)
	}
	code, _, stderr := run(t, validWorkerSource, twoEntries(), "-brief", "ghost", "-machine", "demo")
	if code != 2 || !strings.Contains(stderr, `machine demo has no state "ghost"`) {
		t.Fatalf("an undeclared state: exit %d, stderr %q", code, stderr)
	}
}

// -json and -brief show one machine: with several declared, -machine
// must say which.
func TestOneMachineViewsNeedAMachine(t *testing.T) {
	asJSON := []string{"-json"}
	asBrief := []string{"-brief", "work"}
	for _, args := range [][]string{asJSON, asBrief} {
		code, _, stderr := run(t, validWorkerSource, twoEntries(), args...)
		if code != 2 || !strings.Contains(stderr, "shows one machine: pass -machine NAME") {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}

func TestFlagsAskForOneThing(t *testing.T) {
	twoViews := []string{"-list", "-json"}
	lawsAndBrief := []string{"-laws", "-brief", "work"}
	colouredList := []string{"-color", "-list"}
	junitList := []string{"-junit", "out.xml", "-list"}
	for _, args := range [][]string{twoViews, lawsAndBrief, colouredList, junitList} {
		if code, _, _ := run(t, validWorkerSource, twoEntries(), args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestViewsRefuseAnEmptyRunner(t *testing.T) {
	var outline []string
	listing := []string{"-list"}
	for _, args := range [][]string{outline, listing} {
		code, _, stderr := run(t, validWorkerSource, nil, args...)
		if code != 2 || !strings.Contains(stderr, "no machines declared") {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}

// -laws with -machine checks that machine only.
func TestLawsWithMachineChecksOne(t *testing.T) {
	broken := sampleMachine(noSilentLaw())
	broken.Name = "broken"
	broken.Laws = append(broken.Laws, brokenLaw(""))
	entries := append(twoEntries(), sampleEntry(broken, []machine.Law{}))
	if code, _, _ := run(t, validWorkerSource, entries, "-laws"); code != 1 {
		t.Fatalf("every machine: exit %d, want 1 for the broken law", code)
	}
	if code, stdout, _ := run(t, validWorkerSource, entries, "-laws", "-machine", "demo"); code != 0 || strings.Contains(stdout, "broken") {
		t.Fatalf("-machine demo: exit %d, report:\n%s", code, stdout)
	}
}
