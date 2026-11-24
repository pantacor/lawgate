package inventory

import (
	"strings"
	"testing"
)

func outsideIncludesConfig(directories string) string {
	return editConfig(`"modules"`, `"outside_includes":`+directories+`,"modules"`)
}

// TestOutsideIncludesAreValidated: an approved outside include directory
// must be a clean absolute path, listed once; anything else would approve
// a directory the owner did not name.
func TestOutsideIncludesAreValidated(t *testing.T) {
	cases := map[string]string{
		`["usr/include"]`:                 "invalid outside include directory",
		`["/usr/include/../lib"]`:         "invalid outside include directory",
		`["/usr/include/"]`:               "invalid outside include directory",
		`[""]`:                            "invalid outside include directory",
		`["/"]`:                           "invalid outside include directory",
		`["/usr/include","/usr/include"]`: "duplicate outside include directory",
	}
	for directories, want := range cases {
		_, err := Decode(strings.NewReader(outsideIncludesConfig(directories)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("outside_includes %s: %v, want %q", directories, err, want)
		}
	}
	valid := `["/usr/include/libfoo","/opt/sdk/include"]`
	config, err := Decode(strings.NewReader(outsideIncludesConfig(valid)))
	if err != nil {
		t.Fatalf("valid outside_includes: %v", err)
	}
	if len(config.OutsideIncludes) != 2 {
		t.Fatalf("outside_includes = %v", config.OutsideIncludes)
	}
}

// TestOutsideIncludesMustNotHoldTheRepository: an approved directory that
// is the repository root or holds it would approve every repository file,
// and neighbouring checkouts, without review.
func TestOutsideIncludesMustNotHoldTheRepository(t *testing.T) {
	for _, directory := range []string{"/work", "/work/repo"} {
		directories := []string{"/opt/sdk", directory}
		config := Config{OutsideIncludes: directories}
		err := config.CheckOutsideIncludes("/work/repo")
		if err == nil || !strings.Contains(err.Error(), "holds the repository") {
			t.Errorf("outside include %s holding /work/repo: %v", directory, err)
		}
	}
	beside := []string{"/opt/sdk", "/work/repository", "/work/repo-sdk"}
	allowed := Config{OutsideIncludes: beside}
	if err := allowed.CheckOutsideIncludes("/work/repo"); err != nil {
		t.Errorf("directories beside the repository were refused: %v", err)
	}
}
