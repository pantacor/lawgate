package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/pantacor/lawgate/gen"
	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

const freshnessLaw = "generated-is-fresh"

func checkGeneration(root string, config inventory.Config, results []moduleResult, goModules []string) []string {
	files, scanErr := gosource.GeneratedFiles(root)
	owners := generationOwners(config, goModules)
	generated := make(map[int][]string)
	var problems []string
	if scanErr != nil {
		problems = append(problems, scanErr.Error())
	}
	for _, file := range files {
		owner := generationOwner(owners, file)
		if owner < 0 {
			problems = append(problems, fmt.Sprintf("%s belongs to no configured module with generate_check", file))
			continue
		}
		generated[owner] = append(generated[owner], file)
	}
	for i, module := range config.Modules {
		phase := generationCheck{root: root, module: module, owners: owners, index: i, result: &results[i]}
		phase.run(generated[i])
	}
	return problems
}

func generationOwners(config inventory.Config, goModules []string) map[string]int {
	owners := make(map[string]int)
	for _, dir := range goModules {
		owners[dir] = -1
	}
	for i, module := range config.Modules {
		owners[module.Path] = i
	}
	return owners
}

func generationOwner(owners map[string]int, file string) int {
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		if owner, ok := owners[filepath.ToSlash(dir)]; ok {
			return owner
		}
		if dir == "." {
			return -1
		}
	}
}

type generationCheck struct {
	root   string
	module inventory.Module
	owners map[string]int
	index  int
	result *moduleResult
}

func (phase generationCheck) run(generated []string) {
	if len(phase.module.GenerateCheck) == 0 {
		if len(generated) != 0 {
			phase.fail("module must declare generate_check for " + strings.Join(generated, ", "))
		}
		return
	}
	if !phase.result.Ready {
		return
	}
	stdout, stderr, err := execute(phase.root, phase.module.Path, phase.module.GenerateCheck, phase.module.Timeout())
	if err != nil {
		recordCommandVerdict(phase.result, freshnessLaw+" command", stdout, stderr, err)
	}
	targets := phase.recordTargets(stdout)
	if len(targets) == 0 {
		phase.fail("generate_check reported no targets")
	}
	for _, file := range generated {
		if !targets[file] {
			phase.fail("no target covers " + file)
		}
	}
}

func (phase generationCheck) fail(detail string) {
	phase.result.Verdicts = append(phase.result.Verdicts, verdict{freshnessLaw, failed, detail})
}

func (phase generationCheck) recordTargets(output []byte) map[string]bool {
	targets := make(map[string]bool)
	for line := range bytes.Lines(output) {
		record := strings.TrimRight(string(line), "\r\n")
		if record != gen.FreshnessRecord && !strings.HasPrefix(record, gen.FreshnessRecord+"\t") {
			continue
		}
		if err := phase.recordTarget(record, targets); err != nil {
			phase.fail(err.Error())
		}
	}
	return targets
}

func (phase generationCheck) recordTarget(record string, targets map[string]bool) error {
	parts := strings.Split(record, "\t")
	if len(parts) != 3 {
		return fmt.Errorf("invalid generate_check evidence %q", record)
	}
	status, target := parts[1], parts[2]
	file := path.Join(phase.module.Path, target)
	if !validGenerationPath(target) || generationOwner(phase.owners, file) != phase.index {
		return fmt.Errorf("generate_check target %q is outside the module", target)
	}
	if targets[file] {
		return fmt.Errorf("generate_check reported target %q more than once", target)
	}
	switch status {
	case gen.Fresh, gen.Stale, gen.Missing:
		targets[file] = true
	default:
		return fmt.Errorf("generate_check target %q has invalid status %q", target, status)
	}
	phase.recordStatus(file, status)
	return nil
}

func validGenerationPath(target string) bool {
	localFile := fs.ValidPath(target) && target != "."
	return localFile && !strings.ContainsAny(target, "\\\r\n")
}

func (phase generationCheck) recordStatus(file, status string) {
	name := fmt.Sprintf("%s (%s: %s)", freshnessLaw, file, status)
	item := verdict{Name: name, Status: passed}
	switch status {
	case gen.Fresh:
		if err := requireFile(phase.root, file); err != nil {
			item.Status, item.Detail = failed, err.Error()
		}
	case gen.Stale, gen.Missing:
		item.Status, item.Detail = failed, file+" is "+status
	}
	phase.result.Verdicts = append(phase.result.Verdicts, item)
}
