package rulechanges

import (
	"strings"
	"testing"
)

func replacementSources() map[string]string {
	files := blankImportSources()
	files["w/law_test.go"] = strings.Replace(files["w/law_test.go"], "example.org/project/reg", "example.org/alias/pkg", 1)
	files["local/go.mod"] = "module example.org/local\n"
	files["local/pkg/main.go"] = "package pkg\n"
	files["go.mod"] += "replace example.org/alias => ./local\n"
	return files
}

// TestLocalReplacementOwnership enforces rule-changes-are-reviewed for
// dependencies whose import path differs from their local module declaration.
func TestLocalReplacementOwnership(t *testing.T) {
	requireRuleFiles(t, replacementSources(), []string{"local/go.mod", "local/pkg/main.go"}, nil)
}

func TestWorkspaceImportOwnership(t *testing.T) {
	files := replacementSources()
	files["go.work"] = "go 1.25\nuse (\n .\n ./used\n)\nreplace example.org/alias => \"./work dir\" // override\n"
	files["work dir/go.mod"] = "module example.org/work\n"
	files["work dir/pkg/main.go"] = "package pkg\nimport _ \"example.org/used/sub\"\n"
	files["used/go.mod"] = "module example.org/used\n"
	files["used/sub/sub.go"] = "package sub\n"
	want := []string{"go.work", "work dir/go.mod", "work dir/pkg/main.go", "used/go.mod", "used/sub/sub.go"}
	requireRuleFiles(t, files, want, []string{"local/pkg/main.go"})
}

func TestUnreadableLocalImportFailsClosed(t *testing.T) {
	files := replacementSources()
	delete(files, "local/pkg/main.go")
	_, err := classifyFixture(t, files)
	if err == nil || !strings.Contains(err.Error(), "example.org/alias/pkg") {
		t.Fatalf("unreadable local import: %v", err)
	}
}

func TestConfiguredGoCommandUsesItsInvokingModule(t *testing.T) {
	files := replacementSources()
	files["lawgate.json"] = `{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false,"generate_check":["env","CGO_ENABLED=0","go","run","example.org/alias/check"]}]}`
	files["go.mod"] += "replace example.org/helper => ./right\n"
	files["local/go.mod"] += "replace example.org/helper => ../wrong\n"
	files["local/check/main.go"] = "package main\nimport _ \"example.org/helper\"\nfunc main() {}"
	files["right/go.mod"] = "module example.org/helper\n"
	files["right/check.go"] = "package helper\n"
	files["wrong/go.mod"] = "module example.org/helper\n"
	files["wrong/check.go"] = "package helper\n"
	want := []string{"local/check/main.go", "right/check.go"}
	requireRuleFiles(t, files, want, []string{"wrong/check.go"})
}

func TestConfiguredFreshnessScriptIsProtected(t *testing.T) {
	files := blankImportSources()
	files["lawgate.json"] = `{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false,"generate_check":["sh","scripts/check.sh"]}]}`
	files["scripts/check.sh"] = "#!/bin/sh\n"
	requireRuleFiles(t, files, []string{"scripts/check.sh"}, nil)
}

func TestAbsoluteInRepositoryReplacementOwnership(t *testing.T) {
	files := replacementSources()
	files["go.mod"] = strings.Replace(files["go.mod"], "./local", "/checkout/local", 1)
	tree := mapTree(files)
	tree.root = "/checkout"
	result := classify(tree)
	if err := result.err(); err != nil {
		t.Fatal(err)
	}
	if !result.rules["local/pkg/main.go"] {
		t.Fatal("an absolute in-repository replacement escaped ownership")
	}
}
