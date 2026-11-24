package lawsdoc

import (
	"slices"
	"strings"
	"testing"
)

func TestEnforcerFieldsFollowDocumentScope(t *testing.T) {
	document := strings.Join([]string{
		"## Group heading",
		"- Enforced by: test `ignored`",
		"### checked",
		"- Enforced by: test `TestHere`,",
		"  test `test title with spaces`, script `scripts/guard.sh`",
		"### known-gap",
		"- Enforced by: not yet. test `not-enforced`",
		"```md",
		"### example",
		"- Enforced by: test `example-only`",
		"```",
	}, "\n")
	references := Enforcers(document)
	if !slices.Equal(references.Tests, []string{"TestHere", "test title with spaces"}) {
		t.Errorf("tests = %v", references.Tests)
	}
	if !slices.Equal(references.Scripts, []string{"scripts/guard.sh"}) {
		t.Errorf("scripts = %v", references.Scripts)
	}
}
