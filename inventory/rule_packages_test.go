package inventory

import (
	"strings"
	"testing"
)

func rulePackagesConfig(packages string) string {
	return editConfig(`"modules"`, `"rule_packages":`+packages+`,"modules"`)
}

// TestRulePackagesAreValidated: a malformed rule_packages
// entry is a configuration error, not a silent no-op.
func TestRulePackagesAreValidated(t *testing.T) {
	cases := map[string]string{
		`[{"path":""}]`:                               "invalid rule package path",
		`[{"path":" example.test/x"}]`:                "invalid rule package path",
		`[{"path":"example.test/x/"}]`:                "invalid rule package path",
		`[{"path":"example.test/x","names":["   "]}]`: "invalid rule package name",
		`[{"path":"example.test/x","names":["1a"]}]`:  "invalid rule package name",
		`[{"path":"example.test/x","names":["A.B"]}]`: "invalid rule package name",
	}
	for packages, want := range cases {
		_, err := Decode(strings.NewReader(rulePackagesConfig(packages)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("rule_packages %s: %v, want %q", packages, err, want)
		}
	}
	valid := `[{"path":"example.test/legacy","names":["Machine","law_v2"]},{"path":"example.test/all"}]`
	if _, err := Decode(strings.NewReader(rulePackagesConfig(valid))); err != nil {
		t.Fatalf("valid rule_packages: %v", err)
	}
}
