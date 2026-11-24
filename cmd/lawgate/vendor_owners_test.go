package main

import (
	"io"
	"strings"
	"testing"
)

const vendoredLaws = "vendor/github.com/pantacor/lawgate/machine/laws.go"

const vendoredLawTest = `package project
import (
	"testing"
	_ "github.com/pantacor/lawgate/machine"
)
func TestLaw(t *testing.T) {}
`

// TestCheckCodeownersFlagsAPatchedVendoredKernel enforces
// rule-changes-are-reviewed: Go builds a vendoring module from
// vendor/, so a change that patches the vendored kernel and breaks a law
// must fail check-codeowners when CODEOWNERS leaves the vendored file out.
func TestCheckCodeownersFlagsAPatchedVendoredKernel(t *testing.T) {
	root := ownersRepository(t)
	writeFixture(t, root, "go.mod", "module example.org/project\n\ngo 1.25\n\nrequire github.com/pantacor/lawgate v0.1.0\n")
	writeFixture(t, root, "law_test.go", vendoredLawTest)
	writeFixture(t, root, "vendor/modules.txt", "# github.com/pantacor/lawgate v0.1.0\n## explicit\ngithub.com/pantacor/lawgate/machine\n")
	writeFixture(t, root, vendoredLaws, "package machine\nfunc check() []string { return []string{\"violation\"} }\n")
	owners := "/LAWS.md @org/law-owners\n/law_test.go @org/law-owners\n/go.mod @org/law-owners\n/lawgate.json @org/law-owners\n/.github/CODEOWNERS @org/law-owners\n"
	writeFixture(t, root, ".github/CODEOWNERS", owners)
	base := commitAll(t, root)
	writeFixture(t, root, vendoredLaws, "package machine\nfunc check() []string { return nil }\n")
	writeFixture(t, root, "LAWS.md", testLaw+"\n")
	commitAll(t, root)
	err := run([]string{"check-codeowners", "--root", root, "--base", base}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), vendoredLaws) {
		t.Fatalf("check-codeowners passed a patched vendored kernel: %v", err)
	}
}
