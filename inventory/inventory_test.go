package inventory

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const validConfig = `{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false}]}`

func TestDecodeValidation(t *testing.T) {
	cases := map[string]string{
		`{`:                                 "unexpected EOF",
		`{"bogus":1}`:                       "unknown field",
		validConfig + "{}":                  "expected exactly one JSON object",
		`{"modules":[]}`:                    "no modules",
		editConfig(`,"machines":false`, ""): "machines must be true or false",
		editConfig(`"path":"."`, `"path":"../escape"`):                                                                        "invalid module path",
		editConfig(`"laws":"LAWS.md"`, `"laws":"."`):                                                                          "invalid laws path",
		editConfig(`"language":"go"`, `"language":"ruby"`):                                                                    "unsupported language",
		editConfig(`"machines":false`, `"machines":false,"machine_command":["run"]`):                                          "machine command requires machines",
		editConfig(`"language":"go","laws":"LAWS.md","machines":false`, `"language":"node","laws":"LAWS.md","machines":true`): "missing machine command",
		editConfig(`"machines":false`, `"machines":false,"shared_laws":["../LAWS.md"]`):                                       "invalid shared laws path",
		editConfig(`"machines":false`, `"machines":false,"required_tests":["\n"]`):                                            "invalid required test",
		editConfig(`}]`, `},{"path":".","language":"go","laws":"LAWS.md","machines":false}]`):                                 "duplicate module",
		editConfig(`"modules"`, `"gate_files":["."],"modules"`):                                                               "invalid gate file",
		editConfig(`"modules"`, `"platform":"other","modules"`):                                                               "unsupported platform",
		editConfig(`"modules"`, `"owner":"@owner","modules"`):                                                                 "owner requires a platform",
		editConfig(`"modules"`, `"readability":{"baseline":"."},"modules"`):                                                   "invalid readability baseline",
		editConfig(`"modules"`, `"readability":{"baseline":"rules.txt","exclude":["["]},"modules"`):                           "invalid readability glob",
	}
	for input, want := range cases {
		_, err := Decode(strings.NewReader(input))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Decode(%s) = %v, want %q", input, err, want)
		}
	}
}

func editConfig(old, replacement string) string {
	return strings.Replace(validConfig, old, replacement, 1)
}

func TestLoadValidConfigurationAndDefaultCommands(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Fatal("accepted absent inventory")
	}
	options := `"owner":"@owner","platform":"github","gate_files":["scripts/check"],`
	options += `"readability":{"baseline":"rules.txt","include":["**.go"]},`
	options += `"rule_packages":[{"path":"example.test/legacy","names":["Machine"]}],`
	config := editConfig(`"modules"`, options+`"modules"`)
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.RulePackages) != 1 || loaded.RulePackages[0].Path != "example.test/legacy" {
		t.Fatalf("rule packages = %v", loaded.RulePackages)
	}
	module := loaded.Modules[0]
	assertArgs(t, module.TestArgs(), "go test -json -count=1 ./...")
	assertArgs(t, module.MachineArgs("report.xml"), "go run ./cmd/machine -laws -junit report.xml")
}

func TestNodeCommandsAndGoBuildOverrides(t *testing.T) {
	module := Module{Language: "node", MachineCommand: []string{"machine-runner"}}
	assertArgs(t, module.TestArgs(), "node --test --test-reporter=tap")
	assertArgs(t, module.MachineArgs("example"), "machine-runner -laws -junit example")
	module.TestCommand = strings.Fields("go test -count=1 -tags foo -mod=vendor -race ./... -args -tags bar")
	if !slices.Equal(module.TestArgs(), module.TestCommand) {
		t.Fatal("lost command override")
	}
	assertArgs(t, module.GoTestBuildFlags(), "-tags foo -mod=vendor -race")
	module.TestCommand = []string{"sh", "-c", "go test"}
	assertArgs(t, module.GoTestBuildFlags(), "")
}

func assertArgs(t *testing.T, arguments []string, expected string) {
	t.Helper()
	if got := strings.Join(arguments, " "); got != expected {
		t.Errorf("command %q, want %q", got, expected)
	}
}

func TestDecodeRequiresBuildInventoryForOpaqueGoRunner(t *testing.T) {
	input := editConfig(`"machines":false`, `"machines":true,"machine_command":["./runner"]`)
	_, err := Decode(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "requires machine_build") {
		t.Fatalf("opaque runner without scoped inventory: %v", err)
	}
	input = strings.Replace(input, `"./runner"]`, `"./runner"],"machine_build":{}`, 1)
	if _, err := Decode(strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeAllowsConfiguredLawDocumentNames(t *testing.T) {
	input := editConfig(`"laws":"LAWS.md"`, `"laws":"rules/worker.md","shared_laws":["rules/common.md"]`)
	if _, err := Decode(strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
}
