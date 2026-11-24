package rulechanges

import (
	"slices"
	"testing"
)

// TestCgoForcedIncludeIsProtected: -include and -imacros make the compiler
// read a header before every C file of the package, with no #include in
// the source, so the header and the headers it includes are protected,
// found from the package directory or a #cgo -I directory, in each
// spelling of the option.
func TestCgoForcedIncludeIsProtected(t *testing.T) {
	flags := "-include ../inc/defs.h -I../third -imacros macros.h --include=../joined/value.h -include../glued/glued.h"
	files := cgoSources("// #cgo CFLAGS: " + flags + "\n")
	files["inc/defs.h"] = "#include \"more.h\"\n"
	files["inc/more.h"] = "#define LIMIT 1\n"
	files["third/macros.h"] = ""
	files["joined/value.h"] = ""
	files["glued/glued.h"] = ""
	wanted := []string{"inc/defs.h", "inc/more.h", "third/macros.h", "joined/value.h", "glued/glued.h"}
	requireRuleFiles(t, files, wanted, []string{"other/other.h"})
}

// TestCgoForcedIncludeProblemsAreReported: a forced header reached through
// a symbolic link, outside the repository or through an unknown variable
// is reported like a quoted include.
func TestCgoForcedIncludeProblemsAreReported(t *testing.T) {
	linked := cgoSources("// #cgo CFLAGS: -include ../link/defs.h\n")
	requireCgoProblem(t, linked, "symbolic link link", []string{"cgo/fixture.go"}, "link")
	outside := cgoSources("// #cgo CPPFLAGS: -include /opt/outside/defs.h\n")
	requireCgoProblem(t, outside, "outside the repository", []string{"cgo/fixture.go"})
	unknown := cgoSources("// #cgo CXXFLAGS: -imacros ${HOME}/defs.h\n")
	unknown["cgo/deep/notes.txt"] = ""
	requireCgoProblem(t, unknown, "${HOME}/defs.h", []string{"cgo/deep/notes.txt"})
}

// TestQuotedCgoFlagsAreSplitAsGoDoes: the go command splits #cgo flags
// with quotes and backslash escapes, so a quoted or escaped include
// directory is searched like a plain one.
func TestQuotedCgoFlagsAreSplitAsGoDoes(t *testing.T) {
	flags := `"-I../inc" -I'../my inc' -I "../separate" -I../back\ slash`
	files := cgoSources("// #cgo CFLAGS: " + flags + "\n// #include <a.h>\n// #include <b.h>\n// #include <c.h>\n// #include <d.h>\n")
	files["inc/a.h"] = ""
	files["my inc/b.h"] = ""
	files["separate/c.h"] = ""
	files["back slash/d.h"] = ""
	requireRuleFiles(t, files, []string{"inc/a.h", "my inc/b.h", "separate/c.h", "back slash/d.h"}, nil)
}

// TestQuotedCgoFlagProblemsAreReported: quoting an include directory does
// not hide a symbolic link or an outside directory, and a quote left open
// is reported.
func TestQuotedCgoFlagProblemsAreReported(t *testing.T) {
	linked := cgoSources("// #cgo CFLAGS: \"-I../link\"\n// #include <limit.h>\n")
	requireCgoProblem(t, linked, "symbolic link link", []string{"cgo/fixture.go"}, "link")
	outside := cgoSources("// #cgo CFLAGS: \"-I/opt/outside\"\n// #include <limit.h>\n")
	requireCgoProblem(t, outside, "/opt/outside", []string{"cgo/fixture.go"})
	open := cgoSources("// #cgo CFLAGS: \"-I../inc\n")
	requireCgoProblem(t, open, "unclosed quote", []string{"cgo/fixture.go"})
}

// TestSplitCgoFlagsMatchesGoBuild: the splitter keeps go/build's rules:
// quotes group, a backslash escapes inside quotes too, and an empty
// quoted argument is still an argument.
func TestSplitCgoFlagsMatchesGoBuild(t *testing.T) {
	cases := map[string][]string{
		` -I../a   -I../b `:   {"-I../a", "-I../b"},
		`"-I../a b" '-I../c'`: {"-I../a b", "-I../c"},
		`-I'../x'y\ z ""`:     {"-I../xy z", ""},
		`'\''`:                {"'"},
		`"a\"b"`:              {`a"b`},
	}
	for list, want := range cases {
		got, err := splitCgoFlags(list)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("splitCgoFlags(%q) = %q, %v; want %q", list, got, err, want)
		}
	}
	if _, err := splitCgoFlags(`-I../a\`); err != errUnfinishedEscape {
		t.Errorf("trailing backslash: %v", err)
	}
}

// TestEveryCgoHeaderCandidateIsProtected: GCC searches include
// directories in a fixed order (-iquote, -I, -isystem, the system
// directories, -idirafter), not in the order the flags are written, so
// every repository header a searched directory holds is protected,
// whichever the compiler reads first.
func TestEveryCgoHeaderCandidateIsProtected(t *testing.T) {
	cases := map[string][]string{
		"-idirafter ${SRCDIR}/../a -I${SRCDIR}/../b\n// #include <x.h>": {"a/x.h", "b/x.h"},
		"-isystem ../a -I../b\n// #include <x.h>":                       {"a/x.h", "b/x.h"},
		"-iquote ../a -I../b\n// #include <x.h>":                        {"b/x.h"},
		"-iquote ../a -I../b\n// #include \"x.h\"":                      {"a/x.h", "b/x.h"},
	}
	for flags, wanted := range cases {
		files := cgoSources("// #cgo CFLAGS: " + flags + "\n")
		files["a/x.h"] = "#define LIMIT 1\n"
		files["b/x.h"] = "#define LIMIT 2\n"
		requireRuleFiles(t, files, wanted, nil)
	}
}

// TestBracketedCgoIncludeSkipsQuoteDirectories: an -iquote directory is
// searched only for quoted includes, so it supplies no bracketed header.
func TestBracketedCgoIncludeSkipsQuoteDirectories(t *testing.T) {
	files := cgoSources("// #cgo CFLAGS: -iquote ../a\n// #include <x.h>\n")
	files["a/x.h"] = ""
	requireRuleFiles(t, files, nil, []string{"a/x.h"})
}

// TestCgoPkgConfigIsReported: #cgo pkg-config takes its include
// directories and forced headers from outside the repository when the
// package is built, so it is reported and the package tree is protected.
func TestCgoPkgConfigIsReported(t *testing.T) {
	files := cgoSources("// #cgo linux pkg-config: libfoo\n")
	files["cgo/deep/notes.txt"] = ""
	requireCgoProblem(t, files, "pkg-config: libfoo", []string{"cgo/fixture.go", "cgo/deep/notes.txt"})
}
