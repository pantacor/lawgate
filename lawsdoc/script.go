package lawsdoc

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScriptPath resolves the script from the named document and keeps it inside
// the repository even when the path contains symlinks.
func ScriptPath(root, doc, name string) (string, error) {
	resolved, err := documentFile(root, doc, name)
	if err != nil {
		return "", fmt.Errorf("is enforced by script %q, which %w", name, err)
	}
	return resolved, nil
}

func documentFile(root, doc, name string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("cannot be checked without a repo root")
	}
	relative, err := documentReference(doc, name)
	if err != nil {
		return "", err
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	return resolveDocumentFile(base, relative)
}

func documentReference(doc, name string) (string, error) {
	relative := filepath.Join(filepath.Dir(doc), name)
	absolute := filepath.IsAbs(name) || filepath.VolumeName(name) != ""
	inside := !absolute && filepath.IsLocal(relative)
	if name == "" || !inside {
		return "", fmt.Errorf("is not a path inside the repo")
	}
	return relative, nil
}

func resolveDocumentFile(base, name string) (string, error) {
	resolved, err := filepath.EvalSymlinks(filepath.Join(base, name))
	if err != nil {
		return "", fmt.Errorf("does not exist or cannot be resolved: %w", err)
	}
	relative, err := filepath.Rel(base, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("is not a path inside the repo")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("is not a regular file")
	}
	return resolved, nil
}
