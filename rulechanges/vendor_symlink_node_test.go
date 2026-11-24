package rulechanges

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const outsideRefusal = "outside the repository: lawgate cannot protect it"

func memberVendorSources() map[string]string {
	return map[string]string{
		"go.work":                              "go 1.25\nuse ./m\n",
		"m/go.mod":                             "module example.org/m\ngo 1.25\nreplace example.org/alias => ../../outside\n",
		"m/vendor/modules.txt":                 "# example.org/alias v0.0.0 => ../../outside\n## explicit\nexample.org/alias/pkg\n",
		"m/vendor/example.org/alias/pkg/p.go":  "package pkg\n",
		"m/LAWS.md":                            "## m-law\n- Enforced by: test `TestMLaw`",
		"m/law_test.go":                        "package m\nimport _ \"example.org/alias/pkg\"\nfunc TestMLaw(t *testing.T) {}",
		"m/unrelated/u.go":                     "package unrelated\n",
		"m/vendor/example.org/alias/pkg/x.txt": "",
	}
}

func requireRefusal(t *testing.T, files map[string]string, want string) {
	t.Helper()
	_, err := classifyFixture(t, files)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want an error containing %q, got %v", want, err)
	}
}

// TestStaleMemberVendorKeepsOutsideReplaceRefused: in
// workspace mode Go reads only the workspace's vendor/, so a member
// module's vendor/modules.txt must not waive an outside replacement.
func TestStaleMemberVendorKeepsOutsideReplaceRefused(t *testing.T) {
	requireRefusal(t, memberVendorSources(), outsideRefusal)
}

// TestVendorWaiverFollowsGoDefaults: Go vendors by default
// only with a go directive of 1.14 or later and a modules.txt written for
// the same mode; otherwise it builds the outside replacement.
func TestVendorWaiverFollowsGoDefaults(t *testing.T) {
	files := memberVendorSources()
	delete(files, "go.work")
	requireRuleFiles(t, files, []string{"m/vendor/example.org/alias/pkg/p.go"}, nil)

	files["m/go.mod"] = strings.Replace(files["m/go.mod"], "go 1.25\n", "", 1)
	requireRefusal(t, files, outsideRefusal)

	files = memberVendorSources()
	delete(files, "go.work")
	files["m/vendor/modules.txt"] = "## workspace\n" + files["m/vendor/modules.txt"]
	requireRefusal(t, files, outsideRefusal)
}

// TestSymlinkedVendorIsRefused: Git lists a symlinked vendor/
// as one path, so its files never reach ownership; the check refuses it.
func TestSymlinkedVendorIsRefused(t *testing.T) {
	for _, link := range []string{"vendor", "vendor/modules.txt"} {
		files := blankImportSources()
		files[link] = ""
		tree := mapTree(files)
		tree.links = map[string]bool{link: true}
		result := classify(tree)
		err := result.err()
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("symlinked %s: %v", link, err)
		}
		if !result.rules[link] {
			t.Fatalf("symlinked %s is unprotected", link)
		}
	}
}

// TestSymlinkedCommandIsRefused: a configured command reached
// through a symbolic link would leave the file it runs unowned.
func TestSymlinkedCommandIsRefused(t *testing.T) {
	for _, link := range []string{"web/scripts/check-machines.mjs", "web/scripts"} {
		tree := mapTree(nodeCommandSources())
		tree.links = map[string]bool{link: true}
		result := classify(tree)
		err := result.err()
		if err == nil || !strings.Contains(err.Error(), "symbolic link "+link) {
			t.Fatalf("symlinked command via %s: %v", link, err)
		}
	}
}

// TestSymlinkedGateFileIsRefused: the same for gate_files.
func TestSymlinkedGateFileIsRefused(t *testing.T) {
	files := nodeCommandSources()
	files["lawgate.json"] = strings.Replace(files["lawgate.json"], `{"modules"`, `{"gate_files":["tools/gen.sh"],"modules"`, 1)
	files["tools/gen.sh"] = ""
	tree := mapTree(files)
	tree.links = map[string]bool{"tools": true}
	err := classify(tree).err()
	if err == nil || !strings.Contains(err.Error(), "symbolic link tools") {
		t.Fatalf("symlinked gate file: %v", err)
	}
}

