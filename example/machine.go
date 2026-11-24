package worker

import (
	"regexp"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/resolve"
	"github.com/pantacor/lawgate/runner"
)

// TagPrefix identifies worker log tags in worker.go.
const TagPrefix = "WORKER_JOB"

// Entry returns the runner entry checked by example/cmd/machine.
func Entry() runner.Entry {
	return runner.Entry{
		Machine:    Machine(),
		Vocabulary: Vocabulary(),
		Prefix:     TagPrefix,
	}
}

// Vocabulary tells resolve how worker.go reports failures and writes.
func Vocabulary() resolve.Vocabulary {
	calls := map[string]machine.SiteKind{
		"fault":     machine.SiteFault,
		"failJob":   machine.SiteFail,
		"storeDone": machine.SiteWrite,
	}
	labelCalls := map[string][]string{
		"stores-done": {"storeDone"},
	}
	return resolve.Vocabulary{
		TagPattern: regexp.MustCompile(`^(WORKER_JOB \d+)`),
		Calls:      calls,
		LabelCalls: labelCalls,
	}
}

// Machine declares the worker control flow and its machine laws.
func Machine() *machine.Machine {
	catches := []machine.Catch{
		{On: machine.SiteFault, To: "retry", Note: "retry transient load errors"},
		{On: machine.SiteFail, To: "failed", Note: "reject invalid input"},
		{On: machine.SiteWrite, To: "retry", Note: "retry transient write errors"},
	}
	states := []machine.State{
		loadState(),
		validateState(),
		completeState(),
		{ID: "done", Terminal: true},
		{ID: "failed", Terminal: true},
		retryState(),
	}
	return &machine.Machine{
		Name:    "worker",
		Source:  "worker.go",
		Doc:     "Loads, validates, and completes one job.",
		Entry:   "load",
		Laws:    workerLaws(),
		Catches: catches,
		States:  states,
	}
}

func loadState() machine.State {
	sites := []machine.Site{{Tag: "1", Kind: machine.SiteFault}}
	steps := []machine.Step{{ID: "load-job", Kind: "store", Sites: sites}}
	edges := []machine.Edge{{On: "ok", To: "validate", Kind: machine.EdgeNext, Name: "Validate"}}
	return machine.State{ID: "load", Func: "Process", Steps: steps, Edges: edges}
}

func validateState() machine.State {
	sites := []machine.Site{{Tag: "2", Kind: machine.SiteFail}}
	steps := []machine.Step{{ID: "validate-input", Kind: "check", Sites: sites}}
	edges := []machine.Edge{{On: "ok", To: "complete", Kind: machine.EdgeNext, Name: "Complete"}}
	return machine.State{ID: "validate", Func: "Process", Steps: steps, Edges: edges}
}

func completeState() machine.State {
	site := machine.Site{Tag: "3", Kind: machine.SiteWrite, Labels: []string{"stores-done"}}
	steps := []machine.Step{{ID: "store-done", Kind: "store", Sites: []machine.Site{site}}}
	edges := []machine.Edge{{On: "ok", To: "done", Kind: machine.EdgeExit, Name: "Finish"}}
	return machine.State{ID: "complete", Func: "Process", Steps: steps, Edges: edges}
}

func retryState() machine.State {
	edge := machine.Edge{On: "again", To: "load", Kind: machine.EdgeLoop, Limit: 3, Name: "Retry"}
	return machine.State{
		ID:      "retry",
		Wait:    true,
		WokenBy: "backoff timer",
		Edges:   []machine.Edge{edge},
	}
}

func workerLaws() []machine.Law {
	return []machine.Law{
		noSilentErrorsLaw(),
		validateBeforeStoreLaw(),
	}
}

func noSilentErrorsLaw() machine.Law {
	return machine.Law{
		Name: "no-silent-errors",
		Doc:  "No failure site in the worker machine may swallow an error silently.",
		Check: func(m *machine.Machine) []string {
			var violations []string
			m.Sites(func(st *machine.State, step *machine.Step, site *machine.Site) {
				if site.Kind == machine.SiteSilent {
					violations = append(violations, machine.Siteline(st, step, site))
				}
			})
			return violations
		},
		Witness: func(m *machine.Machine) {
			m.States[0].Steps[0].Sites[0].Kind = machine.SiteSilent
		},
	}
}

func validateBeforeStoreLaw() machine.Law {
	return machine.Law{
		Name: "validate-before-store",
		Doc:  "Every path through the worker machine validates the job before storing the result.",
		Check: func(m *machine.Machine) []string {
			if m.AlwaysBefore("validate-input", "store-done") {
				return nil
			}
			return []string{"step validate-input does not always precede store-done"}
		},
		Witness: func(m *machine.Machine) {
			bypass := machine.Edge{On: "skip", To: "complete", Kind: machine.EdgeNext}
			m.States[0].Edges = append(m.States[0].Edges, bypass)
		},
	}
}
