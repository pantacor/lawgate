package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

const runImport = "github.com/pantacor/lawgate/run"

func edge(name, to string) machine.Edge {
	return machine.Edge{On: name, To: to, Kind: machine.EdgeNext, Name: name}
}

// genMachine: start --Ok--> work, work --Retry (limit 3)--> work,
// work --Finish--> done.
func genMachine() *machine.Machine {
	retry := edge("Retry", "work")
	retry.Limit = 3
	startEdges := []machine.Edge{edge("Ok", "work")}
	workEdges := []machine.Edge{retry, edge("Finish", "done")}
	start := machine.State{ID: "start", Edges: startEdges}
	work := machine.State{ID: "work", Edges: workEdges}
	done := machine.State{ID: "done", Terminal: true}
	states := []machine.State{start, work, done}
	return &machine.Machine{Name: "m", States: states}
}

func options(states ...string) Options {
	return Options{Package: "p", Prefix: "M", Command: "test", RunImport: runImport, States: states}
}

func generate(t *testing.T, m *machine.Machine, opts Options) string {
	t.Helper()
	out, err := Go(m, opts)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func requireAll(t *testing.T, src string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(src, want) {
			t.Errorf("generated code lacks %q:\n%s", want, src)
		}
	}
}

func TestGoMethodsAreTheDeclaredEdges(t *testing.T) {
	src := generate(t, genMachine(), options("start", "work"))
	requireAll(t, src,
		"type MStart struct",
		"func (s MStart) Ok() (MWork, error)",
		`s.run.Go("start", "Ok", "work")`,
		"func (s MWork) Finish() (MDone, error)",
		"type MDone struct",
	)
}

// Regression: the generator hard-coded Fleet's import path.
func TestGoImportsTheRunPackageItIsGiven(t *testing.T) {
	opts := options("start")
	opts.RunImport = "example.com/vendored/lawgate/run"
	requireAll(t, generate(t, genMachine(), opts), `import run "example.com/vendored/lawgate/run"`)
	opts.RunImport = ""
	if _, err := Go(genMachine(), opts); err == nil {
		t.Fatal("a generator without a run import path must refuse")
	}
}

// TestLimitedEdgeUsesRunGo keeps the count out of the caller API.
func TestLimitedEdgeUsesRunGo(t *testing.T) {
	src := generate(t, genMachine(), options("start", "work"))
	requireAll(t, src,
		"func (s MWork) Retry() (MWork, error)",
		`s.run.Go("work", "Retry", "work")`,
	)
}

// Regression: every generated state had an Enter function, so a
// pass could start, or restart, anywhere.
func TestGoEntersOnlyAtStartStates(t *testing.T) {
	src := generate(t, genMachine(), options("start", "work"))
	requireAll(t, src, "func EnterMStart(r *run.Run) (MStart, error)")
	if strings.Contains(src, "EnterMWork") {
		t.Fatalf("work is not a pass entry and must have no Enter function:\n%s", src)
	}
	m := genMachine()
	m.States[1].PassEntry = true
	requireAll(t, generate(t, m, options("work")), "func EnterMWork(r *run.Run) (MWork, error)")
	if _, err := Go(genMachine(), options("work")); err == nil {
		t.Fatal("generated states with no start state must be refused")
	}
}

func TestGoRefusesBadDeclarations(t *testing.T) {
	unnamed := genMachine()
	unnamed.States[0].Edges[0].Name = ""
	if _, err := Go(unnamed, options("start")); err == nil {
		t.Error("an unnamed edge of a generated state must be refused")
	}
	if _, err := Go(genMachine(), options("ghost")); err == nil {
		t.Error("an undeclared state must be refused")
	}
}

// TestGeneratedCodeCompilesAndRuns builds the generated code against this
// module (offline: it has no dependencies) and runs a pass through it.
func TestGeneratedCodeCompilesAndRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a module")
	}
	dir := t.TempDir()
	writeModule(t, dir, generate(t, genMachine(), options("start", "work")))
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "drift=3 limit=true stale=true empty=true enter=true path=true" {
		t.Fatalf("got %q", got)
	}
}

const passProgram = `package main

import (
	"errors"
	"fmt"

	"github.com/pantacor/lawgate/run"
	"example.com/genned/p"
)

func main() {
	m := p.Machine()
	r := run.New(m)
	r.OnDrift = func(string) {}
	start := must(p.EnterMStart(r))
	work := must(start.Ok())
	for range 3 {
		work = must(work.Retry())
	}
	_, limit := work.Retry()
	_, stale := start.Ok()
	_, repeated := p.EnterMStart(r)
	var empty p.MWork
	_, emptyErr := empty.Finish()
	must(work.Finish())
	fmt.Printf("drift=%d limit=%t stale=%t empty=%t enter=%t path=%t\n",
		len(r.Drift), errors.Is(limit, run.ErrLimit), stale != nil, emptyErr != nil,
		repeated != nil, run.IsPath(m, r.Trace) == nil)
}

func must[T any](state T, err error) T {
	if err != nil {
		panic(err)
	}
	return state
}
`

