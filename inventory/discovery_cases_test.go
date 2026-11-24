package inventory

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfiguredGoRejectsUnclassifiedInputs(t *testing.T) {
	flags := []string{"-overlay=alt.json", "-exec", "-toolexec=tool", "-pkgdir=alternate", "-pgo=profile", "-unknown"}
	for _, flag := range flags {
		config := fmt.Sprintf(`{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false,"test_command":["go","test",%q,"./..."]}]}`, flag)
		_, err := Decode(strings.NewReader(config))
		name, _, _ := strings.Cut(flag, "=")
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: got %v", flag, err)
		}
	}
}

func TestMachineSourceUsesGoWorkingDirectory(t *testing.T) {
	root := sourceModule(t)
	source := "package main; " + typedMachineImport + `var M = m.Machine{Name: "worker"}; func main() {}`
	writeSourceFixture(t, root, "tools/cmd/runner/main.go", source)
	command := []string{"go", "run", "-C", "tools", "./cmd/runner"}
	assertMachineNames(t, root, Module{Path: ".", MachineCommand: command}, []string{"worker"})
}

func TestMachineSourceNamesAllocationBeforeReturn(t *testing.T) {
	root := sourceModule(t)
	source := "package p; " + typedMachineImport + `func build() *m.Machine { result := new(m.Machine); result.Name = "worker"; return result }`
	writeSourceFixture(t, root, "machine.go", source)
	assertMachineNames(t, root, Module{Path: "."}, []string{"worker"})
}

func TestMachineSourceRefusesUnnamedLocalPublication(t *testing.T) {
	root := sourceModule(t)
	source := "package p; " + typedMachineImport + `func publish(any) {}; func build() { var result m.Machine; publish(&result) }`
	writeSourceFixture(t, root, "machine.go", source)
	_, err := (Module{Path: "."}).MachineNames(root, nil)
	if err == nil {
		t.Fatal("accepted an unnamed local machine publication")
	}
}

func TestMachineSourceFindsPublishedEmbeddedMachine(t *testing.T) {
	root := sourceModule(t)
	source := "package p; " + typedMachineImport + `type wrapper struct { m.Machine }; func publish(any) {}; func build() { var result wrapper; result.Name = "worker"; publish(&result.Machine) }`
	writeSourceFixture(t, root, "machine.go", source)
	assertMachineNames(t, root, Module{Path: "."}, []string{"worker"})
}

func TestConfigAcceptsModuleTimeout(t *testing.T) {
	config := `{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false,"timeout_seconds":1}]}`
	parsed, err := Decode(strings.NewReader(config))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Modules[0].Timeout(); got != time.Second {
		t.Fatalf("timeout = %v, want %v", got, time.Second)
	}
	if got := (Module{}).Timeout(); got != 300*time.Second {
		t.Fatalf("default timeout = %v, want 300s", got)
	}
}

func TestConfiguredGoRefusesGOFLAGSPrefix(t *testing.T) {
	command := []string{"env", "GOFLAGS=-mod=readonly", "go", "test", "./..."}
	if _, err := ParseGoCommand(".", command); err == nil || !strings.Contains(err.Error(), "GOFLAGS") {
		t.Fatalf("expected GOFLAGS refusal, got %v", err)
	}
}

func TestGateCommandRunsWithSelectedGoInputs(t *testing.T) {
	root := sourceModule(t)
	t.Setenv("GOFLAGS", "-overlay=unavailable.json")
	writeSourceFixture(t, root, "tools/main.go", "package main; import \"fmt\"; func main() { fmt.Print(\"selected\") }")
	writeSourceFixture(t, root, "alt.mod", "module example.test/fixture\n\ngo 1.25\n")
	command := []string{"env", "GOWORK=off", "go", "run", "-C=tools", "-modfile", "../alt.mod", "main.go"}
	output, stderr, err := RunGateCommand(root, command)
	if err != nil || string(output) != "selected" {
		t.Fatalf("output %q, stderr %q, error %v", output, stderr, err)
	}
}

func TestConfigRejectsModuleTimeoutOutsideBounds(t *testing.T) {
	for _, seconds := range []int{-1, 0, 86401} {
		config := fmt.Sprintf(`{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false,"timeout_seconds":%d}]}`, seconds)
		_, err := Decode(strings.NewReader(config))
		if err == nil || !strings.Contains(err.Error(), "timeout_seconds") {
			t.Errorf("timeout_seconds %d: got %v", seconds, err)
		}
	}
}

func TestGateCommandSkipsWorkspaceAboveRepository(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	writeSourceFixture(t, parent, "go.work", "invalid workspace")
	writeSourceFixture(t, root, Filename, "{}")
	writeSourceFixture(t, root, "go.mod", "module example.test/package\ngo 1.25\n")
	writeSourceFixture(t, root, "check.go", "package fixture")
	_, stderr, err := RunGateCommand(root, []string{"go", "test", "./..."})
	if err != nil {
		t.Fatalf("unexpected parent workspace: %v\n%s", err, stderr)
	}
}
