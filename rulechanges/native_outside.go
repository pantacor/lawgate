package rulechanges

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The owner may approve include directories outside the repository in
// lawgate.json's outside_includes (owner decision, 2026-11-23): no
// CODEOWNERS entry can cover them, so lawgate accepts what the compiler
// reads there and notes each approved directory once, instead of failing
// on every header. An outside directory nobody approved is still a
// problem.

// approvedOutside is the error for an outside path inside an approved
// directory: callers accept it and note the directory.
type approvedOutside struct {
	directory string
}

func (approved approvedOutside) Error() string {
	return "is in the approved outside include directory " + approved.directory
}

// maxElsewhereNamed bounds the headers a package's note names.
const maxElsewhereNamed = 3

// outside is the error for name, an absolute path outside the repository:
// approved when an outside_includes directory holds it.
func (scan *nativeScan) outside(name string) error {
	for _, directory := range scan.reach.result.outsideIncludes {
		if withinDirectory(name, directory) {
			return approvedOutside{directory: directory}
		}
	}
	return errOutsideRepository
}

func withinDirectory(name, directory string) bool {
	prefix := strings.TrimSuffix(directory, "/") + "/"
	return name == directory || strings.HasPrefix(name, prefix)
}

// approved reports whether err is an approved outside path, and notes its
// directory once for the whole classification.
func (scan *nativeScan) approved(err error) bool {
	var approved approvedOutside
	if !errors.As(err, &approved) {
		return false
	}
	scan.reach.result.note(fmt.Sprintf("cgo reads the approved outside include directory %s: lawgate cannot protect it", approved.directory))
	return true
}

// noteElsewhere notes, once for the package, the bracketed headers no
// repository directory holds: the compiler reads them from its own
// directories or an approved one, which no CODEOWNERS entry can cover.
func (scan *nativeScan) noteElsewhere() {
	if len(scan.elsewhere) == 0 {
		return
	}
	names := sortedPaths(scan.elsewhere)
	shown := names[:min(len(names), maxElsewhereNamed)]
	listed := "<" + strings.Join(shown, ">, <") + ">"
	if len(names) > len(shown) {
		listed += fmt.Sprintf(" and %d more", len(names)-len(shown))
	}
	scan.reach.result.note(fmt.Sprintf("%s: cgo includes %s from outside the repository (the compiler's system directories or an approved one)", scan.directory, listed))
}

// note records text once: a fact the owner should see that does not fail
// the gate.
func (result *classification) note(text string) {
	if slices.Contains(result.notes, text) {
		return
	}
	result.notes = append(result.notes, text)
}
