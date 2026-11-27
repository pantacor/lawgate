package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/report"
)

type verdict struct {
	Name   string
	Status string
	Detail string
}

const (
	passed         = "passed"
	failed         = "failed"
	skipped        = "skipped"
	pending        = "pending"
	infrastructure = "infrastructure failure"
)

type moduleResult struct {
	Path       string
	Ready      bool
	Complete   bool
	Verdicts   []verdict
	Machines   []report.LawRun
	Plans      []lawsdoc.LawPlan
	TestRuns   testCompletions
	Declared   []string
	SharedLaws map[string]bool
	SharedDocs []lawDocument
	Timeout    time.Duration
}

func (result *moduleResult) recordError(name string, err error) {
	result.Verdicts = append(result.Verdicts, verdict{name, infrastructure, err.Error()})
}

func checkCommand(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	base := flags.String("base", "", "replay approved proofs from this Git commit")
	parent := flags.String("out", "laws-report", "parent directory for run reports")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("check accepts flags only")
	}
	absoluteRoot, err := gosource.CanonicalRoot(*root)
	if err != nil {
		return err
	}
	reportParent := *parent
	if !filepath.IsAbs(reportParent) {
		reportParent = filepath.Join(absoluteRoot, reportParent)
	}
	if err := os.MkdirAll(reportParent, 0755); err != nil {
		return err
	}
	directory, err := os.MkdirTemp(reportParent, "run-")
	if err != nil {
		return err
	}
	fmt.Fprintln(output, directory)
	progress.begin(progressOutput)
	progress.step("checking %s; reports in %s", absoluteRoot, directory)
	return check(absoluteRoot, directory, *base)
}

func check(root, directory, base string) error {
	meta := checkMetadata(root)
	config, err := inventory.Load(root)
	if err != nil {
		result := moduleResult{Path: "inventory", Complete: true}
		result.recordError(inventory.Filename, err)
		return errors.Join(err, writeReports(directory, meta, []moduleResult{result}, nil))
	}
	progress.step("%d module(s) in %s", len(config.Modules), inventory.Filename)
	results := make([]moduleResult, len(config.Modules))
	for index, module := range config.Modules {
		results[index] = prepareModule(root, module)
	}
	done := progress.timed("machine inventory and generated files")
	goModules, moduleErr := inventory.GoModules(root)
	machineProblems := checkMachineInventory(root, config, results, goModules, moduleErr)
	generationProblems := checkGeneration(root, config, results, goModules)
	done()
	meta.Product = config.Product
	reportErrors := runModulePhases(root, directory, config, results, meta)
	done = progress.timed("repository checks")
	repo, notes := checkRepository(root, config)
	recordRepositoryProblems(&repo, "machine inventory", machineProblems)
	recordRepositoryProblems(&repo, freshnessLaw, generationProblems)
	done()
	done = progress.timed("proof lock%s", replayLabel(base))
	checkAnchoredProofs(root, base, config, &repo)
	done()
	if len(repo.Verdicts) > 0 {
		results = append(results, repo)
	}
	reportErrors = errors.Join(reportErrors, writeReports(directory, meta, results, notes))
	failures, pendings, passes := verdictCounts(results)
	progress.step("finished: %d passed, %d failed, %d pending", passes, failures, pendings)
	if slices.ContainsFunc(results, moduleFailed) {
		return errors.Join(reportErrors, fmt.Errorf("law checks failed; see %s", directory))
	}
	return reportErrors
}

func replayLabel(base string) string {
	if base == "" {
		return ""
	}
	return " (replaying the proofs of " + base + ")"
}

func recordRepositoryProblems(result *moduleResult, name string, problems []string) {
	if len(problems) > 0 {
		recordProblems(result, name, problems)
	}
}

func runModulePhases(root, directory string, config inventory.Config, results []moduleResult, meta report.Meta) error {
	reportErrors := writeReports(directory, meta, results, nil)
	for index, module := range config.Modules {
		done := progress.timed("machines of %s", moduleLabel(module.Path))
		checkMachines(root, directory, index, module, &results[index])
		done()
	}
	reportErrors = errors.Join(reportErrors, writeReports(directory, meta, results, nil))
	for index, module := range config.Modules {
		done := progress.timed("tests of %s", moduleLabel(module.Path))
		checkTests(root, module, &results[index])
		done()
	}
	reportErrors = errors.Join(reportErrors, writeReports(directory, meta, results, nil))
	for index, module := range config.Modules {
		done := progress.timed("laws and bites of %s", moduleLabel(module.Path))
		checkLawsDoc(root, module, &results[index])
		checkBites(root, module, &results[index])
		done()
	}
	return errors.Join(reportErrors, writeReports(directory, meta, results, nil))
}

func prepareModule(root string, module inventory.Module) moduleResult {
	result := moduleResult{Path: module.Path, Timeout: module.Timeout()}
	if err := requireModule(root, module); err != nil {
		result.recordError("module inventory", err)
		return result
	}
	doc, err := readLawDocument(root, module.Laws)
	if err != nil {
		result.recordError("law documentation", err)
		return result
	}
	result.Plans = doc.Plans
	result.SharedDocs, err = readSharedDocuments(root, module.SharedLaws)
	if err != nil {
		result.recordError("shared law documentation", err)
		return result
	}
	result.SharedLaws = sharedMachineLaws(result.SharedDocs)
	result.Ready = true
	result.Verdicts = append(result.Verdicts, verdict{Name: "module inventory", Status: passed})
	return result
}

func requireModule(root string, module inventory.Module) error {
	if module.Language == "go" {
		return requireFile(root, filepath.Join(module.Path, "go.mod"))
	}
	info, err := os.Stat(filepath.Join(root, module.Path))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("module %q is not a directory", module.Path)
	}
	actual, err := filepath.EvalSymlinks(filepath.Join(root, module.Path))
	if err != nil {
		return err
	}
	return requireInside(root, module.Path, actual)
}

func requireInside(root, name, resolved string) error {
	relative, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("%s resolves outside repository", name)
	}
	return nil
}

func requireFile(root, name string) error {
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		return err
	}
	if err := requireInside(root, name, resolved); err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", name)
	}
	return nil
}

func moduleFailed(result moduleResult) bool {
	if len(result.Machines) > 0 && report.Sum(result.Machines).Outcome().Refuses() {
		return true
	}
	return slices.ContainsFunc(result.Verdicts, func(item verdict) bool {
		return item.Status == failed || item.Status == infrastructure
	})
}
