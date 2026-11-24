package lawsdoc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// CheckModule checks the LAWS.md at a Go module's root against in, with the
// module's compiled tests discovered by DiscoverTests (in.Tests is replaced).
// A missing LAWS.md is a problem, not an error: the module's laws are not
// documented.
func CheckModule(moduleRoot string, in Inputs) ([]string, error) {
	path := filepath.Join(moduleRoot, FileName)
	doc, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{fmt.Sprintf("%s does not exist: the module's laws are not documented", path)}, nil
	}
	if err != nil {
		return nil, err
	}
	in.Tests, err = DiscoverTests(moduleRoot)
	if err != nil {
		return nil, err
	}
	if in.RepoRoot != "" {
		in.DocPath, err = filepath.Rel(in.RepoRoot, path)
		if err != nil {
			return nil, err
		}
	}
	return Check(string(doc), in), nil
}

// FindRepoRoot returns the nearest directory at or above dir that holds a
// .git entry, or an error when there is none.
func FindRepoRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("no .git at or above %s", dir)
		}
		abs = parent
	}
}
