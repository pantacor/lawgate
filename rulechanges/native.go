package rulechanges

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A reached package's C compiler reads more than the files of its own
// directory: every header its cgo preamble and its C sources include, in
// turn. Such a header can define a bound the enforcing test asserts, so
// each repository file the includes reach is protected. An include lawgate
// cannot resolve protects the package's whole directory tree instead, and
// one that may resolve where ownership cannot follow (outside the
// repository, through a symbolic link, through an unknown variable, a
// macro or pkg-config) is reported as well, so the owner decides.

var (
	// nativeDirective matches every include directive; its operand is
	// parsed apart so a computed one (#include MACRO) is seen, not skipped.
	nativeDirective = regexp.MustCompile(`(?m)^\s*#\s*(?:include_next|include|import)\b[ \t]*([^\r\n]*)`)
	includeOperand  = regexp.MustCompile(`^([<"])([^>"\r\n]+)[>"]`)
	cgoDirective    = regexp.MustCompile(`(?m)^\s*#cgo\s+([^:\r\n]*):(.*)$`)
)

var (
	errOutsideRepository = errors.New("is outside the repository: lawgate cannot protect it")
	errUnknownVariable   = errors.New("names a variable other than ${SRCDIR}: lawgate cannot resolve it")
	errUnsearchedInclude = errors.New("is in no repository directory and may be in an include directory lawgate cannot read")
	errPkgConfig         = errors.New("takes its compiler flags from pkg-config, outside the repository, at build time: lawgate cannot review them")
)

// cgoFlagDirectives are the #cgo flag sets that carry include directories.
var cgoFlagDirectives = map[string]bool{"CFLAGS": true, "CPPFLAGS": true, "CXXFLAGS": true, "FFLAGS": true}

// cgoPreamble is the comment above a file's import "C", which cgo compiles
// as C, or "" when the file does not use cgo.
func cgoPreamble(syntax *ast.File) string {
	for _, node := range syntax.Decls {
		declaration, ok := node.(*ast.GenDecl)
		imports := ok && declaration.Tok == token.IMPORT
		if !imports {
			continue
		}
		if preamble, found := importCPreamble(declaration); found {
			return preamble
		}
	}
	return ""
}

func importCPreamble(declaration *ast.GenDecl) (string, bool) {
	for _, spec := range declaration.Specs {
		imported, ok := spec.(*ast.ImportSpec)
		if !ok || imported.Path.Value != strconv.Quote("C") {
			continue
		}
		comment := imported.Doc
		if comment == nil {
			comment = declaration.Doc
		}
		return comment.Text(), true
	}
	return "", false
}

// nativeScan follows the includes of one reached package.
type nativeScan struct {
	reach     *ruleReach
	tree      sourceTree
	known     map[string]bool
	directory string
	// searched are the -I, -isystem and -idirafter directories of the
	// package's #cgo lines, and quoteSearched its -iquote directories,
	// which only quoted includes search.
	searched      []string
	quoteSearched []string
	// forced are the headers -include and -imacros options name.
	forced []forcedInclude
	// unsearchable records an include directory lawgate cannot read and
	// the owner did not approve: a header found in no repository
	// directory may be there.
	unsearchable bool
	// elsewhere are the bracketed headers found in no repository
	// directory: the compiler's own or an approved directory's.
	elsewhere map[string]bool
	visited   map[string]bool
	whole     bool
}

func (reach *ruleReach) protectNativeInputs(paths []string) {
	for directory := range reach.packages {
		scan := &nativeScan{reach: reach, tree: reach.sources.deferred.tree, known: reach.sources.deferred.known, directory: directory, visited: make(map[string]bool), elsewhere: make(map[string]bool)}
		scan.run(paths)
	}
}

// run reads every #cgo line of the package before it resolves any
// include: the go command applies each Go file's flags to all of the
// package's cgo files.
func (scan *nativeScan) run(paths []string) {
	files := scan.reach.sources.directories[scan.directory]
	for _, file := range files {
		scan.addSearched(file.name, cgoPreamble(file.syntax))
	}
	scan.followForced()
	for _, file := range files {
		scan.follow(file.name, cgoPreamble(file.syntax))
	}
	scan.followBuildFiles(paths)
	scan.noteElsewhere()
	if scan.whole {
		scan.protectTree(paths)
	}
}

func (scan *nativeScan) followBuildFiles(paths []string) {
	for _, name := range paths {
		relative, inside := relativeTo(name, scan.directory)
		native := inside && buildFile(relative)
		if native {
			scan.followFile(name)
		}
	}
}

// searchable is the repository directory a #cgo include option names,
// or false when it adds none: lawgate cannot read it (reported on the
// declaring file) or the owner approved it as an outside directory.
func (scan *nativeScan) searchable(declaring, value string) (string, bool) {
	directory, err := scan.includeDirectory(value)
	if scan.approved(err) {
		return "", false
	}
	if err != nil {
		scan.unsearchable = true
		scan.refuse(declaring, fmt.Errorf("#cgo include directory %q %w", value, err))
		return "", false
	}
	return directory, true
}

// includeDirectory is the repository directory a #cgo flag value names.
// ${SRCDIR} is the package directory; a relative directory is relative to
// it, where the go command runs the compiler.
func (scan *nativeScan) includeDirectory(value string) (string, error) {
	relative, fromSource := strings.CutPrefix(value, "${SRCDIR}")
	if strings.Contains(relative, "$") {
		return "", errUnknownVariable
	}
	if !fromSource && path.IsAbs(value) {
		return scan.absolute(value)
	}
	return scan.reachable(scan.directory, relative)
}

