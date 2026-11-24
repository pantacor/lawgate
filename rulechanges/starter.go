package rulechanges

import (
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

// StarterRuleFiles discovers rules with the configuration and paths init will write.
func StarterRuleFiles(root string, config []byte, outputs []string) ([]string, error) {
	root, err := gosource.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	paths, links, err := starterPaths(root)
	if err != nil {
		return nil, err
	}
	read := func(name string) ([]byte, error) {
		if name == inventory.Filename {
			return config, nil
		}
		return readWorktreeFile(root, name)
	}
	tree := sourceTree{root: root, paths: slices.Concat(paths, outputs), links: links, read: read}
	result := classify(tree)
	return sortedPaths(result.rules), result.err()
}

// starterPaths lists a directory that is not a Git worktree yet. Only .git
// and node_modules are left out: vendor/, "_" and hidden directories, and
// testdata hold files the classifier protects (.github, vendored packages).
func starterPaths(root string) ([]string, map[string]bool, error) {
	if _, err := gitCommand(root, "rev-parse", "--show-toplevel"); err == nil {
		return trackedAndUntracked(root)
	}
	var paths []string
	links := make(map[string]bool)
	err := filepath.WalkDir(root, func(full string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipStarterDir(full, root, entry.Name())
		}
		relative, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		paths = append(paths, name)
		links[name] = entry.Type()&fs.ModeSymlink != 0
		return nil
	})
	return paths, links, err
}

func skipStarterDir(full, root, name string) error {
	unlisted := name == ".git" || name == "node_modules"
	if full != root && unlisted {
		return filepath.SkipDir
	}
	return nil
}
