package render

import "github.com/pantacor/lawgate/machine"

// The test machine is a small worker, built part by part instead of one
// nested literal: start stores the running status, work runs the job and
// may wait for a retry, finish stores it done.

func site(tag string, kind machine.SiteKind, effect string, line int) machine.Site {
	return machine.Site{Tag: tag, Kind: kind, Effect: effect, Line: line}
}

func edge(on, to string, kind machine.EdgeKind) machine.Edge {
	return machine.Edge{On: on, To: to, Kind: kind}
}

func startState() machine.State {
	sites := []machine.Site{site("1", machine.SiteWrite, "store the running status", 10)}
	steps := []machine.Step{{ID: "store-running", Kind: "store", Sites: sites}}
	edges := []machine.Edge{edge("stored", "work", machine.EdgeNext)}
	return machine.State{ID: "start", Func: "Start", Status: "running", Steps: steps, Edges: edges}
}

func workState() machine.State {
	fault := site("2", machine.SiteFault, "the job retries", 20)
	fail := machine.Site{Match: "failJob(", Kind: machine.SiteFail, Effect: "the job fails with the cause", Line: 21}
	steps := []machine.Step{{ID: "run", Kind: "compute", Doc: "runs the job", Sites: []machine.Site{fault, fail}}}
	again := edge("busy", "work", machine.EdgeLoop)
	again.Guard, again.Limit = "the lock is held", 2
	edges := []machine.Edge{edge("ran", "finish", machine.EdgeNext), again}
	return machine.State{ID: "work", Func: "Work", When: "the job is claimed", Steps: steps, Edges: edges}
}

func finishState() machine.State {
	stored := site("3", machine.SiteWrite, "store the job as done", 30)
	stored.Labels = []string{"stores-done"}
	steps := []machine.Step{{ID: "store-done", Kind: "store", Sites: []machine.Site{stored}}}
	edges := []machine.Edge{edge("stored", "done", machine.EdgeExit)}
	return machine.State{ID: "finish", Steps: steps, Edges: edges}
}

func retryState() machine.State {
	edges := []machine.Edge{edge("next claim", "work", machine.EdgeLoop), edge("ladder exhausted", "failed", machine.EdgeExit)}
	return machine.State{ID: "retry", Status: "waiting", Wait: true, PassEntry: true, Doc: "the job says when it retries", WokenBy: "the backoff timer", Edges: edges}
}

func worker() *machine.Machine {
	m := &machine.Machine{Name: "demo", Source: "worker.go", Test: "go run ./cmd/machine -laws", Labels: []string{"retrying"}}
	m.States = append(m.States, startState(), workState(), finishState(), retryState())
	m.States = append(m.States, machine.State{ID: "done", Terminal: true}, machine.State{ID: "failed", Terminal: true})
	faults := machine.Catch{On: machine.SiteFault, To: "retry", Note: "the job retries on the ladder", Limit: 3}
	fails := machine.Catch{On: machine.SiteFail, To: "failed", Note: "the job fails"}
	m.Catches = append(m.Catches, faults, fails)
	m.Findings = append(m.Findings, machine.Finding{Severity: "high", Line: 20, Text: "the retry hides the cause"})
	return m
}
