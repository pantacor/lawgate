package rulechanges

import (
	"strings"
	"testing"
)

// approvingSources are cgoSources with lawgate.json approving the outside
// include directories listed.
func approvingSources(preamble, approved string) map[string]string {
	files := cgoSources(preamble)
	files["lawgate.json"] = `{"outside_includes":` + approved + `,"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false}]}`
	return files
}

// notesNaming are the notes of result that contain text.
func notesNaming(result *classification, text string) []string {
	var named []string
	for _, note := range result.notes {
		if strings.Contains(note, text) {
			named = append(named, note)
		}
	}
	return named
}

// TestApprovedOutsideIncludesAreNotedOnce: include directories and headers
// inside an approved outside directory are accepted, and the directory is
// noted once, not once per flag or header.
func TestApprovedOutsideIncludesAreNotedOnce(t *testing.T) {
	preamble := "// #cgo CFLAGS: -I/usr/include/libfoo -I/usr/include/libfoo/sub -include /usr/include/libfoo/config.h\n" +
		"// #include <foo.h>\n// #include \"/usr/include/libfoo/bar.h\"\n// #include <stdio.h>\n"
	result := classifyLinked(approvingSources(preamble, `["/usr/include/libfoo"]`), "")
	if err := result.err(); err != nil {
		t.Fatalf("approved outside includes were refused: %v", err)
	}
	if named := notesNaming(result, "/usr/include/libfoo"); len(named) != 1 {
		t.Fatalf("want one note for the approved directory, got %q", named)
	}
}

// TestUnlistedOutsideIncludeIsStillRefused: approving one outside
// directory does not approve another.
func TestUnlistedOutsideIncludeIsStillRefused(t *testing.T) {
	files := approvingSources("// #cgo CFLAGS: -I/opt/other\n// #include <limit.h>\n", `["/usr/include/libfoo"]`)
	requireCgoProblem(t, files, "/opt/other", []string{"cgo/fixture.go"})
	near := approvingSources("// #cgo CFLAGS: -I/usr/include/libfoobar\n", `["/usr/include/libfoo"]`)
	requireCgoProblem(t, near, "/usr/include/libfoobar", []string{"cgo/fixture.go"})
}

// TestSystemHeadersAreNotedOncePerPackage: bracketed headers the
// compiler finds in its own directories are not problems, and they are
// noted once for the package rather than once per header.
func TestSystemHeadersAreNotedOncePerPackage(t *testing.T) {
	files := cgoSources("// #include <stdio.h>\n// #include <string.h>\n")
	files["cgo/other.go"] = "package cgo\n// #include <stdlib.h>\nimport \"C\"\n"
	result := classifyLinked(files, "")
	if err := result.err(); err != nil {
		t.Fatalf("system headers were refused: %v", err)
	}
	named := notesNaming(result, "cgo: cgo includes")
	if len(named) != 1 || !strings.Contains(named[0], "<stdlib.h>") {
		t.Fatalf("want one note for package cgo naming its headers, got %q", result.notes)
	}
}

// TestOutsideIncludeHoldingTheRepositoryApprovesNothing: an approved
// directory that holds the repository is reported on lawgate.json, and
// approves nothing, so an outside include is still refused.
func TestOutsideIncludeHoldingTheRepositoryApprovesNothing(t *testing.T) {
	files := approvingSources("// #cgo CFLAGS: -I/work/sdk\n", `["/work"]`)
	result := classifyLinked(files, "/work/repo")
	err := result.err()
	for _, want := range []string{"outside include directory \"/work\" holds the repository", "/work/sdk\" is outside the repository"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want a problem naming %q, got %v", want, err)
		}
	}
}
