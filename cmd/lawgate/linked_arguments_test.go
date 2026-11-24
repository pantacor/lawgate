package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecuteRefusesSymlinkedCommandArguments keeps execution in step with
// ownership: a configured command must not run a repository file reached
// through a symbolic link, since CODEOWNERS protects files by path.
func TestExecuteRefusesSymlinkedCommandArguments(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "real/check.sh", "#!/bin/sh\nexit 0\n")
	if err := os.Symlink("real", filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	_, _, err := execute(root, ".", []string{"sh", "linked/check.sh"})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("a symlinked command argument ran: %v", err)
	}
	if _, _, err := execute(root, ".", []string{"sh", "real/check.sh"}); err != nil {
		t.Fatalf("a plain command argument was refused: %v", err)
	}
}
