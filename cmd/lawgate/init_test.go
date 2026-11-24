package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/rulechanges"
)

func TestInitWritesPlatformStarterFiles(t *testing.T) {
	t.Run("github", testGitHubStarter)
	t.Run("gitlab", testGitLabStarter)
}

func testGitHubStarter(t *testing.T) {
	root := initFixture(t, "github")
	assertNoFallback(t, root, ".github/CODEOWNERS")
	ci := ".github/workflows/lawgate.yml"
	assertReport(t, root, ci, "actions/setup-go", "check-codeowners")
	assertStarterOwners(t, root, "github", ".github/CODEOWNERS", ci)
}

func testGitLabStarter(t *testing.T) {
	root := initFixture(t, "gitlab")
	ci := ".gitlab-ci.yml"
	assertReport(t, root, ci, "extends: .lawgate", "check-codeowners", "--platform gitlab")
	assertStarterOwners(t, root, "gitlab", "CODEOWNERS", ci)
}

func initFixture(t *testing.T, platform string, extra ...string) string {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/root\n\ngo 1.25\n")
	writeFixture(t, root, "worker/go.mod", "module example.test/worker\n\ngo 1.25\n")
	writeFixture(t, root, "worker/LAWS.md", testLaw)
	writeFixture(t, root, "worker/check_test.go", lawTestSource)
	args := append([]string{"init", "--root", root, "--platform", platform, "--owner", "@owner"}, extra...)
	if err := run(args, io.Discard); err != nil {
		t.Fatal(err)
	}
	config, err := inventory.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Owner != "@owner" || config.Platform != platform {
		t.Fatalf("unexpected owner or platform: %+v", config)
	}
	if len(config.Modules) != 2 {
		t.Fatalf("expected two discovered modules, got %+v", config.Modules)
	}
	assertStarterModule(t, config.Modules[0], ".", "LAWS.md")
	assertStarterModule(t, config.Modules[1], "worker", "worker/LAWS.md")
	data, err := os.ReadFile(filepath.Join(root, config.Readability.Baseline))
	if err != nil || len(data) != 0 {
		t.Fatalf("baseline = %q, error = %v", data, err)
	}
	return root
}

func assertStarterModule(t *testing.T, module inventory.Module, directory, laws string) {
	t.Helper()
	locationMatches := module.Path == directory && module.Laws == laws
	if !locationMatches || *module.Machines {
		t.Fatalf("unexpected starter module: %+v", module)
	}
}

func assertStarterOwners(t *testing.T, root, platform, owners, ci string) {
	t.Helper()
	rules := []string{inventory.Filename, inventory.DefaultBaseline, "worker/LAWS.md", "worker/check_test.go", owners, ci}
	if err := rulechanges.CheckOwners(root, rules, "@owner", platform); err != nil {
		t.Fatal(err)
	}
}

func TestInitRefusesOverwriteBeforeWritingOtherFiles(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/root\n\ngo 1.25\n")
	const ci = ".github/workflows/lawgate.yml"
	writeFixture(t, root, ci, "existing workflow")
	err := run([]string{"init", "--root", root, "--owner", "@owner"}, io.Discard)
	if err == nil {
		t.Fatal("init accepted an existing destination")
	}
	assertReport(t, root, ci, "existing workflow")
	if _, err := os.Stat(filepath.Join(root, inventory.Filename)); !os.IsNotExist(err) {
		t.Fatalf("init left a config after refusing the workflow: %v", err)
	}
}

func assertNoFallback(t *testing.T, root, owners string) {
	t.Helper()
	err := rulechanges.CheckOwners(root, []string{"unrelated/new_rule.go"}, "@owner", "github")
	if err == nil {
		t.Fatalf("%s covers every file although --fallback was not given", owners)
	}
}

func TestInitFallbackCoversEveryFile(t *testing.T) {
	for _, platform := range []string{"github", "gitlab"} {
		root := initFixture(t, platform, "--fallback")
		err := rulechanges.CheckOwners(root, []string{"unrelated/new_rule.go"}, "@owner", platform)
		if err != nil {
			t.Fatalf("%s: fallback does not cover a new file: %v", platform, err)
		}
	}
}

func TestInitRequiresAnOwner(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/root\n\ngo 1.25\n")
	err := run([]string{"init", "--root", root}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "CODEOWNER") {
		t.Fatalf("init without --owner = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, inventory.Filename)); !os.IsNotExist(err) {
		t.Fatalf("init wrote files without an owner: %v", err)
	}
}
