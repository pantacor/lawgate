package rulechanges

import (
	"slices"
	"testing"
)

// requireRuleFiles fails unless every wanted path is a rule file and no
// unwanted path is.
func requireRuleFiles(t *testing.T, files map[string]string, wanted, unwanted []string) {
	t.Helper()
	actual, err := classifyFixture(t, files)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range wanted {
		if !slices.Contains(actual, name) {
			t.Errorf("%s is not a rule file; rule files = %v", name, actual)
		}
	}
	for _, name := range unwanted {
		if slices.Contains(actual, name) {
			t.Errorf("%s became a rule file; rule files = %v", name, actual)
		}
	}
}

func qualifiedReferenceSources() map[string]string {
	return map[string]string{
		"go.mod":           "module example.org/project\n",
		"LAWS.md":          "## a-law\n- Enforced by: test `example.org/project/w/TestALaw`",
		"w/law_test.go":    "package w\nimport \"example.org/project/h\"\nfunc TestALaw(t *testing.T) { h.Check(t) }",
		"h/h.go":           "package h\nfunc Check(t any) {}",
		"v/law_test.go":    "package v\nfunc TestALaw(t *testing.T) {}",
		"other/other.go":   "package other\nfunc Unrelated() {}",
		"other/go_test.go": "package other\nfunc TestOther(t *testing.T) {}",
	}
}

// TestQualifiedTestReferenceIsAnEnforcer enforces rule-changes-are-reviewed:
// the qualified form lawsdoc asks for (import path plus test name) seeds the
// enforcing test and what it reaches, and only that package's test.
func TestQualifiedTestReferenceIsAnEnforcer(t *testing.T) {
	wanted := []string{"w/law_test.go", "h/h.go"}
	unwanted := []string{"v/law_test.go", "other/other.go"}
	requireRuleFiles(t, qualifiedReferenceSources(), wanted, unwanted)
}

func wholePackageSources() map[string]string {
	return map[string]string{
		"go.mod":               "module example.org/project\n",
		"go.sum":               "",
		"LAWS.md":              "## a-law\n- Enforced by: test `TestALaw`",
		"w/law_test.go":        "package w\nfunc TestALaw(t *testing.T) { for range extraCases {} }",
		"w/init_test.go":       "package w\nimport \"example.org/project/extra\"\nvar extraCases []int\nfunc init() { extraCases = extra.Cases() }",
		"w/main_test.go":       "package w\nfunc TestMain(m *testing.M) { strict = true }",
		"w/w.go":               "package w\nimport _ \"embed\"\n//go:embed fixtures/*.json\nvar fixtures string\nvar strict bool",
		"w/fixtures/a.json":    "{}",
		"w/fixtures/notes.txt": "",
		"w/testdata/cases.txt": "",
		"extra/extra.go":       "package extra\nfunc Cases() []int { return nil }",
		"unrelated/u.go":       "package unrelated\nfunc U() {}",
	}
}

// TestReachedPackagesAreProtectedWhole enforces rule-changes-are-reviewed:
// init, TestMain, fixtures, embedded files and the module manifest of a
// package the closure reaches can make its law test pass vacuously.
func TestReachedPackagesAreProtectedWhole(t *testing.T) {
	wanted := []string{
		"go.mod", "go.sum", "w/law_test.go", "w/init_test.go", "w/main_test.go",
		"w/w.go", "w/fixtures/a.json", "w/testdata/cases.txt", "extra/extra.go",
	}
	unwanted := []string{"w/fixtures/notes.txt", "unrelated/u.go"}
	requireRuleFiles(t, wholePackageSources(), wanted, unwanted)
}

const hiddenLawsConfig = `{"modules":[{"path":".","language":"go","laws":"_rules/LAWS.md","machines":false,"shared_laws":[".shared/LAWS.md"]}]}`

func configuredDocumentSources() map[string]string {
	return map[string]string{
		"go.mod":          "module example.org/project\n",
		"lawgate.json":    hiddenLawsConfig,
		"_rules/LAWS.md":  "## a-law\n- Enforced by: test `TestOwn`",
		".shared/LAWS.md": "## shared-law\n- Enforced by: test `TestShared`",
		"own/own_test.go": "package own\nfunc TestOwn(t *testing.T) {}",
		"sh/sh_test.go":   "package sh\nfunc TestShared(t *testing.T) {}",
		"no/no_test.go":   "package no\nfunc TestNobody(t *testing.T) {}",
	}
}

// TestConfiguredLawDocumentsSeedWherever enforces rule-changes-are-reviewed:
// a law document lawgate.json names supplies enforcers even in a directory
// the classifier otherwise skips.
func TestConfiguredLawDocumentsSeedWherever(t *testing.T) {
	wanted := []string{"_rules/LAWS.md", ".shared/LAWS.md", "own/own_test.go", "sh/sh_test.go"}
	unwanted := []string{"no/no_test.go"}
	requireRuleFiles(t, configuredDocumentSources(), wanted, unwanted)
}

const runnerMainSource = `package main
import (
	"example.org/project/example"
	"github.com/pantacor/lawgate/runner"
)
func main() { runner.Main(worker.Entries()) }`

func runnerSources() map[string]string {
	return map[string]string{
		"go.mod":              "module example.org/project\n",
		"cmd/machine/main.go": runnerMainSource,
		"example/entries.go":  "package worker\nfunc Entries() []E { return nil }\ntype E struct{}",
		"vocab/vocab.go":      "package vocab\nimport \"github.com/pantacor/lawgate/resolve\"\nvar V = resolve.Vocabulary{}",
		"entry/entry.go":      "package entry\nimport \"github.com/pantacor/lawgate/runner\"\nvar E = runner.Entry{}",
		"unrelated/u.go":      "package unrelated\nfunc U() {}",
	}
}

// TestRunnerAndVocabularyPackagesAreSeeded enforces rule-changes-are-reviewed:
// a machine runner, its entries and the resolve vocabulary are rule code even
// in their own files, and an import whose package name differs from its
// directory is still followed.
func TestRunnerAndVocabularyPackagesAreSeeded(t *testing.T) {
	wanted := []string{"cmd/machine/main.go", "example/entries.go", "vocab/vocab.go", "entry/entry.go"}
	unwanted := []string{"unrelated/u.go"}
	requireRuleFiles(t, runnerSources(), wanted, unwanted)
}
