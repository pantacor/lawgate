package rulechanges

import (
	"errors"
	"slices"
	"testing"
)

func blankImportSources() map[string]string {
	return map[string]string{
		"go.mod":         "module example.org/project\n",
		"LAWS.md":        "## a-law\n- Enforced by: test `TestALaw`",
		"w/law_test.go":  "package w\nimport _ \"example.org/project/reg\"\nfunc TestALaw(t *testing.T) {}",
		"reg/reg.go":     "package reg\nimport _ \"example.org/project/deep\"\nfunc init() { register() }\nfunc register() {}",
		"deep/deep.go":   "package deep\nfunc init() {}",
		"unrelated/u.go": "package unrelated\nfunc U() {}",
	}
}

// TestImportedPackagesAreReachedTransitively enforces rule-changes-are-reviewed:
// a package an enforcing package imports only for its side effects (a blank
// import running init) changes what the law test sees, so it is protected,
// and so is everything it imports in turn.
func TestImportedPackagesAreReachedTransitively(t *testing.T) {
	wanted := []string{"w/law_test.go", "reg/reg.go", "deep/deep.go"}
	unwanted := []string{"unrelated/u.go"}
	requireRuleFiles(t, blankImportSources(), wanted, unwanted)
}

const quotedEmbedSource = "package w\nimport _ \"embed\"\n//go:embed \"case one.json\" `case two.json`\nvar cases string"

func quotedEmbedSources() map[string]string {
	return map[string]string{
		"go.mod":          "module example.org/project\n",
		"LAWS.md":         "## a-law\n- Enforced by: test `TestALaw`",
		"w/law_test.go":   "package w\nfunc TestALaw(t *testing.T) { _ = cases }",
		"w/w.go":          quotedEmbedSource,
		"w/case one.json": "{}",
		"w/case two.json": "{}",
		"w/case":          "",
		"w/one.json":      "",
	}
}

// TestQuotedEmbedPatternsAreProtected enforces rule-changes-are-reviewed:
// Go accepts quoted //go:embed names holding spaces; the files they name are
// inputs of the law test like any other embedded file.
func TestQuotedEmbedPatternsAreProtected(t *testing.T) {
	wanted := []string{"w/case one.json", "w/case two.json"}
	unwanted := []string{"w/case", "w/one.json"}
	requireRuleFiles(t, quotedEmbedSources(), wanted, unwanted)
}

func badEmbedSources() map[string]string {
	return map[string]string{
		"go.mod":        "module example.org/project\n",
		"LAWS.md":       "## a-law\n- Enforced by: test `TestALaw`",
		"w/law_test.go": "package w\nfunc TestALaw(t *testing.T) {}",
		"w/w.go":        "package w\nimport _ \"embed\"\n//go:embed \"unterminated\nvar data string",
		"w/data/a.txt":  "",
		"other/o.txt":   "",
	}
}

// TestUnreadableEmbedProtectsThePackageTree enforces rule-changes-are-reviewed:
// an embed directive the classifier cannot evaluate fails closed. Every file
// under the package directory is protected and the problem is reported.
func TestUnreadableEmbedProtectsThePackageTree(t *testing.T) {
	result := classify(mapTree(badEmbedSources()))
	rules := sortedPaths(result.rules)
	if !slices.Contains(rules, "w/data/a.txt") {
		t.Errorf("w/data/a.txt is not a rule file; rule files = %v", rules)
	}
	if slices.Contains(rules, "other/o.txt") {
		t.Errorf("other/o.txt became a rule file; rule files = %v", rules)
	}
	if !errors.Is(result.err(), errUnparsable) {
		t.Errorf("problems = %v, want an unparsable embed reported", result.err())
	}
}

const linuxVariant = "//go:build linux\n\npackage w\nimport \"example.org/project/hl\"\nfunc TestALaw(t *testing.T) { hl.Check() }"
const otherVariant = "//go:build !linux\n\npackage w\nimport \"example.org/project/ho\"\nfunc TestALaw(t *testing.T) { ho.Check() }"

func buildVariantSources() map[string]string {
	return map[string]string{
		"go.mod":              "module example.org/project\n",
		"LAWS.md":             "## a-law\n- Enforced by: test `example.org/project/w/TestALaw`",
		"w/law_linux_test.go": linuxVariant,
		"w/law_other_test.go": otherVariant,
		"hl/hl.go":            "package hl\nfunc Check() {}",
		"ho/ho.go":            "package ho\nfunc Check() {}",
		"v/law_test.go":       "package v\nfunc TestALaw(t *testing.T) {}",
	}
}

// TestQualifiedReferenceCoversEveryBuildVariant enforces
// rule-changes-are-reviewed: mutually exclusive build variants each compile
// one test of the qualified name, so ownership protects all of them; only
// execution needs the one active test.
func TestQualifiedReferenceCoversEveryBuildVariant(t *testing.T) {
	wanted := []string{"w/law_linux_test.go", "w/law_other_test.go", "hl/hl.go", "ho/ho.go"}
	unwanted := []string{"v/law_test.go"}
	requireRuleFiles(t, buildVariantSources(), wanted, unwanted)
}

func secretNamedSources() map[string]string {
	return map[string]string{
		"go.mod":                "module example.org/project\n",
		"LAWS.md":               "## a-law\n- Enforced by: test `TestALaw`",
		"w/secrets_test.go":     "package w\nimport \"example.org/project/secrets\"\nfunc TestALaw(t *testing.T) { secrets.Check() }",
		"secrets/check.go":      "package secrets\nfunc Check() {}",
		"loose/secret_notes.go": "package loose\nfunc Notes() {}",
		"w/.env":                "TOKEN=x",
	}
}

// TestSecretNamedGoSourceIsRead enforces rule-changes-are-reviewed: Go
// source is read whatever its name (one walking rule, gosource), so an
// enforcing test in secrets_test.go brings in the secrets package it calls,
// and a secret-named file nothing reaches is not protected by its name.
func TestSecretNamedGoSourceIsRead(t *testing.T) {
	wanted := []string{"w/secrets_test.go", "secrets/check.go"}
	unwanted := []string{"loose/secret_notes.go", "w/.env"}
	requireRuleFiles(t, secretNamedSources(), wanted, unwanted)
}
