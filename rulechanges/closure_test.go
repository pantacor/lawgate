package rulechanges

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
)

// mapTree serves fixture files from memory in sorted path order.
func mapTree(files map[string]string) sourceTree {
	read := func(name string) ([]byte, error) {
		data, ok := files[name]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(data), nil
	}
	return sourceTree{paths: sortedFixturePaths(files), read: read}
}

func classifyFixture(t *testing.T, files map[string]string) ([]string, error) {
	t.Helper()
	result := classify(mapTree(files))
	return sortedPaths(result.rules), result.err()
}

func closureSources() map[string]string {
	return map[string]string{
		"go.mod":          "module example.org/project\n",
		"LAWS.md":         "## owner-rule\n- Enforced by: test `TestOwner`",
		"owner_test.go":   ownerTestSource,
		"helper.go":       "package project\nfunc checkOwner(t any) { assertSame(t) }",
		"assert.go":       "package project\nfunc assertSame(t any) {}",
		"checker.go":      "package project\ntype checker struct{}\nfunc newChecker() *checker { return &checker{} }",
		"verify.go":       "package project\nfunc (c *checker) verify() {}",
		"domain/rules.go": "package domain\nvar Rules = ruleSet{}\ntype ruleSet struct{}",
		"domain/check.go": "package domain\nfunc (ruleSet) Check() { compare() }",
		"domain/cmp.go":   "package domain\nfunc compare() {}",
		"other/other.go":  "package other\nfunc Unrelated() {}",
	}
}

const ownerTestSource = `package project
import "example.org/project/domain"
func TestOwner(t *testing.T) {
	checkOwner(t)
	newChecker().verify()
	domain.Rules.Check()
}`

// TestEnforcingTestCalleesAreRuleFiles enforces rule-changes-are-reviewed:
// emptying a helper that an enforcing test calls is a rule change.
func TestEnforcingTestCalleesAreRuleFiles(t *testing.T) {
	actual, err := classifyFixture(t, closureSources())
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"LAWS.md", "assert.go", "checker.go", "domain/check.go", "domain/cmp.go",
		"domain/rules.go", "go.mod", "helper.go", "owner_test.go", "verify.go",
	}
	if !slices.Equal(actual, expected) {
		t.Fatalf("rule files = %v; want %v", actual, expected)
	}
}

func TestSkippedDirectoriesAndBrokenFilesDoNotAbort(t *testing.T) {
	files := closureSources()
	files["testdata/bad.go"] = "package x\nfunc {"
	files["testdata/LAWS.md"] = "## fixture-law\n- Enforced by: test `TestFixture`"
	files["vendor/example.org/dep/bad.go"] = "func {"
	files[".tools/bad.go"] = "func {"
	files["_scratch/bad.go"] = "func {"
	files["broken.go"] = "package project\nfunc {"
	files["fixtures/fixture_test.go"] = "package fixtures\nfunc TestFixture(t *testing.T) {}"
	actual, err := classifyFixture(t, files)
	if err == nil || !strings.Contains(err.Error(), "broken.go") {
		t.Fatalf("expected a reported parse problem for broken.go, got %v", err)
	}
	if strings.Contains(err.Error(), "testdata") || strings.Contains(err.Error(), "vendor") {
		t.Fatalf("skipped directories were parsed: %v", err)
	}
	if !slices.Contains(actual, "broken.go") || !slices.Contains(actual, "helper.go") {
		t.Fatalf("classification stopped or left the broken file unprotected: %v", actual)
	}
	// testdata is not read for rules, but it is a fixture of the reached
	// root package, so it is protected whole.
	if !slices.Contains(actual, "testdata/LAWS.md") || slices.Contains(actual, "fixtures/fixture_test.go") {
		t.Fatalf("testdata was read for enforcers or left unprotected: %v", actual)
	}
}

func TestScriptEnforcersResolveFromTheirLawsFile(t *testing.T) {
	files := map[string]string{
		"module/LAWS.md":          "## law\n- Enforced by: script `scripts/check.sh`, script `../tools/audit.sh`, script `../../escape.sh`",
		"module/scripts/check.sh": "",
		"tools/audit.sh":          "",
		"scripts/check.sh":        "",
	}
	actual, err := classifyFixture(t, files)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"module/LAWS.md", "module/scripts/check.sh", "tools/audit.sh"}
	if !slices.Equal(actual, expected) {
		t.Fatalf("rule files = %v; want %v", actual, expected)
	}
}

func TestNodeEnforcersFollowRelativeImports(t *testing.T) {
	files := map[string]string{
		"LAWS.md":              "## law\n- Enforced by: test `node title`",
		"node/checks.test.cjs": "const h = require('./helpers.cjs');\ntest(\"node title\", () => h.check());",
		"node/helpers.cjs":     "const { same } = require('../lib/assert');\nmodule.exports = { check: same };",
		"lib/assert.js":        "exports.same = function same() {}",
		"lib/unrelated.js":     "export function other() {}",
		"node/escape.cjs":      "",
		"node/other.test.cjs":  "require('./escape.cjs'); test(\"other\", () => {});",
	}
	actual, err := classifyFixture(t, files)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"LAWS.md", "lib/assert.js", "node/checks.test.cjs", "node/helpers.cjs"}
	if !slices.Equal(actual, expected) {
		t.Fatalf("rule files = %v; want %v", actual, expected)
	}
}

func TestRulePackagesComeFromConfiguration(t *testing.T) {
	files := map[string]string{
		"go.mod":         "module example.org/project\n",
		"worker.go":      "package project\nimport sm \"example.org/project/lib/statemachine\"\nvar worker = sm.Machine{}",
		"third/party.go": "package third\nimport other \"example.org/elsewhere/statemachine\"\nvar thing = other.Machine{}",
		"entry.go":       "package project\nimport cm \"example.org/project/lib/cmdmachine\"\nvar entry = cm.Entry{}",
	}
	tree := mapTree(files)
	vocabulary := frameworkVocabulary()
	vocabulary.add(inventory.RulePackage{Path: "example.org/project/lib/statemachine", Names: []string{"Machine"}})
	vocabulary.add(inventory.RulePackage{Path: "example.org/project/lib/cmdmachine", Names: []string{"Entry"}})
	actual := sortedPaths(classifyWith(tree, vocabulary).rules)
	expected := []string{"entry.go", "go.mod", "worker.go"}
	if !slices.Equal(actual, expected) {
		t.Fatalf("rule files = %v; want %v", actual, expected)
	}
	unconfigured := sortedPaths(classify(tree).rules)
	if len(unconfigured) != 0 {
		t.Fatalf("product package names are still built in: %v", unconfigured)
	}
}
