package rulechanges

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

type enforcers struct {
	tests    map[string][]string
	modules  map[string][]string
	required map[string][]string
	scripts  []string
	// nodeModules are the Node modules lawgate.json declares, whose named
	// enforcers must each bind to a test file ownership can find.
	nodeModules []inventory.Module
}

func newEnforcers() *enforcers {
	return &enforcers{tests: make(map[string][]string), modules: make(map[string][]string), required: make(map[string][]string)}
}

func (enforcers *enforcers) add(name string, content []byte, rules map[string]bool) {
	refs := lawsdoc.Enforcers(string(content))
	enforcers.tests[name] = refs.Tests
	for _, reference := range refs.Scripts {
		script, inside := scriptPath(name, reference)
		if inside {
			rules[script] = true
			enforcers.scripts = append(enforcers.scripts, script)
		}
	}
}

// refuseLinkedScripts reports each script enforcer reached through a
// symbolic link. The check refuses to run one, so ownership refuses it too
// rather than protect the link and leave the file it points to unowned.
func (enforcers *enforcers) refuseLinkedScripts(tree sourceTree, result *classification) {
	for _, script := range enforcers.scripts {
		if link, linked := tree.linkOn(script); linked {
			result.protect(script, fmt.Errorf("script enforcer is reached through the symbolic link %s: lawgate refuses symlinked script enforcers", link))
		}
	}
}

// scriptPath resolves a script enforcer relative to the LAWS.md that names
// it, and reports whether the result stays inside the repository.
func scriptPath(laws, reference string) (string, bool) {
	script := path.Join(path.Dir(laws), reference)
	inside := !path.IsAbs(reference) && fs.ValidPath(script) && script != "."
	return script, inside
}

func (enforcers *enforcers) namesFor(file string) map[string]bool {
	names := enforcers.documentNames(file)
	for root, required := range enforcers.required {
		if !inModule(file, root) {
			continue
		}
		recordRequiredNames(names, required)
	}
	return names
}

// recordRequiredNames adds required test references as written: a Go
// reference is resolved like a LAWS.md one (enforcingTests), so a qualified
// name selects one package's test, not every test with its base name.
func recordRequiredNames(names map[string]bool, required []string) {
	for _, name := range required {
		names[name] = true
	}
}

// allNames is every test reference of every document and module.
func (enforcers *enforcers) allNames() map[string]bool {
	names := make(map[string]bool)
	for _, tests := range enforcers.tests {
		recordRequiredNames(names, tests)
	}
	for _, required := range enforcers.required {
		recordRequiredNames(names, required)
	}
	return names
}

func inModule(file, root string) bool {
	return root == "." || strings.HasPrefix(file, root+"/")
}

func (enforcers *enforcers) documentNames(file string) map[string]bool {
	names := make(map[string]bool)
	for document, tests := range enforcers.tests {
		if !enforcers.inDocumentScope(document, file) {
			continue
		}
		for _, test := range tests {
			names[test] = true
		}
	}
	return names
}

// inDocumentScope reports whether a document's test references govern file:
// files under the document's own directory, and under every module that
// lists it in lawgate.json as its laws or shared laws.
func (enforcers *enforcers) inDocumentScope(document, file string) bool {
	if inModule(file, path.Dir(document)) {
		return true
	}
	for _, root := range enforcers.modules[document] {
		if inModule(file, root) {
			return true
		}
	}
	return false
}
