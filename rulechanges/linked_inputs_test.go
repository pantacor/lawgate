package rulechanges

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func classifyLinked(files map[string]string, root string, links ...string) *classification {
	tree := mapTree(files)
	tree.root = root
	tree.links = make(map[string]bool)
	for _, link := range links {
		tree.links[link] = true
	}
	read := tree.read
	// A worktree read follows no symbolic link (readWorktreeFile).
	tree.read = func(name string) ([]byte, error) {
		if _, linked := tree.linkOn(name); linked {
			return nil, fmt.Errorf("rule input %s is a symlink", name)
		}
		return read(name)
	}
	return classify(tree)
}

func requireLinkRefusal(t *testing.T, result *classification, link string) {
	t.Helper()
	err := result.err()
	if err == nil || !strings.Contains(err.Error(), "symbolic link "+link) {
		t.Fatalf("symlinked %s was not refused: %v", link, err)
	}
}

// TestLinkedConfigInputsAreRefused: the readability baseline, CODEOWNERS
// and a law document reached through a symbolic link are refused, since
// protecting the link would leave the file the gate reads unowned.
func TestLinkedConfigInputsAreRefused(t *testing.T) {
	files := nodeCommandSources()
	files["lawgate.json"] = strings.Replace(files["lawgate.json"], `{"modules"`, `{"readability":{"baseline":".lawgate-readability"},"modules"`, 1)
	files[".lawgate-readability"] = ""
	files[".github/CODEOWNERS"] = ""
	for _, link := range []string{".lawgate-readability", ".github", "web"} {
		requireLinkRefusal(t, classifyLinked(files, "", link), link)
	}
}

func linkedVendorSources() map[string]string {
	files := blankImportSources()
	files["go.mod"] = "module example.org/project\ngo 1.25\nrequire example.org/dep v1.0.0\n"
	files["w/law_test.go"] = "package w\nimport _ \"example.org/dep/a\"\nimport _ \"example.org/dep/b\"\nfunc TestALaw(t *testing.T) {}"
	files["vendor"] = ""
	files["vendor/modules.txt"] = "# example.org/dep v1.0.0\n## explicit\nexample.org/dep/a\nexample.org/dep/b\n"
	files["vendor/example.org/dep/a/a.go"] = "package a\n"
	files["vendor/example.org/dep/b/b.go"] = "package b\n"
	return files
}

// TestLinkedVendorIsReportedOnce: the files beneath a symlinked vendor/
// are not resolved through it, so the link is the one problem reported.
func TestLinkedVendorIsReportedOnce(t *testing.T) {
	result := classifyLinked(linkedVendorSources(), "", "vendor")
	err := result.err()
	if err == nil || !strings.Contains(err.Error(), errLinkedVendor.Error()) {
		t.Fatalf("symlinked vendor/ was not refused: %v", err)
	}
	if len(result.problems) != 1 {
		t.Fatalf("want one problem for the symlinked vendor/, got %d: %v", len(result.problems), result.err())
	}
}

// TestSelectedWorkspaceLinkedVendorIsRefused: a command's GOWORK selects
// a workspace whose vendor/ is a symbolic link; the selection is the only
// place that workspace is known, so the link is refused there.
func TestSelectedWorkspaceLinkedVendorIsRefused(t *testing.T) {
	root := t.TempDir()
	files := commandBuildFiles()
	workspace := filepath.Join(root, "nested", "alternate.work")
	files["lawgate.json"] = commandConfig(fmt.Sprintf(`"env",%q,"go","run","-C","tools","./runner"`, "GOWORK="+workspace))
	files["LAWS.md"] = "# Laws"
	files["nested/alternate.work"] = "go 1.25\nuse (\n../\n../dep\n)\n"
	files["nested/vendor"] = ""
	result := classifyLinked(files, root, "nested/vendor")
	err := result.err()
	if err == nil || !strings.Contains(err.Error(), errLinkedVendor.Error()) {
		t.Fatalf("symlinked vendor/ of the selected workspace: %v", err)
	}
}
