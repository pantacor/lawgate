package rulechanges

import (
	"strings"
	"testing"
)

// linkedThrough is the problem a walk through the symbolic link
// other/link reports.
const linkedThrough = "symbolic link other/link"

// TestCgoPathsAreWalkedAsWritten: the compiler opens "link/../x.h"
// through the link, so a forced header named from ${SRCDIR} with a link
// and ".." in it is refused, not cleaned into a path that hides the link.
func TestCgoPathsAreWalkedAsWritten(t *testing.T) {
	files := cgoSources("// #cgo CFLAGS: -include ${SRCDIR}/../other/link/../defs.h\n")
	files["unowned/defs.h"] = "#define LIMIT 42\n"
	requireCgoProblem(t, files, linkedThrough, []string{"cgo/fixture.go"}, "other/link")
}

// TestAbsoluteCgoPathsAreWalkedAsWritten: an absolute include directory,
// forced header or include inside the repository is walked from the root
// element by element too, so a link in it is refused.
func TestAbsoluteCgoPathsAreWalkedAsWritten(t *testing.T) {
	preambles := []string{
		"// #cgo CFLAGS: -I/work/repo/other/link/..\n// #include <defs.h>\n",
		"// #cgo CFLAGS: -include /work/repo/other/link/../defs.h\n",
		"// #include \"/work/repo/other/link/../defs.h\"\n",
	}
	for _, preamble := range preambles {
		files := cgoSources(preamble)
		files["unowned/defs.h"] = "#define LIMIT 42\n"
		requireRootedCgoProblem(t, files, "/work/repo", linkedThrough)
	}
}

// TestCgoLinkIsMetBeforeAnApprovedOutsideDirectory: a path that cleans
// lexically into an approved outside directory, but that the compiler
// opens through a link inside the repository, is refused for the link
// rather than accepted as approved.
func TestCgoLinkIsMetBeforeAnApprovedOutsideDirectory(t *testing.T) {
	preamble := "// #include \"../other/link/../../../approved/defs.h\"\n"
	files := approvingSources(preamble, `["/work/approved"]`)
	requireRootedCgoProblem(t, files, "/work/repo", linkedThrough)
}

// TestCgoWalkThatReentersTheRepositoryIsChecked: a path that leaves the
// repository and comes back in by its directory name is inside it again,
// so a link on the way back in is still refused.
func TestCgoWalkThatReentersTheRepositoryIsChecked(t *testing.T) {
	files := cgoSources("// #include \"../../repo/other/link/../defs.h\"\n")
	requireRootedCgoProblem(t, files, "/work/repo", linkedThrough)
}

// requireRootedCgoProblem fails unless classifying files in a worktree at
// root, with other/link a symbolic link, reports a problem naming want.
func requireRootedCgoProblem(t *testing.T, files map[string]string, root, want string) {
	t.Helper()
	result := classifyLinked(files, root, "other/link")
	err := result.err()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("want a problem naming %q, got %v", want, err)
	}
}
