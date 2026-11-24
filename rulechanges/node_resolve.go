package rulechanges

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Node resolves a relative specifier differently per module system, and a
// helper is protected only if it is the file Node loads. CommonJS require
// tries the exact name, then each extension, then the directory (its
// package.json "main", then its index file), and loads the first match.
// ESM import loads the exact file and nothing else. A TypeScript source is
// run through a loader whose rules lawgate cannot know, as is a JSX
// source, so there every candidate either system or a loader could pick is
// protected.
var (
	requireExtensions = []string{"", ".js", ".json", ".node"}
	requireIndexes    = []string{"index.js", "index.json", "index.node"}
	loaderExtensions  = append([]string{"", ".json", ".node"}, nodeSourceExtensions...)
	loaderIndexes     = indexFiles(loaderExtensions[1:])
)

// nodeSourceExtensions are the sources Node or a loader runs as JavaScript
// or TypeScript: the one list the graph follows and the loader resolves.
var nodeSourceExtensions = []string{".js", ".cjs", ".mjs", ".jsx", ".ts", ".cts", ".mts", ".tsx"}

func indexFiles(extensions []string) []string {
	indexes := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		indexes = append(indexes, "index"+extension)
	}
	return indexes
}

func nodeSource(name string) bool {
	return slices.Contains(nodeSourceExtensions, path.Ext(name))
}

// typeScriptTwins maps the extension an import names to the source a
// TypeScript loader substitutes for it ("./x.js" loads x.ts).
var typeScriptTwins = map[string]string{".js": ".ts", ".cjs": ".cts", ".mjs": ".mts", ".jsx": ".tsx"}

var errNoRepositoryFile = errors.New("loads no repository file")

type nodeSystem int

const (
	nodeCommonJS nodeSystem = iota
	nodeModule
	nodeLoader
)

// nodeLookup resolves specifiers for one module system. Under a loader,
// every candidate is kept; otherwise only the first, as Node does.
type nodeLookup struct {
	graph      *nodeGraph
	system     nodeSystem
	extensions []string
	indexes    []string
}

// lookupFor picks the resolver for a specifier written with syntax in file.
func (graph *nodeGraph) lookupFor(file, syntax string) nodeLookup {
	lookup := nodeLookup{graph: graph, system: nodeLoader, extensions: loaderExtensions, indexes: loaderIndexes}
	switch {
	case loaderSource(file):
		return lookup
	case strings.HasPrefix(syntax, "require"):
		lookup.system = nodeCommonJS
		lookup.extensions, lookup.indexes = requireExtensions, requireIndexes
	default:
		lookup.system = nodeModule
	}
	return lookup
}

// loaderSource is a file Node runs only through a loader: TypeScript, or
// JSX in either language.
func loaderSource(name string) bool {
	switch path.Ext(name) {
	case ".ts", ".cts", ".mts", ".tsx", ".jsx":
		return true
	}
	return false
}

// resolve returns every repository file loading specifier from directory
// reads: the loaded file and, for a directory, the package.json that chose
// it. A trailing slash, "." and ".." name a directory only, as in Node.
func (lookup nodeLookup) resolve(directory, specifier string) ([]string, error) {
	base := path.Join(directory, specifier)
	if err := lookup.checkReach(base); err != nil {
		return nil, err
	}
	directoryOnly := strings.HasSuffix(specifier, "/") || path.Base(specifier) == "." || path.Base(specifier) == ".."
	if lookup.system == nodeModule {
		return lookup.exactFile(nodeTarget{base: base, directoryOnly: directoryOnly})
	}
	var found []string
	if !directoryOnly {
		found = lookup.asFile(base)
	}
	if len(found) > 0 && lookup.system == nodeCommonJS {
		return found, nil
	}
	fromDirectory, err := lookup.asDirectory(base)
	found = append(found, fromDirectory...)
	if len(found) == 0 && err == nil {
		err = errNoRepositoryFile
	}
	return found, err
}

