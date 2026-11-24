package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

// TestBiteRunsAgreedEnforcers enforces laws-bite for both Go and scripts.
func TestBiteRunsAgreedEnforcers(t *testing.T) {
	root := t.TempDir()
	writeBiteGoModule(t, root, "sub")
	writeFixture(t, root, "sub/law_test.go", hookedBiteTestSource)
	writeExecutable(t, root, "rules/bites.sh", "#!/bin/sh\ntest -f go.mod || exit 2\n"+rejectScriptLaw("script-law"))
	writeExecutable(t, root, "rules/blind.sh", "#!/bin/sh\nexit 0\n")
	command := []string{"go", "test", "-json", "-count=1", "-run", "^(TestLaw|TestSecondLaw|TestBlind|TestSkips)$", "./..."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	result := biteFixture(module, root)
	checkBites(root, module, &result)
	expectBite(t, result, "go-law", passed, "", "example.test/bites/sub/TestLaw", "TestSecondLaw")
	expectBite(t, result, "script-law", passed, "")
	expectBite(t, result, "blind-law", failed, "passed")
	expectBite(t, result, "blind-script", failed, "passed")
	expectBite(t, result, "skipped-law", failed, "did not fail")
	expectBite(t, result, "gap-law", pending, "owner recorded a gap", "TestBlind", "rules/blind.sh")
}

const hookedBiteTestSource = `package sub
import "testing"
func TestLaw(t *testing.T) {
	if !acceptsLaw() { t.Fatal("go-law") }
}
func TestSecondLaw(t *testing.T) {
	if !acceptsLaw() { t.Fatal("go-law") }
}
func TestLawOther(t *testing.T) { t.Fatal("unselected test should never run") }
func TestBlind(t *testing.T) { _ = "blind-law and gap-law" }
func TestSkips(t *testing.T) {
	if skipLaw() { t.Skip("skipped-law") }
}
`

func writeBiteGoModule(t *testing.T, root, subdir string) {
	t.Helper()
	writeBiteModuleManifest(t, root)
	writeFixture(t, root, filepath.Join(subdir, "hook.go"), biteHookSource)
}

func writeBiteModuleManifest(t *testing.T, root string) {
	t.Helper()
	local, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	mod := "module example.test/bites\n\ngo 1.25\n\nrequire github.com/pantacor/lawgate v0.0.0\n"
	mod += "replace github.com/pantacor/lawgate => " + strconv.Quote(local) + "\n"
	writeFixture(t, root, "go.mod", mod)
}

const biteHookSource = `package sub
import "github.com/pantacor/lawgate/bite"
func acceptsLaw() bool { return !bite.Off("go-law") }
func skipLaw() bool { return bite.Off("skipped-law") }
func unrelatedHook() bool { return bite.Off("blind-law") }
func shortLawOff() bool { return bite.Off("short-law") }
`

const biteTestSource = `package sub
import ("github.com/pantacor/lawgate/bite"; "testing")
func TestLaw(t *testing.T) {
	if bite.Off("go-law") { t.Fatal("go-law") }
}
`

func biteFixture(module inventory.Module, root string) moduleResult {
	testDoc := strings.Replace(biteLaw("go-law", "test", "example.test/bites/sub/TestLaw"), "- Status:", "  test `TestSecondLaw`\n- Status:", 1)
	testDoc += biteLaw("blind-law", "test", "TestBlind")
	testDoc += biteLaw("skipped-law", "test", "TestSkips")
	gap := biteLaw("gap-law", "test", "TestBlind")
	gap = strings.Replace(gap, "- Status:", "  script `rules/blind.sh`\n- Status:", 1)
	testDoc += gap + "- Bites: not yet. owner recorded a gap\n"
	scripts := biteLaw("script-law", "script", "bites.sh") + biteLaw("blind-script", "script", "blind.sh")
	shared := lawDocument{Path: "rules/laws.md", Plans: lawsdoc.Plan(scripts)}
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(testDoc), SharedDocs: []lawDocument{shared}}
	checkTests(root, module, &result)
	checkScripts(root, shared, &result)
	return result
}

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

