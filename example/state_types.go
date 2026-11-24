package worker

import (
	"io"

	"github.com/pantacor/lawgate/gen"
)

//go:generate go run ./cmd/genstates

// StateTypeTarget is shared by generation and the freshness test.
func StateTypeTarget() gen.Target {
	worker := Machine()
	states := make([]string, 0, len(worker.States))
	for _, state := range worker.States {
		states = append(states, state.ID)
	}
	options := gen.Options{
		Package:   "worker",
		Prefix:    "worker",
		Command:   "go generate ./...",
		RunImport: "github.com/pantacor/lawgate/run",
		States:    states,
	}
	return gen.Target{Path: "states_gen.go", Machine: worker, Options: options}
}

// CheckWorkerStateTypes checks that the generated worker state types are current.
func CheckWorkerStateTypes(out io.Writer) error {
	return gen.ReportCheck(out, StateTypeTarget())
}
