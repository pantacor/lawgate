package main

import (
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

func TestDocumentedGapsStayPendingInReports(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language = "node"
	module.TestCommand = []string{"sh", "-c", nodeTapFixture}
	writeConfig(t, root, []inventory.Module{module})
	doc := testLaw + "\n### missing-law\n- Source: owner decision 2026-11-20\n- Enforced by: not yet. Needs a fixture.\n- Status: agreed 2026-11-20\n"
	writeFixture(t, root, "LAWS.md", doc)
	directory, err := runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, directory, "laws-summary.md", "2 pending", "bite gap: The fixture", "Needs a fixture.")
	assertReport(t, directory, "laws-report.html", "2 pending", "bite gap: The fixture", "Needs a fixture.")
	assertReport(t, directory, "001.xml", `skipped="2"`, `message="pending: bite gap: The fixture`, `message="pending: Needs a fixture."`)
}

func TestScriptBiteDoesNotRequireEnvOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	writeExecutable(t, root, "rule.sh", "#!/bin/sh\n"+rejectScriptLaw("script-law"))
	result := scriptBite(t, root, "rule.sh")
	expectBite(t, result, "script-law", passed, "")
}

func TestBiteRejectsAbnormalTestExit(t *testing.T) {
	assertAbnormalTestExit(t, "exit 126", "exit status 126")
	assertAbnormalTestExit(t, "exit 127", "exit status 127")
	assertAbnormalTestExit(t, "kill -TERM $$", "signal: terminated")
}

func assertAbnormalTestExit(t *testing.T, exit, detail string) {
	t.Helper()
	root := t.TempDir()
	script := "if [ \"$LAWGATE_BITE\" = go-law ]; then " + exit + "; fi\n"
	script += "printf '%s\\n' 'TAP version 13' 'ok 1 - Example' '1..1'\n"
	command := []string{"sh", "-c", script}
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md", TestCommand: command}
	result := runTestBite(root, module, biteLaw("go-law", "test", "Example"))
	expectBite(t, result, "go-law", infrastructure, detail)
}

func runTestBite(root string, module inventory.Module, doc string) moduleResult {
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(doc)}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	return result
}
