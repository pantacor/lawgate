package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

func TestBiteRequiresFailureFromTheNamedNodeTest(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "test.sh", nodeBiteSource)
	command := []string{"sh", "test.sh"}
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md", TestCommand: command}
	doc := biteLaw("real-law", "test", "first") + biteLaw("unrelated-law", "test", "first")
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(doc)}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	expectBite(t, result, "real-law", passed, "")
	expectBite(t, result, "unrelated-law", failed, "did not fail")
}

const nodeBiteSource = `#!/bin/sh
printf '%s\n' 'TAP version 13'
if [ "$LAWGATE_BITE" = real-law ]; then
	printf '%s\n' 'not ok 1 - first' 'ok 2 - other' '1..2'
	exit 1
fi
if [ "$LAWGATE_BITE" = unrelated-law ]; then
	printf '%s\n' 'ok 1 - first' 'not ok 2 - other' '1..2'
	exit 1
fi
printf '%s\n' 'ok 1 - first' 'ok 2 - other' '1..2'
`

func biteLaw(name, kind, enforcer string) string {
	return "\n### " + name + "\n- Source: owner decision 2026-11-20\n- Enforced by: " + kind + " `" + enforcer + "`\n- Status: agreed 2026-11-20\n"
}

func expectBite(t *testing.T, result moduleResult, law, status, detail string, enforcers ...string) {
	t.Helper()
	var found []string
	for _, item := range result.Verdicts {
		if !strings.HasPrefix(item.Name, "laws-bite ("+law+":") {
			continue
		}
		if item.Status != status || !strings.Contains(item.Detail, detail) {
			t.Fatalf("bite for %s: got %+v, want %s and %q", law, item, status, detail)
		}
		_, enforcer, _ := strings.Cut(item.Name, ", ")
		found = append(found, strings.TrimSuffix(enforcer, ")"))
	}
	if len(enforcers) == 0 && len(found) == 1 {
		return
	}
	if len(enforcers) == 0 || !slices.Equal(found, enforcers) {
		t.Fatalf("bite for %s: got enforcers %v, expected %v: %+v", law, found, enforcers, result.Verdicts)
	}
}

func writeExecutable(t *testing.T, root, name, content string) {
	t.Helper()
	writeFixture(t, root, name, content)
	if err := os.Chmod(filepath.Join(root, name), 0700); err != nil {
		t.Fatal(err)
	}
}

func rejectScriptLaw(law string) string {
	return fmt.Sprintf("if [ \"$LAWGATE_BITE\" = %[1]s ]; then printf 'lawgate: rejects %[1]s\\n'; exit 1; fi\n", law)
}

func TestBiteRejectsScriptInfrastructureFailure(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, root, "rule.sh", "#!/bin/sh\nif [ \"$LAWGATE_BITE\" = script-law ]; then exit 2; fi\n")
	result := scriptBite(t, root, "rule.sh")
	expectBite(t, result, "script-law", infrastructure, "exit status 2")
}

func TestBiteRequiresScriptRejectionMarker(t *testing.T) {
	for _, output := range []string{"", "lawgate: rejects a-different-law", "prefix lawgate: rejects script-law"} {
		assertScriptMarkerRequired(t, output)
	}
}

func assertScriptMarkerRequired(t *testing.T, output string) {
	t.Helper()
	root := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$LAWGATE_BITE\" = script-law ]; then printf '%%s\\n' '%s'; exit 1; fi\n", output)
	writeExecutable(t, root, "rule.sh", script)
	result := scriptBite(t, root, "rule.sh")
	expectBite(t, result, "script-law", infrastructure, "lawgate: rejects script-law")
}

func TestBiteSetsEnvironmentForScriptNamedEnv(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, root, "env", "#!/bin/sh\n"+rejectScriptLaw("script-law"))
	result := scriptBite(t, root, "env")
	expectBite(t, result, "script-law", passed, "")
}

func scriptBite(t *testing.T, root, script string) moduleResult {
	t.Helper()
	module := inventory.Module{Path: ".", Language: "node", Laws: "LAWS.md"}
	doc := lawDocument{Path: module.Laws, Plans: lawsdoc.Plan(biteLaw("script-law", "script", script))}
	result := moduleResult{Ready: true, Plans: doc.Plans}
	checkScripts(root, doc, &result)
	checkBites(root, module, &result)
	return result
}

func TestBiteGapIsReportedAsPendingReason(t *testing.T) {
	item := verdict{Name: "laws-bite (go-law: LAWS.md, TestLaw)", Status: pending, Detail: "bite gap: owner recorded a gap"}
	if got := reportVerdict(item); !got.IsPending() || got.Law.Pending != item.Detail {
		t.Fatalf("verdict = %+v, want pending with %q", got, item.Detail)
	}
}

func TestBiteDiscoveryReportsOneInfrastructureError(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/bites\n\ngo 1.25\n")
	command := []string{"go", "test", "-overlay", "unknown", "."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	doc := goLaw("first-law", "TestFirst", "example.test/bites.First") + goLaw("second-law", "TestSecond", "example.test/bites.Second")
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(doc)}
	checkLawsDoc(root, module, &result)
	checkBites(root, module, &result)
	discovery := 0
	for _, item := range result.Verdicts {
		if item.Status == passed {
			continue
		}
		if item.Name != "test discovery" || item.Status != infrastructure {
			t.Errorf("unexpected extra verdict: %+v", item)
		}
		discovery++
	}
	hasCause := slices.ContainsFunc(result.Verdicts, func(item verdict) bool {
		return strings.Contains(item.Detail, "unsupported Go flag -overlay")
	})
	if discovery != 1 || !hasCause {
		t.Fatalf("expected one discovery error: %+v", result.Verdicts)
	}

}

func TestLawgateBinaryRefusesAmbientBite(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "lawgate")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	for _, value := range []string{"rule-changes-are-reviewed", ""} {
		assertAmbientBiteRefused(t, binary, value)
	}
}

func assertAmbientBiteRefused(t *testing.T, binary, value string) {
	t.Helper()
	t.Setenv("LAWGATE_BITE", value)
	command := exec.Command(binary, "init", "--help")
	out, err := command.CombinedOutput()
	if biteExitCode(err) != 2 || !strings.Contains(string(out), "LAWGATE_BITE is set") {
		t.Fatalf("LAWGATE_BITE=%q: exit %v, output %s", value, err, out)
	}
}
