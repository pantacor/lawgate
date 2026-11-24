package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/report"
)

func checkMachines(root, directory string, index int, module inventory.Module, result *moduleResult) {
	if !result.Ready || !*module.Machines {
		return
	}
	raw := filepath.Join(directory, fmt.Sprintf("machine-%03d.input", index+1))
	command := module.MachineArgs(raw)
	stdout, stderr, runErr := execute(root, module.Path, command, module.Timeout())
	defer os.Remove(raw)
	runs, parseErr := machineVerdicts(raw)
	if parseErr != nil {
		result.recordError("machine report", errors.Join(parseErr, commandError(runErr, stdout, stderr)))
		return
	}
	for index := range runs {
		runs[index].Module = module.Path
	}
	result.Machines = runs
	for _, problem := range machineCoverage(result) {
		result.Verdicts = append(result.Verdicts, verdict{"machine evidence", failed, problem})
	}
	if runErr != nil && !reportedMachineFailure(runs, runErr) {
		result.recordError("machine command", commandError(runErr, stdout, stderr))
	}
}

func reportedMachineFailure(runs []report.LawRun, err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return false
	}
	return report.Sum(runs).Outcome().Refuses()
}

func checkTests(root string, module inventory.Module, result *moduleResult) {
	if !result.Ready {
		return
	}
	stdout, stderr, runErr := execute(root, module.Path, module.TestArgs(), module.Timeout())
	evidence, parseErr := readTestEvidence(module.Language, stdout)
	result.Verdicts = append(result.Verdicts, evidence.Verdicts...)
	result.TestRuns = evidence.tests
	if parseErr != nil {
		result.recordError("test evidence", errors.Join(parseErr, commandError(runErr, stdout, stderr)))
		return
	}
	var exit *exec.ExitError
	reportedFailure := errors.As(runErr, &exit) && containsFailure(evidence.Verdicts)
	if runErr != nil && !reportedFailure {
		result.recordError("test command", commandError(runErr, stdout, stderr))
	}
}

func execute(root, module string, arguments []string, timeouts ...time.Duration) ([]byte, string, error) {
	if err := refuseLinkedArguments(root, module, arguments); err != nil {
		return nil, "", err
	}
	return inventory.RunGateCommand(filepath.Join(root, module), arguments, timeouts...)
}

// refuseLinkedArguments refuses a command whose arguments name a repository
// path reached through a symbolic link: ownership protects files by their
// path, so a link could run code the owner never reviewed (the same rule
// rulechanges applies to configured commands).
func refuseLinkedArguments(root, module string, arguments []string) error {
	for _, argument := range arguments {
		if err := refuseLinkedPath(root, filepath.Join(module, argument)); err != nil {
			return fmt.Errorf("command argument %q: %w", argument, err)
		}
	}
	return nil
}

// refuseLinkedPath walks name from the repository root and reports the first
// existing component that is a symbolic link. Paths outside the repository,
// flags and names that do not exist are not repository inputs.
func refuseLinkedPath(root, name string) error {
	clean := filepath.Clean(name)
	if !filepath.IsLocal(clean) {
		return nil
	}
	current := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("resolves through the symbolic link %s: lawgate refuses symlinked inputs", current)
		}
	}
	return nil
}

func commandError(err error, stdout []byte, stderr string) error {
	if err == nil {
		return nil
	}
	if stderr == "" {
		stderr = string(stdout)
	}
	const maxDetail = 4096
	if len(stderr) > maxDetail {
		stderr = stderr[len(stderr)-maxDetail:]
	}
	return fmt.Errorf("%w: %s", err, stderr)
}

func containsFailure(items []verdict) bool {
	for _, item := range items {
		if item.Status == failed {
			return true
		}
	}
	return false
}

func machineCoverage(result *moduleResult) []string {
	expected := result.expectedMachines()
	ownLaws := documentedMachineLaws(result.Plans)
	seen := make(map[string]bool)
	var problems []string
	for _, run := range result.Machines {
		if seen[run.Machine] {
			problems = append(problems, fmt.Sprintf("duplicate machine %q in runner verdicts", run.Machine))
		}
		seen[run.Machine] = true
		if run.Failure != "" {
			continue
		}
		if !slices.Contains(expected, run.Machine) {
			problems = append(problems, fmt.Sprintf("unexpected machine %q in verdicts", run.Machine))
			continue
		}
		required := expectedLawNames(ownLaws[run.Machine], result.SharedLaws)
		problems = append(problems, missingMachineLaws(run, required)...)

	}
	for _, name := range expected {
		if !seen[name] {
			problems = append(problems, fmt.Sprintf("machine %q is missing from runner verdicts", name))
		}
	}
	return problems
}

func (result *moduleResult) expectedMachines() []string {
	expected := slices.Clone(result.Declared)
	expected = append(expected, namedMachines(result.Plans)...)
	for _, doc := range result.SharedDocs {
		expected = append(expected, namedMachines(doc.Plans)...)
	}
	slices.Sort(expected)
	return slices.Compact(expected)
}

func namedMachines(plans []lawsdoc.LawPlan) []string {
	var names []string
	for _, plan := range plans {
		names = append(names, plan.MachineNames()...)
	}
	return slices.DeleteFunc(names, func(name string) bool { return name == "*" })
}

func documentedMachineLaws(plans []lawsdoc.LawPlan) map[string][]string {
	laws := make(map[string][]string)
	for _, law := range plans {
		for _, machine := range law.MachineNames() {
			laws[machine] = append(laws[machine], law.Name)
		}
	}
	return laws
}

func expectedLawNames(own []string, shared map[string]bool) []string {
	names := slices.Clone(own)
	for name, required := range shared {
		if required {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func missingMachineLaws(run report.LawRun, required []string) []string {
	seen := make(map[string]bool)
	for _, verdict := range run.Verdicts {
		seen[verdict.Law.Name] = true
	}
	var missing []string
	for _, law := range required {
		if !seen[law] {
			missing = append(missing, fmt.Sprintf("machine %q is missing law %q from runner verdicts", run.Machine, law))
		}
	}
	return missing
}
