package rulechanges

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// embedSet is what a Go file's //go:embed directives name. When they cannot
// be evaluated, the set fails closed: it names every file of the package
// tree.
type embedSet struct {
	patterns []string
	all      bool
}

// includes reports whether the set names relative, a path inside the
// file's directory, or one of its parent directories (an embedded directory
// brings its whole tree).
func (set embedSet) includes(relative string) bool {
	if set.all {
		return true
	}
	for candidate := relative; candidate != "."; candidate = path.Dir(candidate) {
		if set.matches(candidate) {
			return true
		}
	}
	return false
}

func (set embedSet) matches(name string) bool {
	for _, pattern := range set.patterns {
		matched, err := path.Match(pattern, name)
		if matched || err != nil {
			return true // A pattern Go would reject protects everything.
		}
	}
	return false
}

// readEmbeds reads a file's //go:embed patterns with go/build, which parses
// them exactly as the go tool does (quoted names with spaces included). The
// "all:" prefix only changes which hidden files match, so it is dropped.
// go/build skips a directive it cannot parse, so a directive line it has no
// pattern for (directives holds them all) fails closed.
func readEmbeds(name string, content []byte, directives map[int]bool) (embedSet, error) {
	if len(directives) == 0 {
		return embedSet{}, nil
	}
	pkg, err := singleFileContext(name, content).ImportDir(path.Dir(name), 0)
	if err == nil {
		err = unreadDirectives(pkg, directives)
	}
	if err != nil {
		return embedSet{all: true}, fmt.Errorf("%w: //go:embed: %v", errUnparsable, err)
	}
	var set embedSet
	for _, pattern := range embedPatternsOf(pkg) {
		set.patterns = append(set.patterns, strings.TrimPrefix(pattern, "all:"))
	}
	return set, nil
}

// unreadDirectives reports the first directive line go/build read no
// pattern from.
func unreadDirectives(pkg *build.Package, directives map[int]bool) error {
	read := patternLines(pkg)
	for line := range directives {
		if !read[line] {
			return fmt.Errorf("line %d cannot be evaluated", line)
		}
	}
	return nil
}

func patternLines(pkg *build.Package) map[int]bool {
	lines := make(map[int]bool)
	positions := []map[string][]token.Position{pkg.EmbedPatternPos, pkg.TestEmbedPatternPos, pkg.XTestEmbedPatternPos}
	for _, byPattern := range positions {
		addPositionLines(lines, byPattern)
	}
	return lines
}

func addPositionLines(lines map[int]bool, byPattern map[string][]token.Position) {
	for _, positions := range byPattern {
		for _, position := range positions {
			lines[position.Line] = true
		}
	}
}

// directiveLines are the lines of a file's //go:embed directives.
func directiveLines(fset *token.FileSet, syntax *ast.File) map[int]bool {
	lines := make(map[int]bool)
	for _, group := range syntax.Comments {
		addDirectiveLines(lines, fset, group)
	}
	return lines
}

func addDirectiveLines(lines map[int]bool, fset *token.FileSet, group *ast.CommentGroup) {
	for _, comment := range group.List {
		if embedDirective(comment.Text) {
			lines[fset.Position(comment.Slash).Line] = true
		}
	}
}

func embedDirective(text string) bool {
	arguments, ok := strings.CutPrefix(text, "//go:embed")
	separated := arguments == "" || arguments[0] == ' ' || arguments[0] == '\t'
	return ok && separated
}

func embedPatternsOf(pkg *build.Package) []string {
	patterns := append([]string{}, pkg.EmbedPatterns...)
	patterns = append(patterns, pkg.TestEmbedPatterns...)
	return append(patterns, pkg.XTestEmbedPatterns...)
}

// singleFileContext is a build context whose directory holds only the one
// file, read from memory (the worktree or a history blob), whatever its
// build constraints: a variant the current platform skips is still a
// variant some build compiles.
func singleFileContext(name string, content []byte) *build.Context {
	context := build.Default
	context.UseAllFiles = true
	context.CgoEnabled = true
	context.GOPATH = ""
	context.JoinPath = path.Join
	context.IsDir = func(string) bool { return true }
	context.HasSubdir = func(string, string) (string, bool) { return "", false }
	context.ReadDir = func(string) ([]fs.FileInfo, error) {
		return []fs.FileInfo{memoryFile{path.Base(name), int64(len(content))}}, nil
	}
	context.OpenFile = func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content)), nil
	}
	return &context
}

// memoryFile describes the one regular file of a singleFileContext.
type memoryFile struct {
	name string
	size int64
}

func (file memoryFile) Name() string       { return file.name }
func (file memoryFile) Size() int64        { return file.size }
func (file memoryFile) Mode() fs.FileMode  { return 0o644 }
func (file memoryFile) ModTime() time.Time { return time.Time{} }
func (file memoryFile) IsDir() bool        { return false }
func (file memoryFile) Sys() any           { return nil }
