package gen

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCheckReportsStaleGeneratedFiles enforces generated-is-fresh by
// checking fresh, edited, and missing outputs against the same declaration.
func TestCheckReportsStaleGeneratedFiles(t *testing.T) {
	t.Run("missing machine", checkMissingMachine)
	dir := t.TempDir()
	m, opts := genMachine(), options("start", "work")
	content := generate(t, m, opts)
	fresh := Target{Path: filepath.Join(dir, "fresh.go"), Machine: m, Options: opts}
	edited := Target{Path: filepath.Join(dir, "edited.go"), Machine: m, Options: opts}
	missing := Target{Path: filepath.Join(dir, "missing.go"), Machine: m, Options: opts}
	writeFile(t, fresh.Path, content)
	writeFile(t, edited.Path, content+"// hand edit\n")
	stale, err := Check(fresh, edited, missing)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{edited.Path, missing.Path}
	if !slices.Equal(stale, want) {
		t.Fatalf("stale paths = %v, want %v", stale, want)
	}
	m.States[1].Edges[0].Limit++
	stale, err = Check(fresh)
	if err != nil || !slices.Equal(stale, []string{fresh.Path}) {
		t.Fatalf("changing the declaration made %v stale, error %v", stale, err)
	}
}

func checkMissingMachine(t *testing.T) {
	target := Target{Path: "states.go"}
	_, err := Check(target)
	if err == nil || !strings.Contains(err.Error(), `gen check "states.go": no machine`) {
		t.Fatalf("target without a machine should report a missing declaration, got %v", err)
	}
}
