package review

import (
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/review/internal/fixture"
)

func buildFixture(t *testing.T, root string) *Index {
	t.Helper()
	idx, err := Build(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("problems: %+v", idx.Problems)
	}
	return idx
}

// TestBuildTiesSitesStatesSubjectsAndTestsToLaws builds the index of an
// adopting repository through its machine command and its LAWS.md.
func TestBuildTiesSitesStatesSubjectsAndTestsToLaws(t *testing.T) {
	root := fixture.Repo(t)
	idx := buildFixture(t, root)
	checkSiteAndState(t, root, idx)
	checkLaws(t, root, idx)
}

func checkSiteAndState(t *testing.T, root string, idx *Index) {
	t.Helper()
	tagLine := fixture.Line(t, root, "worker.go", `"SHOP_JOB 1`)
	sites := idx.SitesAt("worker.go", tagLine)
	if len(sites) != 1 || sites[0].Machine != "pay" {
		t.Fatalf("sites at worker.go:%d = %+v", tagLine, sites)
	}
	site := sites[0]
	declared := site.DeclFile == "machine.go" && site.DeclLine == fixture.Line(t, root, "machine.go", `Tag: "1"`)
	if !declared || strings.Join(site.Laws, ",") != "never-overspend" {
		t.Fatalf("site = %+v", site)
	}
	states := idx.StatesIn("worker.go", tagLine, tagLine)
	if len(states) != 1 || states[0].ID != "pay" {
		t.Fatalf("states = %+v", states)
	}
	if want := fixture.Line(t, root, "worker.go", "func Pay"); states[0].StartLine != want {
		t.Fatalf("state pay starts on %d, want %d", states[0].StartLine, want)
	}
}

func checkLaws(t *testing.T, root string, idx *Index) {
	t.Helper()
	machineLaw := idx.Laws["no-silent-errors"]
	if machineLaw == nil || machineLaw.Outcome() != OutcomeHolds {
		t.Fatalf("no-silent-errors = %+v", machineLaw)
	}
	if machineLaw.Verdicts[0].Machine != "pay" {
		t.Fatalf("no-silent-errors verdicts = %+v", machineLaw.Verdicts)
	}
	proved := idx.Laws["never-overspend"]
	if proved == nil || proved.Outcome() != OutcomeUnchecked {
		t.Fatalf("never-overspend = %+v", proved)
	}
	subjects := idx.SubjectsIn("worker.go", 1, 100)
	if len(subjects) != 1 || subjects[0].Line != fixture.Line(t, root, "worker.go", "func affordable") {
		t.Fatalf("subjects = %+v", subjects)
	}
	tests := idx.TestsIn("worker_test.go", 1, 100)
	spansBody := len(tests) == 1 && tests[0].EndLine > tests[0].Line
	if !spansBody || tests[0].Law != "never-overspend" {
		t.Fatalf("tests = %+v", tests)
	}
}

// TestBuildRecordsAMachineCommandThatFails: a module whose machine command
// cannot build is a problem of the index, not a failed review.
func TestBuildRecordsAMachineCommandThatFails(t *testing.T) {
	root := fixture.Repo(t)
	fixture.Replace(t, root, "cmd/machine/main.go", "runner.Main(", "runner.Missing(")
	idx, err := Build(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Problems) != 1 || !strings.Contains(idx.Problems[0].Message, "machine index") {
		t.Fatalf("problems = %+v", idx.Problems)
	}
	if idx.Laws["never-overspend"] == nil {
		t.Fatal("the laws of LAWS.md are missing when the machine command fails")
	}
}

// TestBuildPlacesDriftOnItsLine: a tag no site claims is a problem on the
// tag's line.
func TestBuildPlacesDriftOnItsLine(t *testing.T) {
	root := fixture.Repo(t)
	fixture.Replace(t, root, "worker.go", `return record("SHOP_JOB 2 recorded")`, "record(\"SHOP_JOB 2 recorded\")\n\treturn fail(\"SHOP_JOB 3 unclaimed\")")
	idx, err := Build(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	line := fixture.Line(t, root, "worker.go", "SHOP_JOB 3")
	found := len(idx.Problems) == 1 && idx.Problems[0].Line == line
	if !found || idx.Problems[0].File != "worker.go" {
		t.Fatalf("problems = %+v, want the unclaimed tag on worker.go:%d", idx.Problems, line)
	}
}

func TestIndexArgsReplaceTheCheckFlags(t *testing.T) {
	custom := inventory.Module{MachineCommand: []string{"go", "run", "./tools/machines"}}
	args, err := IndexArgs(custom, "/tmp/index.json")
	if err != nil || strings.Join(args, " ") != "go run ./tools/machines -index /tmp/index.json" {
		t.Fatalf("args = %v, %v", args, err)
	}
	args, err = IndexArgs(inventory.Module{}, "-")
	if err != nil || strings.Join(args, " ") != "go run ./cmd/machine -index -" {
		t.Fatalf("default args = %v, %v", args, err)
	}
}

func TestSectionEndStopsAtTheNextHeading(t *testing.T) {
	doc := strings.Split("# laws\n\n## one\n\nText.\n\n```\n## not a heading\n```\n\n## two\n\nMore.\n", "\n")
	if end := sectionEnd(doc, 3); end != 9 {
		t.Fatalf("end of one = %d, want 9", end)
	}
	if end := sectionEnd(doc, 11); end != 13 {
		t.Fatalf("end of two = %d, want 13", end)
	}
}

func TestDuplicateLawNamesKeepBothLaws(t *testing.T) {
	idx := &Index{Laws: make(map[string]*Law), modules: []string{".", "sub"}}
	idx.addDocument(".", "LAWS.md")
	idx.addDocument("sub", "sub/LAWS.md")
	idx.addLaw(&Law{Name: "fresh", Module: ".", File: "LAWS.md"})
	idx.addLaw(&Law{Name: "fresh", Module: "sub", File: "sub/LAWS.md"})
	if law := idx.LawFor("fresh", "sub/worker.go"); law == nil || law.Key != "sub/LAWS.md#fresh" {
		t.Fatalf("law for sub = %+v", law)
	}
	if law := idx.LawFor("fresh", "main.go"); law == nil || law.Key != "fresh" {
		t.Fatalf("law for the root = %+v", law)
	}
}

func TestPackageDirsUseTheLongestModule(t *testing.T) {
	modules := packageDirs([]goModule{{importPath: "example.com/shop", dir: "."}, {importPath: "example.com/shop/tools", dir: "tools"}})
	cases := map[string]string{
		"example.com/shop":           ".",
		"example.com/shop/pay":       "pay",
		"example.com/shop/tools/gen": "tools/gen",
	}
	for importPath, want := range cases {
		if dir, ok := modules.dirFor(importPath); !ok || dir != want {
			t.Errorf("dirFor(%s) = %s, %v; want %s", importPath, dir, ok, want)
		}
	}
	if _, ok := modules.dirFor("example.org/other"); ok {
		t.Error("a package outside every module was found")
	}
}
