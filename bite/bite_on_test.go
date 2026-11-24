//go:build lawgate_bite

package bite

import "testing"

func TestOffSelectsOnlyTheNamedLaw(t *testing.T) {
	t.Setenv(Env, "some-law")
	if !Off("some-law") || Off("other-law") {
		t.Fatal("tagged build did not select exactly the named law")
	}
	t.Setenv(Env, "")
	if Off("") || Off("some-law") {
		t.Fatal("tagged build selected a law for an empty variable")
	}
}