func writeModule(t *testing.T, dir, generated string) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	goMod := "module example.com/genned\n\ngo 1.25\n\nrequire github.com/pantacor/lawgate v0.0.0\n\nreplace github.com/pantacor/lawgate => " + root + "\n"
	files := map[string]string{
		"go.mod":          goMod,
		"main.go":         passProgram,
		"p/states_gen.go": generated,
		"p/machine.go":    machineSource,
	}
	for name, body := range files {
		writeFile(t, filepath.Join(dir, name), body)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// machineSource is genMachine, for the generated package to hand to main.
const machineSource = `package p

import "github.com/pantacor/lawgate/machine"

func edge(name, to string) machine.Edge {
	return machine.Edge{On: name, To: to, Kind: machine.EdgeNext, Name: name}
}

func Machine() *machine.Machine {
	retry := edge("Retry", "work")
	retry.Limit = 3
	startEdges := []machine.Edge{edge("Ok", "work")}
	workEdges := []machine.Edge{retry, edge("Finish", "done")}
	start := machine.State{ID: "start", Edges: startEdges}
	work := machine.State{ID: "work", Edges: workEdges}
	done := machine.State{ID: "done", Terminal: true}
	states := []machine.State{start, work, done}
	return &machine.Machine{Name: "m", States: states}
}
`

func TestGoRejectsNormalizedStateCollisions(t *testing.T) {
	m := genMachine()
	m.States[1].ID = "retry-now"
	m.States[2].ID = "retry_now"
	m.States[0].Edges[0].To = "retry_now"
	m.States[1].Edges[0].To = "retry-now"
	m.States[1].Edges[1].To = "retry_now"
	checkNameCollision(t, m, options("start", "retry-now", "retry_now"), "MRetryNow", "retry-now", "retry_now")
	checkNameCollision(t, m, options("start", "retry-now"), "MRetryNow", "retry-now", "retry_now")
	checkNameCollision(t, m, options("start", "start"), "MStart", "start")
}

func TestGoRejectsEntryAndOutsideTypeCollision(t *testing.T) {
	m := genMachine()
	m.States[0].ID = "Worker"
	m.States[1].ID = "EnterWorker"
	m.States[0].Edges[0].To = "EnterWorker"
	opts := options("Worker")
	opts.Prefix = ""
	checkNameCollision(t, m, opts, "EnterWorker", "state \"Worker\"", "state \"EnterWorker\"")
}

func checkNameCollision(t *testing.T, m *machine.Machine, opts Options, wants ...string) {
	t.Helper()
	_, err := Go(m, opts)
	if err == nil {
		t.Errorf("generated namespace collision returned no error for %v", wants)
		return
	}
	requireAll(t, err.Error(), wants...)
}

func TestGoRejectsNamesReservedByTheOutput(t *testing.T) {
	m := genMachine()
	m.States[0].Edges[0].Name = "run"
	checkNameCollision(t, m, options("start"), "run", "field", "start")
	m.States[0].Edges[0].Name = "Ok"
	m.States[0].ID = "run"
	opts := options("run")
	opts.Prefix = ""
	checkNameCollision(t, m, opts, "run", "state", "import")
}

func TestGoKeepsGeneratedLocalsFromShadowingStateTypes(t *testing.T) {
	m := genMachine()
	m.States[0].ID = "r"
	m.States[1].ID = "err"
	m.States[2].ID = "s"
	m.States[0].Edges[0].To = "err"
	m.States[1].Edges[0].To = "err"
	m.States[1].Edges[1].To = "s"
	opts := options("r", "err")
	opts.Prefix = ""
	requireAll(t, generate(t, m, opts),
		"func enterR(r_ *run.Run) (r, error)",
		"if err_ := s.run.Go(",
		"return err{}, err_",
		"func (s_ err) Finish() (s, error)",
		"return s{run: s_.run}, nil",
	)
}

func TestGoRejectsReservedEntryEdge(t *testing.T) {
	m := genMachine()
	m.States[0].Edges[0].Name = "enter"
	_, err := Go(m, options("start"))
	if err == nil || !strings.Contains(err.Error(), `"enter" is reserved`) {
		t.Fatalf("a declared edge named enter must be rejected, got %v", err)
	}
}
