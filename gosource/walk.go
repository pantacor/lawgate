// Package gosource discovers Go modules and the source files in their builds.
package gosource

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

type scope int

const (
	repository scope = iota
	module
)

// CanonicalRoot resolves a repository root while leaving its file paths
// available for the caller to check separately.
func CanonicalRoot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

// Modules returns paths relative to root and reports any directories it could
// not scan. The paths discovered elsewhere are still returned.
func Modules(root string) ([]string, error) {
	root, err := CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	var modules []string
	err = walk(root, repository, func(file string) {
		if filepath.Base(file) == "go.mod" {
			dir, _ := filepath.Rel(root, filepath.Dir(file))
			modules = append(modules, filepath.ToSlash(dir))
		}
	})
	return modules, err
}

// Files returns the physical Go files eligible for selection in this module.
// The Go tool selects the active subset from these candidates.
func Files(root string) (map[string]bool, error) {
	root, err := CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	files := make(map[string]bool)
	err = walk(root, module, func(file string) {
		if strings.HasSuffix(file, ".go") {
			files[file] = true
		}
	})
	return files, err
}

// Skipped reports whether a repository-relative file is under a directory
// that is excluded from ordinary package discovery.
func Skipped(name string) bool {
	return slices.ContainsFunc(strings.Split(path.Dir(filepath.ToSlash(name)), "/"), skippedDirectory)
}

func skippedDirectory(name string) bool {
	switch name {
	case "vendor", "testdata", "node_modules":
		return true
	}
	hidden := strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
	return hidden && name != "."
}

// Sensitive reports whether a non-source file has a credential-like path.
// Source files and Go manifests are build inputs regardless of the path's spelling.
func Sensitive(name string) bool {
	if sourceFile(path.Base(filepath.ToSlash(name))) {
		return false
	}
	for _, component := range strings.Split(strings.ToLower(filepath.ToSlash(name)), "/") {
		if strings.HasPrefix(component, ".env") || strings.Contains(component, "secret") {
			return true
		}
	}
	return false
}

func sourceFile(name string) bool {
	switch filepath.Ext(name) {
	case ".js", ".mjs", ".cjs", ".ts":
		return true
	}
	return goSource(name)
}

func goSource(name string) bool {
	return name == "go.mod" || strings.HasSuffix(name, ".go")
}

func walk(root string, level scope, visit func(string)) error {
	walker := &treeWalker{root: root, level: level, visit: visit}
	err := filepath.WalkDir(root, walker.step)
	return errors.Join(walker.problems, err)
}

// treeWalker visits the regular files of one walk, skipping excluded
// directories and other modules, and collects the symbolic links it
// refuses to scan.
type treeWalker struct {
	root     string
	level    scope
	visit    func(string)
	problems error
}

func (walker *treeWalker) step(file string, entry fs.DirEntry, err error) error {
	if err != nil {
		return err
	}
	if file == walker.root {
		return nil
	}
	if entry.IsDir() {
		return walker.enter(file, entry)
	}
	if goSource(entry.Name()) && entry.Type()&fs.ModeSymlink != 0 {
		walker.problems = errors.Join(walker.problems, fmt.Errorf("%s was not scanned because it is a symbolic link", file))
		return nil
	}
	if entry.Type().IsRegular() {
		walker.visit(file)
	}
	return nil
}

func (walker *treeWalker) enter(file string, entry fs.DirEntry) error {
	if skippedDirectory(entry.Name()) {
		return fs.SkipDir
	}
	return enterDir(file, walker.level)
}

func enterDir(dir string, level scope) error {
	if level == repository {
		return nil
	}
	_, err := os.Lstat(filepath.Join(dir, "go.mod"))
	switch {
	case err == nil:
		return fs.SkipDir
	case os.IsNotExist(err):
		return nil
	default:
		return err
	}
}
