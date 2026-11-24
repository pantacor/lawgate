package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCheckModeRefusesStaleOutputWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	command := buildGenerator(t, dir)
	path := filepath.Join(dir, "states_gen.go")
	fresh, err := os.ReadFile("../../states_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	writeOutput(t, path, fresh)
	freshEvidence, err := checkCommand(command, dir).Output()
	if err != nil || string(freshEvidence) != "lawgate-freshness\tfresh\tstates_gen.go\n" {
		t.Fatalf("fresh evidence = %q, %v", freshEvidence, err)
	}
	stale := []byte("// stale\n")
	writeOutput(t, path, stale)
	output, err := checkCommand(command, dir).Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("stale output must exit 1, got %v\n%s", err, output)
	}
	if string(output) != "lawgate-freshness\tstale\tstates_gen.go\n" {
		t.Fatalf("the check must name the stale path: %s", output)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, stale) {
		t.Fatalf("the check changed stale output: %q, %v", after, err)
	}
}

func TestCheckModeReportsMissingOutputWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	command := buildGenerator(t, dir)
	output, err := checkCommand(command, dir).Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("missing output must exit 1, got %v", err)
	}
	if string(output) != "lawgate-freshness\tmissing\tstates_gen.go\n" {
		t.Fatalf("missing evidence = %q", output)
	}
	if _, err := os.Stat(filepath.Join(dir, "states_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("check wrote the missing target: %v", err)
	}
}

func buildGenerator(t *testing.T, dir string) string {
	t.Helper()
	command := filepath.Join(dir, "genstates")
	if output, err := exec.Command("go", "build", "-o", command, ".").CombinedOutput(); err != nil {
		t.Fatalf("build genstates: %v\n%s", err, output)
	}
	return command
}

func checkCommand(command, dir string) *exec.Cmd {
	cmd := exec.Command(command, "-check")
	cmd.Dir = dir
	return cmd
}

func writeOutput(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