func TestBiteUsesActiveGoBuild(t *testing.T) {
	root := t.TempDir()
	writeBiteGoModule(t, filepath.Join(root, "sub"), ".")
	writeFixture(t, root, "sub/law_test.go", "//go:build bithook\n\n"+hookedBiteTestSource)
	command := []string{"env", "GOWORK=off", "go", "test", "-C", "sub", "-tags", "bithook", "-json", "-run", "^TestLaw$", "."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	plan := lawsdoc.Plan(biteLaw("go-law", "test", "TestLaw"))
	result := moduleResult{Ready: true, Plans: plan}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	expectBite(t, result, "go-law", passed, "")
}

func TestBiteChecksExampleEnforcers(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	module := inventory.Module{Path: "example", Language: "go", Laws: "example/LAWS.md"}
	doc, err := readLawDocument(root, module.Laws)
	if err != nil {
		t.Fatal(err)
	}
	result := moduleResult{Ready: true, Plans: doc.Plans}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	expectBite(t, result, "rejects-empty-payload", passed, "")
	expectBite(t, result, "generated-is-fresh", passed, "")
}

func TestBitePreservesGoTestFlags(t *testing.T) {
	root := t.TempDir()
	writeBiteGoModule(t, root, ".")
	writeFixture(t, root, "law_test.go", biteShortSource)
	command := []string{"go", "test", "-json", "-short", "-timeout", "1m", "."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	plan := lawsdoc.Plan(biteLaw("short-law", "test", "TestShort"))
	result := moduleResult{Ready: true, Plans: plan}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	expectBite(t, result, "short-law", passed, "")
}

const biteShortSource = `package sub
import ("testing"; "time")
func TestShort(t *testing.T) {
	if !testing.Short() { t.Fatal("short-law requires -short") }
	if deadline, ok := t.Deadline(); !ok || time.Until(deadline) > time.Minute { t.Fatal("short-law requires -timeout") }
	if shortLawOff() { t.Fatal("short-law") }
}
`

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

func TestBiteRejectsTestOnlyHook(t *testing.T) {
	root := t.TempDir()
	writeBiteModuleManifest(t, root)
	writeFixture(t, root, "law_test.go", biteTestSource)
	command := []string{"go", "test", "-json", "-run", "^TestLaw$", "."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(biteLaw("go-law", "test", "TestLaw"))}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	expectBite(t, result, "go-law", failed, "bite hook missing")
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
	doc := biteLaw("first-law", "test", "TestFirst") + biteLaw("second-law", "test", "TestSecond")
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

func TestBiteKeepsRepositoryWorkspaceBoundary(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	writeFixture(t, parent, "go.work", "go 1.25\nuse ./missing\n")
	writeFixture(t, root, "lawgate.json", "{}")
	writeBiteGoModule(t, root, ".")
	writeFixture(t, root, "law_test.go", hookedBiteTestSource)
	command := []string{"go", "test", "-json", "-run", "^TestLaw$", "."}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(biteLaw("go-law", "test", "TestLaw"))}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	expectBite(t, result, "go-law", passed, "")
}

func TestBitePreservesTestBinaryArgs(t *testing.T) {
	root := t.TempDir()
	writeBiteGoModule(t, root, ".")
	writeFixture(t, root, "law_test.go", biteArgsTestSource)
	command := []string{"go", "test", "-json", ".", "-args", "-strict"}
	module := inventory.Module{Path: ".", Language: "go", Laws: "LAWS.md", TestCommand: command}
	result := moduleResult{Ready: true, Plans: lawsdoc.Plan(biteLaw("go-law", "test", "TestLaw"))}
	checkTests(root, module, &result)
	checkBites(root, module, &result)
	expectBite(t, result, "go-law", passed, "")
}

const biteArgsTestSource = `package sub
import ("flag"; "testing")
var strict = flag.Bool("strict", false, "require strict test")
func TestLaw(t *testing.T) {
	if !*strict || !acceptsLaw() { t.Fatal("go-law") }
}
`
