package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/report"
)

func checkLawsDoc(root string, module inventory.Module, result *moduleResult) {
	result.Complete = true
	if !result.Ready {
		return
	}
	input, err := lawsDocInputs(root, module, result)
	if err != nil {
		result.recordError("test discovery", err)
	}
	primary := lawDocument{Path: module.Laws, Plans: result.Plans}
	checkDocumentOverlaps(primary, result)
	checkDocument(primary, input, result)
	input.Required = nil
	input.SharedMachines = append([]string{}, result.expectedMachines()...)
	for _, doc := range result.SharedDocs {
		checkDocument(doc, input, result)
	}
}

func lawsDocInputs(root string, module inventory.Module, result *moduleResult) (lawsdoc.Inputs, error) {
	machines, required := machineLaws(result.Machines, result.SharedLaws)
	input := lawsdoc.Inputs{RepoRoot: root, DocPath: module.Laws, Machines: machines, Required: required}
	if module.Language == "go" {
		return bindGoTests(root, module, result, input)
	}
	input.CheckTest = func(_, name string) error { return result.TestRuns.require(name) }
	for _, name := range module.RequiredTests {
		recordRequiredTest(result, name, result.TestRuns.require(name))
	}
	return input, nil
}

func bindGoTests(root string, module inventory.Module, result *moduleResult, input lawsdoc.Inputs) (lawsdoc.Inputs, error) {
	tests, err := lawsdoc.DiscoverCommandTests(filepath.Join(root, module.Path), module.TestCommand, module.Timeout())
	if err != nil {
		input.CheckTest = func(_, _ string) error { return nil }
		return input, err
	}
	input.CheckTest = func(law, name string) error {
		test, err := tests.EnforcingTest(law, name)
		if err != nil {
			return err
		}
		return result.TestRuns.require(test.QualifiedName())
	}
	for _, name := range module.RequiredTests {
		test, err := tests.Resolve(name)
		if err == nil {
			err = result.TestRuns.require(test.QualifiedName())
		}
		recordRequiredTest(result, name, err)
	}
	return input, nil
}

func recordRequiredTest(result *moduleResult, name string, err error) {
	if err != nil {
		result.recordError(name, err)
	}
}

type lawDocument struct {
	Path  string
	Plans []lawsdoc.LawPlan
}

func readLawDocument(root, name string) (lawDocument, error) {
	if err := requireFile(root, name); err != nil {
		return lawDocument{}, err
	}
	doc, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return lawDocument{}, err
	}
	return lawDocument{Path: name, Plans: lawsdoc.Plan(string(doc))}, nil
}

func readSharedDocuments(root string, paths []string) ([]lawDocument, error) {
	var documents []lawDocument
	for _, path := range paths {
		doc, err := readLawDocument(root, path)
		if err != nil {
			return nil, err
		}
		documents = append(documents, doc)
	}
	return documents, nil
}

func sharedMachineLaws(documents []lawDocument) map[string]bool {
	names := make(map[string]bool)
	for _, doc := range documents {
		for _, law := range doc.Plans {
			names[law.Name] = names[law.Name] || len(law.MachineNames()) != 0
		}
	}
	return names
}

func checkDocumentOverlaps(primary lawDocument, result *moduleResult) {
	sources := make(map[string]string)
	documents := append([]lawDocument{primary}, result.SharedDocs...)
	for _, doc := range documents {
		for _, law := range doc.Plans {
			recordOverlappingLaw(result, sources, doc.Path, law.Name)
		}
	}
}

func recordOverlappingLaw(result *moduleResult, sources map[string]string, path, law string) {
	previous, documented := sources[law]
	if !documented {
		sources[law] = path
		return
	}
	if previous != path {
		detail := fmt.Sprintf("law %s has sections in both %s and %s", law, previous, path)
		result.Verdicts = append(result.Verdicts, verdict{path, failed, detail})
	}
}

func checkDocument(doc lawDocument, input lawsdoc.Inputs, result *moduleResult) {
	input.DocPath = doc.Path
	recordProblems(result, doc.Path, lawsdoc.CheckPlan(doc.Plans, input))
	recordEnforcementGaps(result, doc)
	checkScripts(input.RepoRoot, doc, result)
}

func recordEnforcementGaps(result *moduleResult, doc lawDocument) {
	for _, law := range doc.Plans {
		if !law.Agreed() {
			continue
		}
		if reason := law.EnforcementGap(); reason != "" {
			name := fmt.Sprintf("%s (%s)", law.Name, doc.Path)
			result.Verdicts = append(result.Verdicts, verdict{Name: name, Status: pending, Detail: reason})
		}
	}
}

func machineLaws(runs []report.LawRun, shared map[string]bool) (map[string][]string, map[string][]string) {
	machines := make(map[string][]string)
	required := make(map[string][]string)
	for _, run := range runs {
		for _, verdict := range run.Verdicts {
			machines[run.Machine] = append(machines[run.Machine], verdict.Law.Name)
		}
		required[run.Machine] = ownedLaws(machines[run.Machine], shared)
	}
	return machines, required
}

func ownedLaws(names []string, shared map[string]bool) []string {
	var owned []string
	for _, name := range names {
		if !shared[name] {
			owned = append(owned, name)
		}
	}
	return owned
}

func checkScripts(root string, doc lawDocument, result *moduleResult) {
	for _, law := range doc.Plans {
		for _, enforcer := range law.Enforcers {
			runScript(root, doc.Path, law.Name, enforcer, result)
		}
	}
}

func runScript(root, doc, law string, enforcer lawsdoc.Enforcer, result *moduleResult) {
	if enforcer.Kind != lawsdoc.Script {
		return
	}
	script, err := lawsdoc.ScriptPath(root, doc, enforcer.Name)
	if err != nil {
		return // The documentation check already reports the invalid path.
	}
	name := scriptVerdictName(law, enforcer.Name)
	if err := refuseScriptLink(root, doc, enforcer.Name, script); err != nil {
		result.recordError(name, err)
		return
	}
	timeout := result.Timeout
	if timeout == 0 {
		timeout = inventory.CommandTimeout
	}
	stdout, stderr, err := execute(root, ".", []string{script}, timeout)
	recordCommandVerdict(result, name, stdout, stderr, err)
}

// scriptVerdictName names a script enforcer's normal run. The bite phase
// finds that run by this name before it mutates the script.
func scriptVerdictName(law, script string) string {
	return fmt.Sprintf("%s (script %s)", law, script)
}

func refuseScriptLink(root, doc, name, resolved string) error {
	base, err := gosource.CanonicalRoot(root)
	if err != nil {
		return err
	}
	original := filepath.Join(base, filepath.Dir(doc), name)
	if original != resolved {
		return fmt.Errorf("script enforcer %q is a symlink or resolves through a symlink", name)
	}
	return nil
}

func recordCommandVerdict(result *moduleResult, name string, stdout []byte, stderr string, err error) {
	if err == nil {
		result.Verdicts = append(result.Verdicts, verdict{Name: name, Status: passed})
		return
	}
	detail := strings.TrimSpace(commandError(err, stdout, stderr).Error())
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		result.recordError(name, errors.New(detail))
		return
	}
	result.Verdicts = append(result.Verdicts, verdict{Name: name, Status: failed, Detail: detail})
}

func recordProblems(result *moduleResult, name string, problems []string) {
	if len(problems) == 0 {
		result.Verdicts = append(result.Verdicts, verdict{Name: name, Status: passed})
	}
	for _, problem := range problems {
		result.Verdicts = append(result.Verdicts, verdict{Name: name, Status: failed, Detail: problem})
	}
}
