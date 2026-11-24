package rulechanges

import (
	"strings"
	"testing"
)

const vendoredKernel = "github.com/pantacor/lawgate/machine"

func vendoredSources() map[string]string {
	files := blankImportSources()
	files["w/law_test.go"] = "package w\nimport _ \"" + vendoredKernel + "\"\nfunc TestALaw(t *testing.T) {}"
	files["vendor/modules.txt"] = "# github.com/pantacor/lawgate v0.1.0\n## explicit\n" + vendoredKernel + "\n"
	files["vendor/"+vendoredKernel+"/laws.go"] = "package machine\nimport _ \"github.com/pantacor/lawgate/resolve\"\n"
	files["vendor/"+vendoredKernel+"/laws_amd64.s"] = "TEXT ·f(SB),0,$0\n"
	files["vendor/"+vendoredKernel+"/README.md"] = "docs\n"
	files["vendor/github.com/pantacor/lawgate/resolve/resolve.go"] = "package resolve\n"
	files["vendor/github.com/other/unused/bad.go"] = "func {"
	return files
}

// TestVendoredPackagesAreOwned enforces rule-changes-are-reviewed: Go builds
// a module that has vendor/modules.txt from vendor/, so a vendored package
// a law test imports, and what it imports in turn, is a rule file.
func TestVendoredPackagesAreOwned(t *testing.T) {
	wanted := []string{
		"vendor/modules.txt",
		"vendor/" + vendoredKernel + "/laws.go",
		"vendor/" + vendoredKernel + "/laws_amd64.s",
		"vendor/github.com/pantacor/lawgate/resolve/resolve.go",
	}
	unwanted := []string{"vendor/github.com/other/unused/bad.go", "vendor/" + vendoredKernel + "/README.md"}
	requireRuleFiles(t, vendoredSources(), wanted, unwanted)
}

func TestVendoredLocalReplacementIsOwned(t *testing.T) {
	files := replacementSources()
	// Go vendors by default only when go.mod says go 1.14 or later.
	files["go.mod"] = strings.Replace(files["go.mod"], "./local", "../outside", 1) + "go 1.25\n"
	files["vendor/modules.txt"] = "# example.org/alias v0.0.0 => ../outside\n## explicit\nexample.org/alias/pkg\n"
	files["vendor/example.org/alias/pkg/main.go"] = "package pkg\n"
	requireRuleFiles(t, files, []string{"vendor/example.org/alias/pkg/main.go", "vendor/modules.txt"}, nil)
}

func TestWorkspaceVendorIsOwned(t *testing.T) {
	files := vendoredSources()
	files["go.work"] = "go 1.25\nuse .\n"
	files["vendor/modules.txt"] = "## workspace\n" + files["vendor/modules.txt"]
	requireRuleFiles(t, files, []string{"vendor/" + vendoredKernel + "/laws.go"}, nil)
}

func TestUnderscoreDirectoryImportIsRead(t *testing.T) {
	files := blankImportSources()
	files["deep/deep.go"] = "package deep\nimport _ \"example.org/project/_impl\"\n"
	files["_impl/impl.go"] = "package impl\n"
	requireRuleFiles(t, files, []string{"_impl/impl.go"}, nil)
}

func TestOutsideReplacementIsNamed(t *testing.T) {
	files := replacementSources()
	files["go.mod"] = strings.Replace(files["go.mod"], "./local", "../outside", 1)
	_, err := classifyFixture(t, files)
	if err == nil || !strings.Contains(err.Error(), "outside the repository: lawgate cannot protect it") {
		t.Fatalf("out-of-repository replace: %v", err)
	}
}

// TestOneCharacterNestedModule: "a" and "." have the same length, but "a"
// is the inner module.
func TestOneCharacterNestedModule(t *testing.T) {
	files := map[string]string{
		"go.mod":        "module example.org/root\n",
		"a/go.mod":      "module example.org/a\n",
		"a/LAWS.md":     "## a-law\n- Enforced by: test `TestALaw`",
		"a/law_test.go": "package a\nimport _ \"example.org/a/helper\"\nfunc TestALaw(t *testing.T) {}",
		"a/helper/h.go": "package helper\n",
		"root/r.go":     "package root\n",
	}
	for range 30 {
		requireRuleFiles(t, files, []string{"a/helper/h.go", "a/go.mod"}, []string{"root/r.go"})
	}
}

func TestModuleOutsideParentWorkspace(t *testing.T) {
	files := map[string]string{
		"go.work":       "go 1.25\nuse ./a\n",
		"a/go.mod":      "module example.org/a\n",
		"b/go.mod":      "module example.org/b\n",
		"b/LAWS.md":     "## b-law\n- Enforced by: test `TestBLaw`",
		"b/law_test.go": "package b\nimport _ \"example.org/b/helper\"\nfunc TestBLaw(t *testing.T) {}",
		"b/helper/h.go": "package helper\n",
	}
	requireRuleFiles(t, files, []string{"b/helper/h.go"}, nil)
}

func nodeCommandSources() map[string]string {
	return map[string]string{
		"lawgate.json":                   `{"modules":[{"path":"web","language":"node","laws":"web/LAWS.md","machines":true,"machine_command":["node","scripts/check-machines.mjs"]}]}`,
		"web/package.json":               "{}",
		"web/LAWS.md":                    "## web-law\n- Enforced by: script `scripts/check-law.mjs`",
		"web/scripts/check-machines.mjs": "import { laws } from './machine-laws.mjs'\n",
		"web/scripts/machine-laws.mjs":   "export const laws = []\n",
		"web/scripts/check-law.mjs":      "const help = require('./law-helper')\n",
		"web/scripts/law-helper.js":      "module.exports = {}\n",
		"web/scripts/unrelated.mjs":      "export {}\n",
	}
}

// TestNodeCommandAndScriptImportsAreOwned: a Node command or script
// enforcer is followed through its relative imports like a Node law test.
func TestNodeCommandAndScriptImportsAreOwned(t *testing.T) {
	wanted := []string{"web/scripts/machine-laws.mjs", "web/scripts/law-helper.js"}
	requireRuleFiles(t, nodeCommandSources(), wanted, []string{"web/scripts/unrelated.mjs"})
}

func TestSymlinkedScriptEnforcerIsRefused(t *testing.T) {
	for _, link := range []string{"module/scripts/check.sh", "module/scripts"} {
		files := map[string]string{
			"module/LAWS.md":          "## law\n- Enforced by: script `scripts/check.sh`",
			"module/scripts/check.sh": "",
		}
		tree := mapTree(files)
		tree.links = map[string]bool{link: true}
		result := classify(tree)
		err := result.err()
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("symlinked script enforcer via %s: %v", link, err)
		}
		if !result.rules["module/scripts/check.sh"] {
			t.Fatalf("symlinked script enforcer via %s is unprotected", link)
		}
	}
}
