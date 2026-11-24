package rulechanges

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGoCommandProtectsEffectiveManifest(t *testing.T) {
	files := commandBuildFiles()
	files["lawgate.json"] = commandConfig(`"env","GOWORK=off","go","run","-C=tools","-modfile=../alt.mod","./runner"`)
	files["alt.mod"] = files["go.mod"] + "replace example.test/dep => ./dep\n"
	files["alt.sum"] = ""
	assertCommandRules(t, files, t.TempDir(), "alt.mod", "alt.sum", "dep/dep.go", "tools/runner/main.go")
}

func TestGoCommandProtectsExplicitWorkspace(t *testing.T) {
	files := commandBuildFiles()
	root := t.TempDir()
	workspace := filepath.Join(root, "nested", "alternate.work")
	files["lawgate.json"] = commandConfig(fmt.Sprintf(`"env",%q,"go","run","-C","tools","./runner"`, "GOWORK="+workspace))
	files["nested/alternate.work"] = "go 1.25\nuse (\n../\n../dep\n)\n"
	files["nested/alternate.work.sum"] = ""
	assertCommandRules(t, files, root, "nested/alternate.work", "nested/alternate.work.sum", "dep/dep.go")
}

func commandBuildFiles() map[string]string {
	files := make(map[string]string)
	files["go.mod"] = "module example.test/top\ngo 1.25\nrequire example.test/dep v0.0.0\n"
	files["dep/go.mod"] = "module example.test/dep\ngo 1.25\n"
	files["dep/dep.go"] = "package dep\n"
	files["tools/runner/main.go"] = "package main\nimport _ \"example.test/dep\"\nfunc main() {}\n"
	return files
}

func commandConfig(args string) string {
	return `{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false,"generate_check":[` + args + `]}]}`
}

func assertCommandRules(t *testing.T, files map[string]string, root string, expected ...string) {
	t.Helper()
	result := classifyCommandFiles(files, root)
	if err := result.err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range expected {
		if !result.rules[name] {
			t.Errorf("%s is unprotected: %v", name, sortedPaths(result.rules))
		}
	}
}

func classifyCommandFiles(files map[string]string, root string) *classification {
	var paths []string
	for name := range files {
		paths = append(paths, name)
	}
	slices.Sort(paths)
	tree := sourceTree{root: root, paths: paths, read: func(name string) ([]byte, error) {
		if data, ok := files[name]; ok {
			return []byte(data), nil
		}
		return nil, fmt.Errorf("missing %q", name)
	}}
	tree.paths = append(tree.paths, "LAWS.md")
	files["LAWS.md"] = "# Laws"
	return classify(tree)
}

func TestGoCommandModModeProtectsReplacementEvenWithVendor(t *testing.T) {
	for _, mode := range []string{`"-mod","mod"`, `"-mod=readonly"`} {
		files := commandBuildFiles()
		files["lawgate.json"] = commandConfig(`"go","run",` + mode + `,"./tools/runner"`)
		files["go.mod"] += "replace example.test/dep => ./dep\n"
		files["vendor/modules.txt"] = "# example.test/dep v0.0.0 => ./dep\nexample.test/dep\n"
		files["vendor/example.test/dep/dep.go"] = "package dep\n"
		assertCommandRules(t, files, t.TempDir(), "dep/dep.go")
	}
}

func TestGoCommandProtectsExplicitUnderscorePackage(t *testing.T) {
	files := commandBuildFiles()
	files["lawgate.json"] = commandConfig(`"go","run","./_tool"`)
	files["_tool/main.go"] = "package main\nfunc main() {}\n"
	assertCommandRules(t, files, t.TempDir(), "_tool/main.go")
}

func TestGoCommandReadOnlyRefusesExternalReplaceDespiteVendor(t *testing.T) {
	files := commandBuildFiles()
	files["lawgate.json"] = commandConfig(`"go","run","-mod=readonly","./tools/runner"`)
	files["go.mod"] += "replace example.test/dep => ../elsewhere\n"
	files["vendor/modules.txt"] = "# example.test/dep v0.0.0 => ../elsewhere\nexample.test/dep\n"
	files["vendor/example.test/dep/dep.go"] = "package dep\n"
	result := classifyCommandFiles(files, t.TempDir())
	if err := result.err(); err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("classified replace as vendored: %v", err)
	}
}