func nodeDirectorySources() map[string]string {
	files := nodeCommandSources()
	files["web/scripts/check-law.mjs"] = "const help = require('./lib')\nconst idx = require('./plain')\n"
	files["web/scripts/lib/package.json"] = `{"main": "src/entry.js"}`
	files["web/scripts/lib/src/entry.js"] = "module.exports = require('./dep')\n"
	files["web/scripts/lib/src/dep.js"] = "module.exports = {}\n"
	files["web/scripts/lib/index.js"] = "module.exports = {}\n"
	files["web/scripts/plain/index.js"] = "module.exports = {}\n"
	return files
}

// TestNodeDirectoryImportsResolveLikeNode: require("./dir")
// loads dir/package.json "main" (then dir/index.js), so the manifest and
// the file it selects are protected, and the selected file's imports too.
func TestNodeDirectoryImportsResolveLikeNode(t *testing.T) {
	wanted := []string{
		"web/scripts/lib/package.json",
		"web/scripts/lib/src/entry.js",
		"web/scripts/lib/src/dep.js",
		"web/scripts/plain/index.js",
	}
	requireRuleFiles(t, nodeDirectorySources(), wanted, []string{"web/scripts/lib/index.js"})
}

// TestUnresolvedNodeImportIsReported: a relative import that
// names no repository file is reported, never silently dropped.
func TestUnresolvedNodeImportIsReported(t *testing.T) {
	files := nodeCommandSources()
	files["web/scripts/check-law.mjs"] = "const help = require('./missing-helper')\n"
	requireRefusal(t, files, "./missing-helper")
}

// TestSeededNodeFileInUnderscoreDirectoryIsFollowed: a script
// enforcer under _scripts/ is followed through its imports like any other.
func TestSeededNodeFileInUnderscoreDirectoryIsFollowed(t *testing.T) {
	files := map[string]string{
		"lawgate.json":              `{"modules":[{"path":"web","language":"node","laws":"web/LAWS.md","machines":false}]}`,
		"web/package.json":          "{}",
		"web/LAWS.md":               "## web-law\n- Enforced by: script `_scripts/check.mjs`",
		"web/_scripts/check.mjs":    "import { help } from './helper.mjs'\n",
		"web/_scripts/helper.mjs":   "export const help = 1\n",
		"web/_scripts/unrelated.js": "export {}\n",
	}
	requireRuleFiles(t, files, []string{"web/_scripts/helper.mjs"}, []string{"web/_scripts/unrelated.js"})
}

// TestCheckOwnersThroughSymlinkedRoot: check-codeowners works
// on a root reached through a symbolic link, like rule-files and check.
func TestCheckOwnersThroughSymlinkedRoot(t *testing.T) {
	fixture := newGitFixture(t)
	fixture.write(map[string]string{
		"LAWS.md":            "## law\n",
		".github/CODEOWNERS": "* " + requiredOwner + "\n",
	})
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(fixture.root, link); err != nil {
		t.Fatal(err)
	}
	if err := CheckOwners(link, []string{"LAWS.md"}, requiredOwner, "github"); err != nil {
		t.Fatal(err)
	}
}

// TestRawStringManifestPaths: go.mod and go.work accept Go raw
// strings, and a "//" inside one is part of the path, not a comment.
func TestRawStringManifestPaths(t *testing.T) {
	data := "module example.org/project\nreplace example.org/alias => `./lo//cal` // note\nuse (\n\t`./work dir`\n)\n"
	manifest, err := parseManifest("go.mod", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	directories := []string{}
	for _, replace := range manifest.replacements {
		directories = append(directories, replace.directory)
	}
	raw := slices.Contains(directories, "lo/cal") && slices.Contains(manifest.uses, "work dir")
	if !raw {
		t.Fatalf("raw-string directories = %v, uses = %v", directories, manifest.uses)
	}
}
