package inventory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pantacor/lawgate/gosource"
)

// repositoryContext pins the Go tool's automatic workspace search to the
// repository that supplied the command, so a parent checkout cannot supply
// an unowned go.work.
func (command *GoCommand) repositoryContext() error {
	initial, err := gosource.CanonicalRoot(command.initialDir)
	if err != nil {
		return err
	}
	root, err := commandRepoRoot(initial)
	if err != nil || root == "" {
		return err
	}
	if err := checkGoInput(root, "-C", command.Dir); err != nil {
		return err
	}
	if err := checkGoInput(root, "-modfile", command.ModFile); err != nil {
		return err
	}
	if command.ModFile != "" {
		path := strings.TrimSuffix(command.ModFile, ".mod") + ".sum"
		if err := checkGoInput(root, "-modfile checksum", path); err != nil {
			return err
		}
	}
	return command.repositoryWorkspace(root)
}

func commandRepoRoot(dir string) (string, error) {
	for parent := dir; ; parent = filepath.Dir(parent) {
		_, err := os.Stat(filepath.Join(parent, Filename))
		if err == nil {
			return parent, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(parent) == parent {
			return "", nil
		}
	}
}

func (command *GoCommand) repositoryWorkspace(root string) error {
	if command.Workspace == "" {
		workspace, err := findGoWorkspace(root, command.Dir)
		if err != nil {
			return err
		}
		command.Workspace = workspace
	}
	if command.Workspace == "off" {
		return nil
	}
	if err := checkGoInput(root, "GOWORK", command.Workspace); err != nil {
		return err
	}
	return checkGoInput(root, "GOWORK checksum", command.Workspace+".sum")
}

func findGoWorkspace(root, dir string) (string, error) {
	for parent := dir; ; parent = filepath.Dir(parent) {
		name := filepath.Join(parent, "go.work")
		_, err := os.Lstat(name)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if parent == root {
			return "off", nil
		}
	}
}

func checkGoInput(root, flag, name string) error {
	if name == "" {
		return nil
	}
	filename, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, filename)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("Go input %s=%q is outside the repository", flag, name)
	}
	actual, err := filepath.EvalSymlinks(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // Go diagnoses missing required inputs and tolerates absent checksums.
	}
	if err != nil {
		return err
	}
	if actual != filename {
		return fmt.Errorf("Go input %s=%q resolves through a symlink", flag, name)
	}
	return nil
}