// checkReach refuses a path outside the repository or through a symbolic
// link: Git lists a linked directory as one path, so the file Node loads
// through it would never be protected.
func (lookup nodeLookup) checkReach(name string) error {
	if !fs.ValidPath(name) {
		return fmt.Errorf("%s is outside the repository: lawgate cannot protect it", name)
	}
	if link, linked := lookup.graph.tree.linkOn(name); linked {
		return fmt.Errorf("reaches the symbolic link %s: lawgate refuses symlinked inputs", link)
	}
	return nil
}

// nodeTarget is a resolved specifier, and whether it names a directory.
type nodeTarget struct {
	base          string
	directoryOnly bool
}

// exactFile is ESM resolution: the named file, never a directory.
func (lookup nodeLookup) exactFile(target nodeTarget) ([]string, error) {
	if target.directoryOnly || !lookup.graph.known[target.base] {
		return nil, fmt.Errorf("%w: an ES module import loads exactly the file it names", errNoRepositoryFile)
	}
	return []string{target.base}, nil
}

func (lookup nodeLookup) asFile(base string) []string {
	found := lookup.matches(base, lookup.extensions)
	if lookup.system != nodeLoader {
		return found
	}
	twin, ok := typeScriptTwins[path.Ext(base)]
	if ok {
		found = append(found, lookup.matches(strings.TrimSuffix(base, path.Ext(base))+twin, []string{""})...)
	}
	return found
}

func (lookup nodeLookup) asIndex(directory string) []string {
	candidates := make([]string, 0, len(lookup.indexes))
	for _, index := range lookup.indexes {
		candidates = append(candidates, path.Join(directory, index))
	}
	return lookup.matches("", candidates)
}

// matches are the known files base+suffix, in order: only the first for
// CommonJS, every one under a loader.
func (lookup nodeLookup) matches(base string, suffixes []string) []string {
	var found []string
	for _, suffix := range suffixes {
		if !lookup.graph.known[base+suffix] {
			continue
		}
		found = append(found, base+suffix)
		if lookup.system == nodeCommonJS {
			return found
		}
	}
	return found
}

// asDirectory loads a directory through its package.json "main", then its
// index file, as require does. "exports" applies only to package names,
// never to a relative directory, so it is not consulted.
func (lookup nodeLookup) asDirectory(base string) ([]string, error) {
	manifest := path.Join(base, "package.json")
	if !lookup.graph.known[manifest] {
		return lookup.asIndex(base), nil
	}
	targets := []string{manifest}
	main, err := lookup.graph.packageMain(manifest)
	if err != nil {
		return targets, err
	}
	selected, err := lookup.mainOf(base, main)
	if len(selected) == 0 && err == nil {
		err = fmt.Errorf("%s selects no repository file", manifest)
	}
	return append(targets, selected...), err
}

// mainOf is LOAD_AS_FILE(main), then LOAD_INDEX(main), then the
// directory's own index. A "main" outside the repository or through a
// symbolic link is refused, never replaced by the index.
func (lookup nodeLookup) mainOf(base, main string) ([]string, error) {
	if main == "" {
		return lookup.asIndex(base), nil
	}
	entry := path.Join(base, main)
	if path.IsAbs(main) {
		return nil, fmt.Errorf("\"main\" %q is outside the repository: lawgate cannot protect it", main)
	}
	if err := lookup.checkReach(entry); err != nil {
		return nil, fmt.Errorf("\"main\" %q: %w", main, err)
	}
	return lookup.firstOrEvery(lookup.asFile(entry), lookup.asIndex(entry), lookup.asIndex(base)), nil
}

// firstOrEvery is the first non-empty choice, as require falls through
// them, or every choice under a loader.
func (lookup nodeLookup) firstOrEvery(choices ...[]string) []string {
	var found []string
	for _, choice := range choices {
		found = append(found, choice...)
		if len(found) > 0 && lookup.system == nodeCommonJS {
			return found
		}
	}
	return found
}

func (graph *nodeGraph) packageMain(manifest string) (string, error) {
	data, err := graph.tree.read(manifest)
	if err != nil {
		return "", err
	}
	var fields struct {
		Main string `json:"main"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return "", fmt.Errorf("%s: %w", manifest, err)
	}
	return fields.Main, nil
}
