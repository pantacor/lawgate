package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const gitlabFallback = "[Law gate]\n* @owner\n"

func readFixture(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// requireFallback fails unless content opens with the platform's fallback
// block before the generated entries.
func requireFallback(t *testing.T, platform, content string) {
	t.Helper()
	generated := strings.Index(content, "# lawgate rule files")
	fallback := strings.Index(content, "* @owner\n")
	if fallback < 0 || generated < fallback {
		t.Fatalf("%s: no fallback before the generated entries:\n%s", platform, content)
	}
	section := strings.Contains(content, gitlabFallback)
	if section != (platform == "gitlab") {
		t.Fatalf("%s: [Law gate] section present = %v:\n%s", platform, section, content)
	}
}

func TestInitFallbackFollowsThePlatform(t *testing.T) {
	github := initFixture(t, "github", "--fallback")
	requireFallback(t, "github", readFixture(t, github, ".github/CODEOWNERS"))
	gitlab := initFixture(t, "gitlab", "--fallback")
	requireFallback(t, "gitlab", readFixture(t, gitlab, "CODEOWNERS"))
}

func gitlabOwnersRepository(t *testing.T) string {
	t.Helper()
	root := ownersRepository(t)
	config := strings.ReplaceAll(ownedConfig, `"platform":"github"`, `"platform":"gitlab"`)
	writeFixture(t, root, "lawgate.json", strings.ReplaceAll(config, "@org/law-owners", "@owner"))
	return root
}

func codeownersOutput(t *testing.T, arguments ...string) string {
	t.Helper()
	var output bytes.Buffer
	if err := run(append([]string{"codeowners"}, arguments...), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

// TestCodeownersFallbackFollowsThePlatform also covers --owner: overriding
// the owner keeps lawgate.json's platform.
func TestCodeownersFallbackFollowsThePlatform(t *testing.T) {
	root := gitlabOwnersRepository(t)
	requireFallback(t, "gitlab", codeownersOutput(t, "--root", root, "--fallback"))
	overridden := codeownersOutput(t, "--root", root, "--fallback", "--owner", "@owner")
	requireFallback(t, "gitlab", overridden)
	explicit := codeownersOutput(t, "--root", root, "--fallback", "--platform", "github")
	requireFallback(t, "github", explicit)
}

func TestOwnerFlagKeepsTheConfiguredPlatform(t *testing.T) {
	root := gitlabOwnersRepository(t)
	policy, err := resolveOwnerPolicy(ownerOptions{root: root, owner: "@override"})
	if err != nil {
		t.Fatal(err)
	}
	if policy.owner != "@override" || policy.platform != "gitlab" {
		t.Fatalf("policy = %+v; want @override on gitlab", policy)
	}
	bare := t.TempDir()
	policy, err = resolveOwnerPolicy(ownerOptions{root: bare, owner: "@override"})
	if err != nil || policy.platform != "github" {
		t.Fatalf("without lawgate.json: policy = %+v, err = %v", policy, err)
	}
}

// TestOwnerCommandsRefuseFlagsTheyIgnore: a flag a command does not use is
// refused, so a CI typo cannot pass silently.
func TestOwnerCommandsRefuseFlagsTheyIgnore(t *testing.T) {
	root := t.TempDir()
	refused := []string{
		"rule-files --owner @x",
		"rule-files --fallback",
		"codeowners --base main",
		"check-codeowners --fallback",
	}
	for _, line := range refused {
		arguments := append(strings.Fields(line), "--root", root)
		err := run(arguments, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
			t.Errorf("%v = %v; want the flag refused", arguments, err)
		}
	}
}

// TestExampleGeneratorHasExactOwnership enforces rule-changes-are-reviewed
// even for adopters relying solely on generated exact entries.
func TestExampleGeneratorHasExactOwnership(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	output := codeownersOutput(t, "--root", "../..")
	if strings.Contains(output, "\n* ") || !strings.Contains(output, "\n/example/cmd/genstates/main.go @highercomve\n") {
		t.Fatal("generated exact ownership does not cover the example generator")
	}
}

// TestInvalidPlatformIsRefused: a mistyped --platform must not
// silently produce a GitHub CODEOWNERS block.
func TestInvalidPlatformIsRefused(t *testing.T) {
	root := ownersRepository(t)
	for _, action := range []string{"codeowners", "check-codeowners"} {
		err := run([]string{action, "--root", root, "--owner", "@owner", "--platform", "gitLab"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), `platform "gitLab"`) {
			t.Fatalf("%s accepted an invalid platform: %v", action, err)
		}
	}
}
