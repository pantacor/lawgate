//go:build !lawgate_bite

package bite

import "testing"

func TestOffIgnoresAmbientBite(t *testing.T) {
	t.Setenv(Env, "some-law")
	if Off("some-law") {
		t.Fatal("normal build honored LAWGATE_BITE")
	}
}
