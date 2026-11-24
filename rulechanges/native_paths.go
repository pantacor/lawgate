package rulechanges

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// The compiler opens a path as written: the operating system resolves it
// one element at a time, so in "link/../x.h" the ".." leaves the link's
// target, not the directory holding the link. A path cleaned lexically
// first ("x.h") would hide the link and name a file the compiler never
// reads. lawgate therefore walks each path element by element, refusing
// the first symbolic link it meets, and never cleans a path before that.

// absolute is the repository path of an absolute name, walked from "/".
func (scan *nativeScan) absolute(name string) (string, error) {
	return scan.reachable("/", name)
}

// reachable is the repository path of name walked from base, a
// repository directory, element by element. A symbolic link on the
// way is refused: Git lists a linked directory as one path, so the file
// read through it would never be protected. A walk that ends outside the
// repository is refused too, unless the owner approved the directory.
func (scan *nativeScan) reachable(base, name string) (string, error) {
	current := scan.located(base)
	if link, linked := scan.linkAt(current); linked {
		return "", linkedError(link)
	}
	for _, element := range strings.Split(name, "/") {
		current = scan.located(path.Join(current, element))
		if link, linked := scan.linkAt(current); linked {
			return "", linkedError(link)
		}
	}
	if outsideRepository(current) {
		return "", scan.outside(scan.absolutePath(current))
	}
	return current, nil
}

// located is a walked position in its one spelling: a repository path
// when it is inside the repository, so a walk that leaves and comes back
// in ("../repo/link") is still checked for links, or else an absolute
// path. With no known root it stays as joined, relative to the root.
func (scan *nativeScan) located(position string) string {
	root := filepath.ToSlash(scan.tree.root)
	if root == "" {
		return position
	}
	absolute := position
	if !path.IsAbs(position) {
		absolute = path.Join(root, position)
	}
	return repositoryDirectory(root, absolute)
}

// linkAt is the first symbolic link on the way down to a repository
// position; a position outside the repository has none lawgate knows.
func (scan *nativeScan) linkAt(position string) (string, bool) {
	if outsideRepository(position) {
		return "", false
	}
	return scan.tree.linkOn(position)
}

func outsideRepository(position string) bool {
	return path.IsAbs(position) || !fs.ValidPath(position)
}

// absolutePath is a position outside the repository as an absolute path,
// which outside_includes entries are matched against.
func (scan *nativeScan) absolutePath(position string) string {
	if path.IsAbs(position) {
		return position
	}
	return path.Join(filepath.ToSlash(scan.tree.root), position)
}

func linkedError(link string) error {
	return fmt.Errorf("reaches the symbolic link %s: lawgate refuses symlinked inputs", link)
}
