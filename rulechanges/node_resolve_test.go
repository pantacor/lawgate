package rulechanges

import (
	"strings"
	"testing"
)

func nodeHelperSources(law string) map[string]string {
	files := nodeCommandSources()
	files["web/scripts/check-law.mjs"] = law
	return files
}

// TestRequireLoadsWhatNodeLoads: require("./helper") tries the exact name,
// then .js, .json and .node, never .cjs or .mjs, and loads the first match.
func TestRequireLoadsWhatNodeLoads(t *testing.T) {
	files := nodeHelperSources("const require = createRequire(import.meta.url)\nrequire('./helper')\n")
	files["web/scripts/helper.cjs"] = "module.exports = {}\n"
	files["web/scripts/helper.json"] = "{}\n"
	requireRuleFiles(t, files, []string{"web/scripts/helper.json"}, []string{"web/scripts/helper.cjs"})

	files["web/scripts/helper"] = "module.exports = {}\n"
	requireRuleFiles(t, files, []string{"web/scripts/helper"}, []string{"web/scripts/helper.json"})
}

// TestRequireDirectoryLoadsWhatNodeLoads: a directory's package.json
// "main" is tried as a file with require's extensions, then as a directory,
// then the directory's own index.js, index.json and index.node.
func TestRequireDirectoryLoadsWhatNodeLoads(t *testing.T) {
	files := nodeHelperSources("require('./lib')\n")
	files["web/scripts/lib/package.json"] = `{"main": "entry"}`
	files["web/scripts/lib/entry.mjs"] = "export {}\n"
	files["web/scripts/lib/entry.json"] = "{}\n"
	files["web/scripts/lib/index.js"] = "module.exports = {}\n"
	requireRuleFiles(t, files, []string{"web/scripts/lib/entry.json"}, []string{"web/scripts/lib/entry.mjs", "web/scripts/lib/index.js"})
}

// TestImportLoadsTheExactFile: an ES module import searches no extension
// and no directory, so an import Node cannot load is reported.
func TestImportLoadsTheExactFile(t *testing.T) {
	files := nodeHelperSources("import { help } from './helper.mjs'\n")
	files["web/scripts/helper.mjs"] = "export const help = 1\n"
	files["web/scripts/helper.mjs.js"] = "module.exports = {}\n"
	requireRuleFiles(t, files, []string{"web/scripts/helper.mjs"}, []string{"web/scripts/helper.mjs.js"})

	files["web/scripts/check-law.mjs"] = "import { help } from './helper'\n"
	requireRefusal(t, files, "loads exactly the file it names")
}

// TestTypeScriptImportProtectsEveryCandidate: a loader decides how a
// TypeScript source resolves, so every file it could pick is protected,
// the .ts source an import of "./x.js" stands for included.
func TestTypeScriptImportProtectsEveryCandidate(t *testing.T) {
	files := nodeHelperSources("import './run.ts'\n")
	files["web/scripts/run.ts"] = "import { a } from './a.js'\nconst b = require('./b')\n"
	files["web/scripts/a.js"] = "export const a = 1\n"
	files["web/scripts/a.ts"] = "export const a = 1\n"
	files["web/scripts/b.ts"] = "export const b = 1\n"
	files["web/scripts/b.json"] = "{}\n"
	wanted := []string{"web/scripts/a.js", "web/scripts/a.ts", "web/scripts/b.ts", "web/scripts/b.json"}
	requireRuleFiles(t, files, wanted, nil)
}

// TestDirectorySpecifiersResolve: ".", "..", "./" and "../" load a
// directory, so its index is protected rather than the import dropped.
func TestDirectorySpecifiersResolve(t *testing.T) {
	directories := map[string]string{"..": "web", "../": "web", ".": "web/scripts", "./": "web/scripts"}
	for specifier, directory := range directories {
		files := nodeHelperSources("require('" + specifier + "')\n")
		files[directory+"/index.js"] = "module.exports = {}\n"
		requireRuleFiles(t, files, []string{directory + "/index.js"}, nil)
	}
}

// TestDirectorySpecifierSkipsTheFile: "./lib/" names the directory only,
// even when lib.js exists beside it.
func TestDirectorySpecifierSkipsTheFile(t *testing.T) {
	files := nodeHelperSources("require('./lib/')\n")
	files["web/scripts/lib.js"] = "module.exports = {}\n"
	files["web/scripts/lib/index.js"] = "module.exports = {}\n"
	requireRuleFiles(t, files, []string{"web/scripts/lib/index.js"}, []string{"web/scripts/lib.js"})
}

// TestImportOutsideTheRepositoryIsRefused: a ".." that leaves the
// repository names a file ownership cannot protect.
func TestImportOutsideTheRepositoryIsRefused(t *testing.T) {
	requireRefusal(t, nodeHelperSources("require('../../../outside')\n"), outsideRefusal)
}

// TestLinkedPackageMainIsRefused: a package.json "main" reached through
// a symbolic link is refused, never replaced by the directory's index.
func TestLinkedPackageMainIsRefused(t *testing.T) {
	files := nodeHelperSources("require('./pkg')\n")
	files["web/scripts/pkg/package.json"] = `{"main": "./linked/entry"}`
	files["web/scripts/pkg/linked"] = ""
	files["web/scripts/pkg/index.js"] = "module.exports = {}\n"
	tree := mapTree(files)
	tree.links = map[string]bool{"web/scripts/pkg/linked": true}
	err := classify(tree).err()
	if err == nil || !strings.Contains(err.Error(), "symbolic link web/scripts/pkg/linked") {
		t.Fatalf("symlinked main: %v", err)
	}
}

// TestPackageMainOutsideTheRepositoryIsRefused: the same for a "main"
// that leaves the repository.
func TestPackageMainOutsideTheRepositoryIsRefused(t *testing.T) {
	files := nodeHelperSources("require('./pkg')\n")
	files["web/scripts/pkg/package.json"] = `{"main": "../../../../outside"}`
	files["web/scripts/pkg/index.js"] = "module.exports = {}\n"
	requireRefusal(t, files, outsideRefusal)
}
