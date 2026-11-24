package laws

import (
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

// The test machines are a typical worker: start stores the running status,
// work runs the job, finish stores it done; a fault waits in retry, a
// failure ends in failed. Each part is built by name and assembled, instead
// of one nested literal.

const storesDoneLabel = "stores-done"

const oneShotLabel = "one-shot"

func edgeTo(on, to string, kind machine.EdgeKind) machine.Edge {
	return machine.Edge{On: on, To: to, Kind: kind}
}

func site(tag string, kind machine.SiteKind, effect string) machine.Site {
	return machine.Site{Tag: tag, Kind: kind, Effect: effect}
}

func labelled(s machine.Site, label string) machine.Site {
	s.Labels = append(s.Labels, label)
	return s
}

func step(id string, sites ...machine.Site) machine.Step {
	return machine.Step{ID: id, Kind: "computer", Sites: sites}
}

func state(id string, steps []machine.Step, edges ...machine.Edge) machine.State {
	return machine.State{ID: id, Steps: steps, Edges: edges}
}

func ending(id, status string) machine.State {
	return machine.State{ID: id, Status: status, Terminal: true}
}

func catch(kind machine.SiteKind, to string) machine.Catch {
	return machine.Catch{On: kind, To: to}
}

func steps(list ...machine.Step) []machine.Step { return list }

func startState() machine.State {
	store := step("store-running", site("1", machine.SiteWrite, "store the running status"))
	return state("start", steps(store), edgeTo("stored", "work", machine.EdgeNext))
}

func workState() machine.State {
	run := step("run", site("2", machine.SiteFault, "the job retries"), site("3", machine.SiteFail, "the job fails with the cause"))
	return state("work", steps(run), edgeTo("ran", "finish", machine.EdgeNext))
}

func finishState() machine.State {
	stored := labelled(site("4", machine.SiteWrite, "store the job as done"), storesDoneLabel)
	return state("finish", steps(step("store-done", stored)), edgeTo("stored", "done", machine.EdgeExit))
}

func retryState() machine.State {
	retry := machine.State{ID: "retry", Status: "waiting", Wait: true, WokenBy: "the backoff timer"}
	retry.Edges = append(retry.Edges, edgeTo("ladder exhausted", "failed", machine.EdgeExit))
	retry.Edges = append(retry.Edges, edgeTo("next claim", "start", machine.EdgeLoop))
	return retry
}

// worker is a retrying worker on which every law of this package holds.
func worker() *machine.Machine {
	m := &machine.Machine{Name: "worker", Source: "worker.go"}
	m.States = append(m.States, startState(), workState(), finishState())
	m.States = append(m.States, ending("done", "done"), ending("failed", "failed"), retryState())
	m.Catches = append(m.Catches, catch(machine.SiteFault, "retry"), catch(machine.SiteWrite, "retry"))
	m.Catches = append(m.Catches, catch(machine.SiteFail, "failed"))
	return m
}

// oneShot is a worker labelled one-shot that fails at once on a fault.
func oneShot() *machine.Machine {
	m := &machine.Machine{Name: "copy", Source: "copy.go", Labels: []string{oneShotLabel}}
	m.States = append(m.States, startState(), workState(), finishState())
	m.States = append(m.States, ending("done", "done"), ending("failed", "failed"))
	m.Catches = append(m.Catches, catch(machine.SiteFault, "failed"), catch(machine.SiteWrite, "failed"))
	m.Catches = append(m.Catches, catch(machine.SiteFail, "failed"))
	return m
}

// mustBeValid keeps the test machines realistic: a law is only ever checked
// on a machine machine.Validate accepts.
func mustBeValid(t *testing.T, m *machine.Machine) {
	t.Helper()
	if errs := machine.Validate(m); len(errs) > 0 {
		t.Fatalf("test machine %s is not valid: %v", m.Name, errs)
	}
}

// holds checks law on m through machine.CheckLaws, which also runs the
// law's witness and fails when the witness does not break it.
func holds(t *testing.T, law machine.Law, m *machine.Machine) {
	t.Helper()
	mustBeValid(t, m)
	verdicts, err := machine.CheckLaws(m, []machine.Law{law})
	if err != nil {
		t.Fatalf("%s on %s: %v", law.Name, m.Name, err)
	}
	if !verdicts[0].Holds() {
		t.Fatalf("%s must hold on %s, got %v", law.Name, m.Name, verdicts[0].Violations)
	}
}

// breaks checks that law reports a violation containing want on m.
func breaks(t *testing.T, law machine.Law, m *machine.Machine, want string) {
	t.Helper()
	violations := law.Check(m)
	for _, violation := range violations {
		if strings.Contains(violation, want) {
			return
		}
	}
	t.Fatalf("%s on %s: want a violation with %q, got %v", law.Name, m.Name, want, violations)
}

// witnessBreaks applies the law's witness to m and checks the result still
// validates as a machine and breaks the law: the witness is a realistic
// mistake, not a malformed declaration.
func witnessBreaks(t *testing.T, law machine.Law, m *machine.Machine) {
	t.Helper()
	law.Witness(m)
	if len(law.Check(m)) == 0 {
		t.Fatalf("%s: the witness did not break the law on %s", law.Name, m.Name)
	}
	if errs := machine.Validate(m); len(errs) > 0 {
		t.Fatalf("%s: the witness made %s invalid: %v", law.Name, m.Name, errs)
	}
}
