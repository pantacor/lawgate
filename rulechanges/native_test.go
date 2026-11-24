package rulechanges

import (
	"slices"
	"strings"
	"testing"
)

func cgoSources(preamble string) map[string]string {
	return map[string]string{
		"lawgate.json":    `{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false}]}`,
		"go.mod":          "module example.com/project\ngo 1.25\n",
		"LAWS.md":         "### fixture-law\n- Enforced by: test `TestFixture`\n",
		"cgo/fixture.go":  "package cgo\n" + preamble + "import \"C\"\nfunc Limit() int { return int(C.LIMIT) }\n",
		"cgo/law_test.go": "package cgo\nimport \"testing\"\nfunc TestFixture(t *testing.T) { _ = Limit() }\n",
		"other/other.h":   "#define OTHER 1\n",
	}
}

// TestCgoHeadersAreProtected: a header the cgo preamble includes from a
// subdirectory, and the headers it includes in turn, are compiled into the
// package, so they are protected.
func TestCgoHeadersAreProtected(t *testing.T) {
	files := cgoSources("// #include \"include/limit.h\"\n")
	files["cgo/include/limit.h"] = "#include \"detail/value.h\"\n#include <stdint.h>\n"
	files["cgo/include/detail/value.h"] = "#define LIMIT 1\n"
	files["cgo/include/unused.h"] = ""
	wanted := []string{"cgo/include/limit.h", "cgo/include/detail/value.h"}
	requireRuleFiles(t, files, wanted, []string{"cgo/include/unused.h", "other/other.h"})
}

// TestCgoIncludeDirectoriesAreSearched: an angle include resolves through
// a #cgo -I directory, ${SRCDIR} included, and a C source of the package
// is followed too.
func TestCgoIncludeDirectoriesAreSearched(t *testing.T) {
	files := cgoSources("// #cgo CFLAGS: -I${SRCDIR}/../third/include\n// #include <limit.h>\n")
	files["third/include/limit.h"] = "#define LIMIT 1\n"
	files["cgo/impl.c"] = "#include \"../other/other.h\"\n"
	requireRuleFiles(t, files, []string{"third/include/limit.h", "other/other.h"}, nil)
}

// TestUnresolvedCgoIncludeProtectsThePackageTree: a quoted include that
// names no repository file may still name one through a directory lawgate
// cannot see, so the package's whole tree is protected.
func TestUnresolvedCgoIncludeProtectsThePackageTree(t *testing.T) {
	files := cgoSources("// #include \"generated/limit.h\"\n")
	files["cgo/deep/notes.txt"] = ""
	requireRuleFiles(t, files, []string{"cgo/deep/notes.txt"}, []string{"other/other.h"})
}

// requireCgoProblem fails unless classifying files, with links as the
// symbolic links, reports a problem naming want and protects each wanted
// path.
func requireCgoProblem(t *testing.T, files map[string]string, want string, wanted []string, links ...string) {
	t.Helper()
	result := classifyLinked(files, "", links...)
	err := result.err()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want a problem naming %q, got %v", want, err)
	}
	actual := sortedPaths(result.rules)
	for _, name := range wanted {
		if !slices.Contains(actual, name) {
			t.Errorf("%s is not a rule file; rule files = %v", name, actual)
		}
	}
}

// TestCgoFlagsApplyToEveryFileOfThePackage: the go command applies the
// #cgo flags of every Go file of a package to all of its cgo files, so a
// header found through a directory another file declares is protected.
func TestCgoFlagsApplyToEveryFileOfThePackage(t *testing.T) {
	files := cgoSources("// #include <limit.h>\n")
	files["cgo/zflags.go"] = "package cgo\n// #cgo CFLAGS: -I../third\nimport \"C\"\n"
	files["third/limit.h"] = "#define LIMIT 1\n"
	requireRuleFiles(t, files, []string{"third/limit.h"}, nil)
}

// TestLinkedCgoIncludeIsRefused: a header reached through a symbolic link
// is not the file Git tracks, so a quoted include through one and an
// include directory that is one are reported.
func TestLinkedCgoIncludeIsRefused(t *testing.T) {
	quoted := cgoSources("// #include \"../link/limit.h\"\n")
	requireCgoProblem(t, quoted, "symbolic link link", []string{"cgo/fixture.go"}, "link")
	searched := cgoSources("// #cgo CFLAGS: -I../link\n// #include <limit.h>\n")
	searched["cgo/deep/notes.txt"] = ""
	requireCgoProblem(t, searched, "symbolic link link", []string{"cgo/fixture.go", "cgo/deep/notes.txt"}, "link")
}

// TestOutsideCgoIncludeIsRefused: a header outside the repository cannot
// be protected, so a quoted include that leaves it, an include directory
// outside it and the angle include searched there are reported.
func TestOutsideCgoIncludeIsRefused(t *testing.T) {
	quoted := cgoSources("// #include \"../../outside/limit.h\"\n")
	requireCgoProblem(t, quoted, "outside the repository", []string{"cgo/fixture.go"})
	searched := cgoSources("// #cgo CFLAGS: -I/opt/outside/include\n// #include <limit.h>\n")
	requireCgoProblem(t, searched, "/opt/outside/include", []string{"cgo/fixture.go"})
	requireCgoProblem(t, searched, "<limit.h>", nil)
}

// TestUnknownCgoVariableIsReported: an include directory naming a
// variable other than ${SRCDIR} cannot be resolved, so it is reported and
// the package tree is protected.
func TestUnknownCgoVariableIsReported(t *testing.T) {
	files := cgoSources("// #cgo CPPFLAGS: -isystem ${HOME}/include\n// #include <limit.h>\n")
	files["cgo/deep/notes.txt"] = ""
	requireCgoProblem(t, files, "${HOME}/include", []string{"cgo/deep/notes.txt"})
}

// TestComputedCgoIncludeIsReported: #include MACRO names a header lawgate
// cannot know, so it is reported and the package tree is protected.
func TestComputedCgoIncludeIsReported(t *testing.T) {
	files := cgoSources("// #define LIMIT_HEADER \"limit.h\"\n// #include LIMIT_HEADER\n")
	files["cgo/deep/notes.txt"] = ""
	requireCgoProblem(t, files, "LIMIT_HEADER", []string{"cgo/deep/notes.txt"})
}
