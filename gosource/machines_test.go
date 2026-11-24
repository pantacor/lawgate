package gosource

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const importsMachine = "package worker; import . \"example.test/rules\"; "

func TestMachineDeclarationsByOrigin(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"alias, copy and defined type", `type Alias = Machine; type Defined Alias; var m = Defined{Name: "name"}; var copy = m`, "name"},
		{"pointer allocation", `func build() *Machine { m := new(Machine); m.Name = "name"; return m }; var result = build()`, "name"},
		{"local literal", `func build() *Machine { m := &Machine{}; m.Name = "name"; return m }`, "name"},
		{"named result", `func build() (m Machine) { m.Name = "name"; return }`, "name"},
		{"positional literal", `var m = Machine{"name"}`, "name"},
		{"typed conversion", `type Alias Machine; var m = Machine(Alias{Name: "name"})`, "name"},
		{"forwarded parameters", `func expose(m *Machine, w Wrapper) any { return []any{m, w.Machine} }; type Wrapper struct { Machine }`, ""},
		{"publication in collection", `func build() { var m Machine; _ = []any{&m} }`, "?"},
		{"publication through interface conversion", `func build() any { var m Machine; return any(&m) }`, "?"},
		{"publication by send", `func build(ch chan any) { var m Machine; ch <- &m }`, "?"},
		{"unnamed global", `var m Machine`, "?"},
		{"unnamed call", `func publish(any) {}; func build() { var m Machine; publish(&m) }`, "?"},
		{"conditional name", `func build(ok bool) Machine { var m Machine; if ok {m.Name = "name"}; return m }`, "?,name"},
		{"dynamic write", `func build(name string) Machine { var m Machine; m.Name = name; return m }`, "?"},
		{"map origin", `func build(ms map[string]Machine) Machine { return ms["name"] }`, "?"},
		{"goto over write", `func build(skip bool) Machine { var m Machine; if skip {goto out}; m.Name = "name"; out: return m }`, "?,name"},
		{"embedded identity", `type Wrapper struct { Machine }; func publish(any) {}; func build() { var a, b Wrapper; a.Name = "name"; publish(&b.Machine) }`, "?,name"},
		{"promoted field", `type Wrapper struct { Machine }; func publish(any) {}; func build() { var w Wrapper; w.Name = "name"; publish(&w.Machine) }`, "name"},
		{"overwritten wrapper", `type Wrapper struct { Machine }; func publish(any) {}; func build() { var w Wrapper; w.Name = "name"; w = Wrapper{}; publish(&w.Machine) }`, "?,name"},
		{"unrelated field", `type Wrapper struct { Name string; Machine }; func publish(any) {}; func build() { var w Wrapper; w.Name = "name"; publish(&w.Machine) }`, "?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertMachineOrigins(t, tc.body, tc.want) })
	}
}

func assertMachineOrigins(t *testing.T, body, want string) {
	t.Helper()
	assertSourceOrigins(t, importsMachine+body, want)
}

func assertSourceOrigins(t *testing.T, source, want string) {
	t.Helper()
	root, packages := machinePackages(t, source)
	rulePackages := map[string]bool{"example.test/rules": true}
	found, err := MachineDeclarations(root, packages, rulePackages)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, decl := range found {
		name := decl.Name
		if name == "" {
			name = "?"
		}
		names = append(names, name)
	}
	slices.Sort(names)
	if got := strings.Join(slices.Compact(names), ","); got != want {
		t.Fatalf("machine origins = %q, want %q", got, want)
	}
}

func machinePackages(t *testing.T, source string) (string, []Package) {
	t.Helper()
	root, err := CanonicalRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(t.TempDir(), "machine.go")
	writeGoFixture(t, rules, "package rules; type Machine struct { Name string }")
	writeGoFixture(t, filepath.Join(root, "worker.go"), source)
	packages := []Package{
		{Dir: root, ImportPath: "example.test/worker", GoFiles: []string{"worker.go"}, Imports: []string{"example.test/rules"}},
		{Dir: filepath.Dir(rules), ImportPath: "example.test/rules", GoFiles: []string{rules}},
	}
	return root, packages
}

func writeGoFixture(t *testing.T, file, text string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

// go/types cannot know the type of a cgo operand. It must neither crash the
// scan nor hide the machines declared beside it: one converted from a cgo
// call is an unresolved origin.
func TestMachineDeclarationsScanCgoSource(t *testing.T) {
	cgo := `package worker

// static int one(void) { return 1; }
import "C"
import . "example.test/rules"

func one() int { return int(C.one()) }

func publish(any) {}

func call() { publish(C.one()) }

var count C.int

func unnamed() Machine { return Machine(C.one()) }

var named = Machine{Name: "name"}
`
	assertSourceOrigins(t, cgo, "?,name")
}

func TestMachineDeclarationsReportsBadSource(t *testing.T) {
	root, packages := machinePackages(t, "not Go")
	if _, err := MachineDeclarations(root, packages, map[string]bool{"example.test/rules": true}); err == nil {
		t.Fatal("accepted invalid Go source")
	}
}

func TestReadPackagesFiltersTheGoListInput(t *testing.T) {
	data := `{"Dir":"/src","ImportPath":"example.test/p","GoFiles":["p.go","drop.go"],"CgoFiles":["c.go"],
 "InvalidGoFiles":["bad.go"],"TestGoFiles":["p_test.go"],"XTestGoFiles":["ext_test.go"]}`
	files := map[string]bool{filepath.Join("/src", "p.go"): true, filepath.Join("/src", "c.go"): true}
	packages, err := ReadPackages(strings.NewReader(data), files)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join("/src", "c.go"), filepath.Join("/src", "p.go")}
	if got := packages[0].Sources(); !slices.Equal(got, want) {
		t.Fatalf("sources = %v, want %v", got, want)
	}
	if len(packages[0].Tests()) != 0 {
		t.Fatal("kept inactive test files")
	}
}

func TestReadPackagesReportsGoListErrors(t *testing.T) {
	for _, data := range []string{`{"Error":{"Err":"cannot load package"}}`, `{"Dir":`} {
		if _, err := ReadPackages(strings.NewReader(data), nil); err == nil {
			t.Errorf("accepted %q", data)
		}
	}
}
