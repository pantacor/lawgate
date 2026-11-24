package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

func TestCheckThroughRepositorySymlink(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeGoLawModule(t, root, module)
	writeConfig(t, root, []inventory.Module{module})
	link := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(root, link); err != nil {
		t.Skip(err)
	}
	if _, err := runCheck(t, link); err != nil {
		t.Fatal(err)
	}
}

func TestScriptRefusesSymlinkBeforeExecution(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "script.sh", "#!/bin/sh\ntouch ran\n")
	if err := os.Chmod(filepath.Join(root, "script.sh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("script.sh", filepath.Join(root, "alias.sh")); err != nil {
		t.Skip(err)
	}
	enforcer := lawsdoc.Enforcer{Kind: lawsdoc.Script, Name: "alias.sh"}
	result := moduleResult{}
	runScript(root, "LAWS.md", "my-law", enforcer, &result)
	if _, err := os.Stat(filepath.Join(root, "ran")); !os.IsNotExist(err) {
		t.Fatal("symlink target ran")
	}
	item := result.Verdicts[0]
	if item.Status != infrastructure {
		t.Fatalf("verdict = %+v", item)
	}
}

func TestCheckWritesReproductionMetadata(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	writeGoLawModule(t, root, module)
	writeConfig(t, root, []inventory.Module{module})
	bin := filepath.Join(root, "bin")
	mockGit := filepath.Join(bin, "git")
	writeFixture(t, root, "bin/git", "#!/bin/sh\nprintf 'abc123\\n'\n")
	if err := os.Chmod(mockGit, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	directory, err := runCheck(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, directory, "laws-report.html", "lawgate check", " · generated ", "abc123")
}

func TestCheckEnforcesModuleTimeout(t *testing.T) {
	root := t.TempDir()
	module := fixtureModule(".", withoutMachines)
	module.Language = "node"
	module.TestCommand = []string{"sh", "-c", "sleep 3"}
	oneSecond := 1
	module.TimeoutSeconds = &oneSecond
	writeConfig(t, root, []inventory.Module{module})
	writeFixture(t, root, module.Laws, testLaw)
	directory, err := runCheck(t, root)
	if err == nil {
		t.Fatal("check accepted a test command past its deadline")
	}
	assertReport(t, directory, "laws-summary.md", "command timed out after 1s")
}
