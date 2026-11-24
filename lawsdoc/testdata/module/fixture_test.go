package fixture

import "testing"

// TestNamesLaw enforces first-law.
func TestNamesLaw(t *testing.T) {}

func TestNamesLawInBody(t *testing.T) {
	t.Run("law:second-law", func(t *testing.T) {})
}

func TestNamesNothing(t *testing.T) {}

func Testlowercase(t *testing.T) {}
