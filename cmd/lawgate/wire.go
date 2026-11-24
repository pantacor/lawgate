package main

import (
	"strings"
	"time"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/report"
)

func checkMetadata(root string) report.Meta {
	meta := report.Meta{Generated: time.Now().UTC(), Reproduce: "lawgate check"}
	data, _, err := inventory.RunCommand(root, []string{"git", "rev-parse", "--short", "HEAD"}, 5*time.Second)
	if err == nil {
		meta.Commit = strings.TrimSpace(string(data))
	}
	return meta
}

func machineVerdicts(path string) ([]report.LawRun, error) {
	return report.ReadJUnit(path)
}

func writeReports(directory string, meta report.Meta, modules []moduleResult, notes []string) error {
	groups := make([][]report.LawRun, 0, len(modules))
	for _, module := range modules {
		groups = append(groups, reportRuns(module))
	}
	return report.WriteGate(directory, meta, groups, notes)
}

func reportRuns(module moduleResult) []report.LawRun {
	runs := append([]report.LawRun(nil), module.Machines...)
	checks := report.LawRun{Module: module.Path, Machine: "gate checks", GateChecks: true}
	for _, item := range module.Verdicts {
		if item.Status == infrastructure {
			run := report.LawRun{Module: module.Path, Machine: item.Name, Failure: item.Detail}
			runs = append(runs, run)
			continue
		}
		checks.Verdicts = append(checks.Verdicts, reportVerdict(item))
	}
	if len(checks.Verdicts) > 0 {
		runs = append(runs, checks)
	}
	if !module.Complete {
		runs = append(runs, report.FailedRun(module.Path, "checks in progress"))
	}
	return runs
}

func reportVerdict(item verdict) machine.Verdict {
	law := machine.Law{Name: item.Name}
	var violations []string
	switch item.Status {
	case failed:
		violations = []string{item.Detail}
	case pending:
		law.Pending = item.Detail
		violations = []string{item.Detail}
	case skipped:
		law.Pending = "test did not run"
		violations = []string{item.Detail}
	}
	return machine.Verdict{Law: law, Violations: violations}
}
