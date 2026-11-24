//go:build never

package fixture

import "testing"

// TestGhost enforces first-law, but no build compiles it.
func TestGhost(t *testing.T) {}
