package main

import (
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
)

// TestRepositoryCheckReportsOwnershipNotes: an accepted input the owner
// cannot protect (here a system header of a law-reached cgo package) must
// still reach the summary as a note, never be accepted silently.
func TestRepositoryCheckReportsOwnershipNotes(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/project\ngo 1.25\n")
	writeFixture(t, root, "LAWS.md", "### fixture-law\n- Enforced by: test `TestFixture`\n")
	writeFixture(t, root, "cgo/fixture.go", "package cgo\n// #include <stdio.h>\nimport \"C\"\n")
	writeFixture(t, root, "cgo/law_test.go", "package cgo\nimport \"testing\"\nfunc TestFixture(t *testing.T) {}\n")
	module := fixtureModule(".", withoutMachines)
	module.Laws = "LAWS.md"
	config := inventory.Config{Modules: []inventory.Module{module}, Owner: "@owner", Platform: "github"}
	writeGateConfig(t, root, config)
	gitIn(t, root, "init", "-q")
	_, notes := checkRepository(root, config)
	if !strings.Contains(strings.Join(notes, "\n"), "<stdio.h>") {
		t.Fatalf("the system header note did not reach the summary: %q", notes)
	}
}
