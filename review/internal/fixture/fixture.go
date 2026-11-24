// Package fixture builds, for the review tests, a small adopting repository
// in a temporary directory: a Go module ("shop") with one machine, a machine
// law, a law proved by a test with a subject, a lawgate.json, and a Git
// history on main.
package fixture

import (
	"embed"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

//go:embed testdata/repo
var repo embed.FS

// Repo writes the shop repository into a new temporary directory, commits
// it on main and returns its root. Its go.mod points lawgate at this
// checkout, so the machine command builds offline.
func Repo(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	files, _ := fs.Sub(repo, "testdata/repo")
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, _ := fs.ReadFile(files, name)
		Write(t, root, name, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	Write(t, root, "go.mod", goMod())
	Git(t, root, "init", "--quiet", "--initial-branch=main")
	Commit(t, root, "shop: pay within the balance")
	return root
}

func goMod() string {
	_, file, _, _ := runtime.Caller(0)
	lawgate := filepath.Join(filepath.Dir(file), "..", "..", "..")
	lines := []string{
		"module example.com/shop",
		"",
		"go 1.25",
		"",
		"require github.com/pantacor/lawgate v0.0.0",
		"",
		"replace github.com/pantacor/lawgate => " + filepath.ToSlash(lawgate),
	}
	return strings.Join(lines, "\n") + "\n"
}

// Git runs git in root with an identity and no user configuration (no
// signing, no hooks), and returns its output.
func Git(t testing.TB, root string, args ...string) string {
	t.Helper()
	base := []string{"-c", "user.name=Shop Dev", "-c", "user.email=dev@example.com", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}
	command := exec.Command("git", append(base, args...)...)
	command.Dir = root
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Commit commits every change in root with message.
func Commit(t testing.TB, root, message string) {
	t.Helper()
	Git(t, root, "add", "--all")
	Git(t, root, "commit", "--quiet", "-m", message)
}

// Write writes a file of the repository.
func Write(t testing.TB, root, name, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Replace replaces old with replacement in a file of the repository.
func Replace(t testing.TB, root, name, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%s does not contain %q", name, old)
	}
	Write(t, root, name, strings.Replace(string(data), old, replacement, 1))
}

// Line is the 1-based line of the first line of a file that contains text.
func Line(t testing.TB, root, name, text string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	for number, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, text) {
			return number + 1
		}
	}
	t.Fatalf("%s has no line with %q", name, text)
	return 0
}
