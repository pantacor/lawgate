package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGoModulesSkipNonPackageDirectories(t *testing.T) {
	root := sourceModule(t)
	for _, dir := range []string{".", "worker", "testdata/fixture", "vendor/dependency", ".hidden/module", "_disabled", "node_modules/pkg"} {
		writeSourceFixture(t, root, filepath.Join(dir, "go.mod"), "module example.test/fixture")
	}
	modules, err := GoModules(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(modules, []string{".", "worker"}) {
		t.Fatalf("go modules = %q", modules)
	}
}

func TestDeclaredMachineNamesUseSourceAndVocabulary(t *testing.T) {
	root := sourceModule(t)
	names := []string{"constant", "dot", "literal"}
	writeMachineSources(t, root)
	rule := RulePackage{Path: "example.test/legacy", Names: []string{"Machine", "Law"}}
	found, err := DeclaredMachineNames(root, ".", []RulePackage{rule})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(found, names) {
		t.Fatalf("declared machine names = %q, want %q", found, names)
	}
}

func writeMachineSources(t *testing.T, root string) {
	const alias = "package p; import m \"github.com/pantacor/lawgate/machine\";"
	declarations := "const worker = \"constant\"; var a = m.Machine{Name: worker};"
	declarations += "var b = m.Machine{Name: (\"literal\")}; var c = m.Machine{Name: worker};"
	declarations += "var d m.Machine = m.Machine{Name: worker};"
	writeSourceFixture(t, root, "alias.go", alias+declarations)
	dotted := "package p; import . \"example.test/legacy\"; var dot = Machine{Name: \"dot\"}"
	writeSourceFixture(t, root, "dotted.go", dotted)
	writeSourceFixture(t, root, "unrelated.go", "package p; import m \"example.test/unrelated\"; var other = m.Machine{Name: \"other\"}")
	writeSourceFixture(t, root, "machine_test.go", "not Go")
	writeSourceFixture(t, root, "testdata/fixture.go", "not Go")
	writeSourceFixture(t, root, "nested/go.mod", "module example.test/nested")
	writeSourceFixture(t, root, "nested/malformed.go", "not Go")
}

func TestDeclaredMachineNamesReportsUnparseableSource(t *testing.T) {
	root := sourceModule(t)
	writeSourceFixture(t, root, "broken.go", "not Go")
	if _, err := DeclaredMachineNames(root, ".", nil); err == nil {
		t.Fatal("expected a diagnostic for unparseable source")
	}
}

func writeSourceFixture(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMachineSourceHonorsBuildConstraints(t *testing.T) {
	root := sourceModule(t)
	source := "package p; import m \"github.com/pantacor/lawgate/machine\"; var a = m.Machine{Name: \"optional\"}"
	writeSourceFixture(t, root, "optional.go", "//go:build lawgate_optional\n\n"+source)
	names, err := DeclaredMachineNames(root, ".", nil)
	if err != nil || len(names) != 0 {
		t.Fatalf("machine names = %q, %v", names, err)
	}
}

func TestMachineSourceReportsUnresolvedDeclarations(t *testing.T) {
	cases := []string{
		"func build(name string) *Machine { return &Machine{Name: name} }",
		"var m Machine",
		"var m = new(Machine)",
		`func build(suffix string) Machine { m := Machine{Name: "plain"}; m.Name += suffix; return m }`,
		`func clone(name string) Machine { m := Machine{Name: "plain"}; m.Name = name; return m }`,
	}
	for _, source := range cases {
		root := sourceModule(t)
		writeSourceFixture(t, root, "machine.go", "package p; import . \"github.com/pantacor/lawgate/machine\"; "+source)
		if _, err := DeclaredMachineNames(root, ".", nil); err == nil {
			t.Errorf("accepted unresolved machine: %s", source)
		}
	}
}

func TestGoModulesScanSecretNamedDirectory(t *testing.T) {
	root := sourceModule(t)
	writeSourceFixture(t, root, "secretsync/go.mod", "module example.test/worker")
	if modules, err := GoModules(root); err != nil || !slices.Contains(modules, "secretsync") {
		t.Fatalf("modules = %v, %v", modules, err)
	}
}

func sourceModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	manifest := "module example.test/fixture\n\ngo 1.25\n"
	for _, path := range []string{"github.com/pantacor/lawgate", "example.test/legacy", "example.test/unrelated"} {
		target := "testdata/" + filepath.Base(path)
		manifest += "require " + path + " v0.0.0\nreplace " + path + " => ./" + target + "\n"
		writeFixtureRulePackage(t, root, path, target)
	}
	writeSourceFixture(t, root, "go.mod", manifest)
	return root
}

func writeFixtureRulePackage(t *testing.T, root, path, target string) {
	t.Helper()
	writeSourceFixture(t, root, target+"/go.mod", "module "+path+"\n\ngo 1.25\n")
	if filepath.Base(path) == "lawgate" {
		target += "/machine"
	}
	name := filepath.Base(target)
	writeSourceFixture(t, root, target+"/machine.go", "package "+name+"; type Machine struct { Name string }")
}

func TestMachineSourceFollowsCommandTagsAndTarget(t *testing.T) {
	root := sourceModule(t)
	const source = "package p; import m \"github.com/pantacor/lawgate/machine\"; var M = m.Machine{Name: \"portable\"}"
	writeSourceFixture(t, root, "machine_linux_arm64.go", "//go:build lawgate_target\n\n"+source)
	writeSourceFixture(t, root, "worker.go", "package p")
	writeSourceFixture(t, root, "cmd/machine/main.go", `package main; import _ "example.test/fixture"; func main() {}`)
	module := Module{Path: ".", MachineCommand: []string{"env", "GOOS=linux", "GOARCH=amd64", "go", "run", "-tags", "lawgate_target", "./cmd/machine"}}
	assertMachineNames(t, root, module, nil)
	module.MachineCommand[2] = "GOARCH=arm64"
	assertMachineNames(t, root, module, []string{"portable"})
	module.MachineCommand[1] = "GOOS=windows"
	assertMachineNames(t, root, module, nil)
}

func TestMachineSourceFollowsOpaqueRunnerInventory(t *testing.T) {
	root := sourceModule(t)
	const source = "package p; import m \"github.com/pantacor/lawgate/machine\"; var M = m.Machine{Name: \"opaque\"}"
	writeSourceFixture(t, root, "machine.go", "//go:build lawgate_opaque\n\n"+source)
	build := &GoMachineBuild{Tags: []string{"lawgate_opaque"}}
	module := Module{Path: ".", MachineCommand: []string{"./machine-runner"}, MachineBuild: build}
	assertMachineNames(t, root, module, []string{"opaque"})
}

func assertMachineNames(t *testing.T, root string, module Module, want []string) {
	t.Helper()
	names, err := module.MachineNames(root, nil)
	if err != nil || !slices.Equal(names, want) {
		t.Fatalf("machine names = %q, %v; want %q", names, err, want)
	}
}

func TestMachineDiscoveryScansSecretNamedGoSource(t *testing.T) {
	root := sourceModule(t)
	const source = "package p; import m \"github.com/pantacor/lawgate/machine\"; var M = m.Machine{Name: \"%s\"}"
	writeSourceFixture(t, root, "secrets.go", fmt.Sprintf(source, "root"))
	writeSourceFixture(t, root, "secrets/worker.go", fmt.Sprintf(source, "nested"))
	assertMachineNames(t, root, Module{Path: "."}, []string{"nested", "root"})
}

const typedMachineImport = "import m \"github.com/pantacor/lawgate/machine\"; "

func TestMachineDiscoveryFollowsTypesAndNames(t *testing.T) {
	root := sourceModule(t)
	source := "package p; " + typedMachineImport + `
 type Alias = m.Machine
 type Defined Alias
 const prefix = "orig"
 var Original = Alias{Name: prefix + "inal"}
 var Copy = Original
 func variant() *Alias { copy := Copy; copy.Name = "variant"; return &copy }
 func defined() Defined { return Defined{Name: "defined"} }
 var Result = variant()
 `
	writeSourceFixture(t, root, "machine.go", source)
	assertMachineNames(t, root, Module{Path: "."}, []string{"defined", "original", "variant"})
}

func TestMachineDiscoveryFollowsExplicitFileOperands(t *testing.T) {
	root := sourceModule(t)
	source := "//go:build lawgate_excluded\n\npackage main; " + typedMachineImport
	writeSourceFixture(t, root, "runner.go", source+`var D = m.Machine{Name: "explicit"}; func main() {}`)
	writeSourceFixture(t, root, "ignored.go", "package main; "+typedMachineImport+`var D = m.Machine{Name: "ignored"}`)
	module := Module{Path: ".", MachineCommand: []string{"go", "run", "runner.go", "-args", "ignored.go"}}
	assertMachineNames(t, root, module, []string{"explicit"})
}

func TestMachineDiscoveryFollowsPackageOperandsAndImports(t *testing.T) {
	root := sourceModule(t)
	writeSourceFixture(t, root, "cmd/current/main.go", `package main; import _ "example.test/fixture/worker"; func main() {}`)
	writeSourceFixture(t, root, "worker/worker.go", "package worker; "+typedMachineImport+`var D = m.Machine{Name: "worker"}`)
	writeSourceFixture(t, root, "cmd/other/main.go", "package main; "+typedMachineImport+`var D = m.Machine{Name: "other"}; func main() {}`)
	module := Module{Path: ".", MachineCommand: []string{"go", "run", "./cmd/current"}}
	assertMachineNames(t, root, module, []string{"worker"})
}

func TestMachineDiscoveryFollowsGoTestRunner(t *testing.T) {
	root := sourceModule(t)
	writeSourceFixture(t, root, "machine_test.go", "package p; "+typedMachineImport+`var D = m.Machine{Name: "test-runner"}`)
	writeSourceFixture(t, root, "other/unrelated.go", "package other; "+typedMachineImport+`var D = m.Machine{Name: "other"}`)
	module := Module{Path: ".", MachineCommand: []string{"go", "test", "-run", "TestReport", "machine_test.go", "-args"}}
	assertMachineNames(t, root, module, []string{"test-runner"})
}

func TestMachineDiscoveryResolvesNamedZeroValue(t *testing.T) {
	root := sourceModule(t)
	source := "package p; " + typedMachineImport + `
func build() m.Machine {
	var result m.Machine
	result.Name = "worker"
	return result
}
var declaration = build()
`
	writeSourceFixture(t, root, "machine.go", source)
	assertMachineNames(t, root, Module{Path: "."}, []string{"worker"})
}

func TestMachineDiscoveryRejectsConditionalName(t *testing.T) {
	root := sourceModule(t)
	source := "package p; " + typedMachineImport + `
func build(ok bool) m.Machine {
	var result m.Machine
	if ok { result.Name = "worker" }
	return result
}
`
	writeSourceFixture(t, root, "machine.go", source)
	_, err := (Module{Path: "."}).MachineNames(root, nil)
	if err == nil {
		t.Fatal("accepted a return path with an unknown machine name")
	}
}
