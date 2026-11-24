package inventory

import (
	"slices"
	"strings"
	"testing"
)

// A bite rerun must not write what the configured command writes.
func TestSelectTestDropsFlagsThatWriteFiles(t *testing.T) {
	writers := []string{"-coverprofile=cover.out", "-cpuprofile", "cpu.out", "-memprofile=mem.out",
		"-blockprofile", "block.out", "-mutexprofile=mutex.out", "-trace", "trace.out",
		"-outputdir=out", "-test.coverprofile=again.out"}
	kept := []string{"-cover", "-covermode", "atomic", "-short", "-timeout", "1m", "-race"}
	argv := slices.Concat([]string{"go", "test", "-json"}, writers, kept, []string{"./..."})
	command, err := ParseGoCommand(t.TempDir(), argv)
	if err != nil {
		t.Fatal(err)
	}
	selected := command.SelectTest("TestLaw", "lawgate_bite", []string{"example.test/pkg"})
	for _, fragment := range []string{"profile", ".out", "outputdir", "-trace"} {
		if strings.Contains(strings.Join(selected, " "), fragment) {
			t.Errorf("rerun keeps a %q flag: %q", fragment, selected)
		}
	}
	for _, word := range kept {
		if !slices.Contains(selected, word) {
			t.Errorf("rerun lost %q: %q", word, selected)
		}
	}
}

func TestGoFlagErrorsNameTheFlag(t *testing.T) {
	cases := map[string][]string{
		"unsupported Go flag -overlay":   {"go", "test", "-overlay", "x.json", "."},
		"Go flag -timeout requires":      {"go", "test", "-timeout"},
		"Go flag -modfile requires a .m": {"go", "test", "-modfile=go.sum", "."},
	}
	for want, argv := range cases {
		if _, err := ParseGoCommand(".", argv); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want %q", argv, err, want)
		}
	}
}
