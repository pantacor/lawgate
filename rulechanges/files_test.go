package rulechanges

import (
	"io/fs"
	"slices"
	"testing"
)

func TestRuleFilesFollowDeclarationsAndEnforcers(t *testing.T) {
	files := fixtureSources()
	tree := sourceTree{paths: sortedFixturePaths(files), read: func(name string) ([]byte, error) {
		data, ok := files[name]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return []byte(data), nil
	}}
	result := classify(tree)
	if err := result.err(); err != nil {
		t.Fatal(err)
	}
	actual := result.rules
	expected := []string{
		".github/workflows/laws.yml", "CODEOWNERS", "checks.go", "docs/LAWS.md",
		"domain/caps.go", "go.mod", "inputs.go", "lawgate.json", "node/checks.test.cjs", "revisions.go", "rules/readability.txt",
		"scripts/verify.sh", "witness_test.go", "worker.go", "worker_test.go",
	}
	if !slices.Equal(sortedPaths(actual), expected) {
		t.Fatalf("rule files = %v; want %v", sortedPaths(actual), expected)
	}
}

func fixtureSources() map[string]string {
	return map[string]string{
		"CODEOWNERS":                 "* @highercomve",
		".github/workflows/laws.yml": "",
		"go.mod":                     "module example.org/project\n",
		"lawgate.json":               `{"readability":{"baseline":"rules/readability.txt"},"modules":[{"path":".","language":"go","laws":"docs/LAWS.md","machines":true}]}`,
		"docs/LAWS.md":               "## owner-rule\n- Enforced by: test `TestOwner`, test `node title`, script `../scripts/verify.sh`",
		"scripts/verify.sh":          "",
		"node/checks.test.cjs":       `test("node title", () => {});`,
		"worker.go":                  `package test; import graph "github.com/pantacor/lawgate/machine"; var worker = graph.Machine{Edges: []graph.Edge{{Guard: "retries < TextualLimit", Limit: limit}}}`,
		"inputs.go":                  `package test; import "example.org/project/domain"; const limit = domain.Limit`,
		"domain/caps.go":             "package domain; const (Floor = iota; Limit)",
		"checks.go":                  `package test; import graph "github.com/pantacor/lawgate/machine"; var checks = []graph.Law{{Name: "owner-rule"}}`,
		"worker_test.go":             "package test; func TestOwner(t *testing.T) {}",
		"witness_test.go":            `package test; import p "github.com/pantacor/lawgate/prove"; func TestWitness(t *testing.T) { p.Check(t, decide, p.Ints(0, 8)) }`,
		"revisions.go":               "package test; func decide() int { return TextualLimit }; const TextualLimit = 2",
		"notes/justlaws.go":          "package notes; // graph.Machine is mentioned here.\nvar description = \"p.Check(...)\"",
		"mock/kernel_mock.go":        "package mock; func OrdinaryFunction() {}",
	}
}

func sortedFixturePaths(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