// refuse reports a problem on name and protects the package tree, which
// holds every candidate lawgate can protect.
func (scan *nativeScan) refuse(name string, err error) {
	scan.whole = true
	scan.reach.result.protect(name, err)
}

func (scan *nativeScan) followFile(name string) {
	if scan.visited[name] {
		return
	}
	scan.visited[name] = true
	data, err := scan.tree.read(name)
	if err != nil {
		scan.reach.result.protect(name, err)
		return
	}
	scan.follow(name, string(data))
}

func (scan *nativeScan) follow(name, text string) {
	for _, match := range nativeDirective.FindAllStringSubmatch(text, -1) {
		scan.followInclude(name, parseInclude(path.Dir(name), match[1]))
	}
}

// followInclude protects every header an include of file name could
// load. An include that names no repository file is a system header
// (noted once for the package), unless it is quoted (it may name a file
// lawgate cannot see, so the tree is protected) or an include directory
// lawgate cannot read and the owner did not approve may hold it
// (reported).
func (scan *nativeScan) followInclude(name string, include nativeIncludeRef) {
	if include.computed {
		scan.refuse(name, fmt.Errorf("#include %s names its header through a macro: lawgate cannot resolve it", include.header))
		return
	}
	found := scan.resolve(include)
	for _, header := range found.headers {
		scan.reach.rules[header] = true
		scan.followFile(header)
	}
	for _, problem := range found.problems {
		scan.refuse(name, fmt.Errorf("%s %w", include.spelling(), problem))
	}
	switch {
	case found.any():
	case scan.unsearchable:
		scan.refuse(name, fmt.Errorf("%s %w", include.spelling(), errUnsearchedInclude))
	case include.quoted:
		scan.whole = true
	default:
		scan.elsewhere[include.header] = true
	}
}

// nativeIncludeRef is one #include: the including file's directory, the
// header named, whether it was quoted rather than bracketed, whether a
// macro names it instead, whether a #cgo -include forced it, and whether
// it is anchored: named from ${SRCDIR}, which the go command expands to
// an absolute path the compiler opens without searching.
type nativeIncludeRef struct {
	from     string
	header   string
	quoted   bool
	computed bool
	forced   bool
	anchored bool
}

// parseInclude reads a directive's operand: "header", <header>, or
// anything else, which only a macro expansion can turn into a header.
func parseInclude(from, operand string) nativeIncludeRef {
	include := nativeIncludeRef{from: from, header: strings.TrimSpace(operand)}
	match := includeOperand.FindStringSubmatch(include.header)
	if match == nil {
		include.computed = true
		return include
	}
	include.header, include.quoted = match[2], match[1] == `"`
	return include
}

// spelling is the directive as the source wrote it, for a problem.
func (include nativeIncludeRef) spelling() string {
	switch {
	case include.anchored:
		return "#cgo -include " + strconv.Quote("${SRCDIR}"+include.header)
	case include.forced:
		return "#cgo -include " + strconv.Quote(include.header)
	case include.quoted:
		return "#include " + strconv.Quote(include.header)
	}
	return "#include <" + include.header + ">"
}

// resolution is what one include could load: the repository headers, the
// problems met on the way, and whether an approved outside directory may
// supply it.
type resolution struct {
	headers  []string
	problems []error
	approved bool
}

// any reports whether the search found something: a header, a problem or
// an approved outside directory.
func (found resolution) any() bool {
	met := len(found.headers) + len(found.problems)
	return met > 0 || found.approved
}

// resolve finds the include in every directory the compiler could search
// for it. GCC takes the first match in a fixed order (for a quoted
// include the including file's directory and -iquote, then -I, -isystem,
// the system directories and -idirafter), which is not the order the
// #cgo flags are written in, and Go joins the flags of several files. So
// every repository file any of these directories holds is protected: a
// change to any of them is reviewed, whichever one the compiler reads.
func (scan *nativeScan) resolve(include nativeIncludeRef) resolution {
	return scan.candidates(scan.searchList(include), include.header)
}

func (scan *nativeScan) candidates(directories []string, header string) resolution {
	var found resolution
	for _, directory := range directories {
		candidate, err := scan.reachable(directory, header)
		fresh := scan.known[candidate] && !slices.Contains(found.headers, candidate)
		switch {
		case scan.approved(err):
			found.approved = true
		case err != nil:
			found.problems = append(found.problems, err)
		case fresh:
			found.headers = append(found.headers, candidate)
		}
	}
	return found
}

// searchList is every directory the compiler could search for include:
// none but the package directory for an anchored one and the root for an
// absolute one, and the including file's directory and the -iquote
// directories as well for a quoted one.
func (scan *nativeScan) searchList(include nativeIncludeRef) []string {
	switch {
	case include.anchored:
		return []string{include.from}
	case path.IsAbs(include.header):
		return []string{"/"}
	case include.quoted:
		return slices.Concat([]string{include.from}, scan.quoteSearched, scan.searched)
	}
	return scan.searched
}

// protectTree protects every file under the package directory: an include
// it cannot resolve may name any of them.
func (scan *nativeScan) protectTree(paths []string) {
	for _, name := range paths {
		if _, inside := relativeTo(name, scan.directory); inside {
			scan.reach.rules[name] = true
		}
	}
}
