package rulechanges

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// cgoOption is what an include option of a #cgo line does with its value.
type cgoOption int

const (
	cgoOther cgoOption = iota
	// cgoDirectory adds a directory to the header search.
	cgoDirectory
	// cgoQuoteDirectory adds a directory that only quoted includes search.
	cgoQuoteDirectory
	// cgoForced makes the compiler read a header before every C file of
	// the package, with no #include in the source.
	cgoForced
)

type cgoSpelling struct {
	prefix string
	option cgoOption
}

// cgoSpellings are the include options, each joined ("-Idir") or with its
// value as the next argument ("-I dir"). A longer spelling comes before a
// shorter one it starts with.
var cgoSpellings = []cgoSpelling{
	{"-I", cgoDirectory}, {"-iquote", cgoQuoteDirectory}, {"-isystem", cgoDirectory}, {"-idirafter", cgoDirectory},
	{"--include=", cgoForced}, {"--imacros=", cgoForced}, {"--include", cgoForced}, {"--imacros", cgoForced},
	{"-include", cgoForced}, {"-imacros", cgoForced},
}

// forcedInclude is a header a #cgo line forces into every C file, kept
// until every #cgo line of the package is read: the header is searched in
// the include directories all of them add.
type forcedInclude struct {
	declaring string
	header    string
}

// addSearched records the include directories and forced headers of the
// #cgo lines of the Go file declaring. A pkg-config line is reported: the
// flags it adds come from outside the repository when the package is
// built, so nobody can review them here.
func (scan *nativeScan) addSearched(declaring, preamble string) {
	for _, match := range cgoDirective.FindAllStringSubmatch(preamble, -1) {
		switch directive := cgoDirectiveName(match[1]); {
		case cgoFlagDirectives[directive]:
			scan.addFlagList(declaring, match[2])
		case directive == "pkg-config":
			scan.refuse(declaring, fmt.Errorf("#cgo pkg-config: %s %w", strings.TrimSpace(match[2]), errPkgConfig))
		}
	}
}

// cgoDirectiveName is the flag set a #cgo line sets, after its build
// constraints ("linux,amd64 CFLAGS" sets CFLAGS).
func cgoDirectiveName(words string) string {
	fields := strings.Fields(words)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// addFlagList splits a #cgo flag list as the go command does. A list it
// cannot split is reported: the directories in it are unknown.
func (scan *nativeScan) addFlagList(declaring, list string) {
	flags, err := splitCgoFlags(list)
	if err != nil {
		scan.unsearchable = true
		scan.refuse(declaring, fmt.Errorf("#cgo flags %q: %w", strings.TrimSpace(list), err))
	}
	scan.addFlags(declaring, flags)
}

func (scan *nativeScan) addFlags(declaring string, flags []string) {
	for index := 0; index < len(flags); index++ {
		option, value, consumed := cgoIncludeOption(flags, index)
		index += consumed
		scan.addOption(declaring, option, value)
	}
}

func (scan *nativeScan) addOption(declaring string, option cgoOption, value string) {
	switch {
	case value == "":
		return
	case option == cgoDirectory:
		scan.searched = scan.appendSearchable(scan.searched, declaring, value)
	case option == cgoQuoteDirectory:
		scan.quoteSearched = scan.appendSearchable(scan.quoteSearched, declaring, value)
	case option == cgoForced:
		scan.forced = append(scan.forced, forcedInclude{declaring: declaring, header: value})
	}
}

// appendSearchable is directories with the one value names, when lawgate
// can search it.
func (scan *nativeScan) appendSearchable(directories []string, declaring, value string) []string {
	if directory, ok := scan.searchable(declaring, value); ok {
		return append(directories, directory)
	}
	return directories
}

// cgoIncludeOption is the include option at index, its value and how many
// following arguments the value took (0 or 1).
func cgoIncludeOption(flags []string, index int) (cgoOption, string, int) {
	for _, spelling := range cgoSpellings {
		value, matched := strings.CutPrefix(flags[index], spelling.prefix)
		if !matched {
			continue
		}
		separate := value == "" && index+1 < len(flags)
		if separate {
			return spelling.option, flags[index+1], 1
		}
		return spelling.option, value, 0
	}
	return cgoOther, "", 0
}

// followForced follows every forced header as the compiler searches it:
// like a quoted include, from the package directory, then the include
// directories.
func (scan *nativeScan) followForced() {
	for _, forced := range scan.forced {
		include, err := scan.forcedInclude(forced.header)
		if err != nil {
			scan.refuse(forced.declaring, fmt.Errorf("#cgo -include %q %w", forced.header, err))
			continue
		}
		scan.followInclude(forced.declaring, include)
	}
}

// forcedInclude is a forced header as an include. ${SRCDIR} is the
// package directory, where the go command runs the compiler; the rest of
// the value is kept as written, never cleaned, so a symbolic link in it
// is still met on the walk. Any other variable is refused.
func (scan *nativeScan) forcedInclude(value string) (nativeIncludeRef, error) {
	include := nativeIncludeRef{from: scan.directory, header: value, quoted: true, forced: true}
	relative, fromSource := strings.CutPrefix(value, "${SRCDIR}")
	if strings.Contains(relative, "$") {
		return include, errUnknownVariable
	}
	if fromSource {
		include.header, include.anchored = relative, true
	}
	return include, nil
}

var (
	errUnclosedQuote    = errors.New("unclosed quote")
	errUnfinishedEscape = errors.New("unfinished escaping")
	cgoQuoteCharacter   = map[rune]bool{'"': true, '\'': true}
)

// noQuote is the quote of an argument outside quotes.
const noQuote rune = 0

// cgoSplitter splits a #cgo flag list with go/build's rules: a backslash
// escapes the next character (inside quotes too), single and double
// quotes group, and white space outside quotes separates arguments.
type cgoSplitter struct {
	arguments []string
	current   []rune
	escaped   bool
	// quoted records that the current argument had quotes, so "" is an
	// empty argument rather than none.
	quoted bool
	quote  rune
}

func splitCgoFlags(list string) ([]string, error) {
	var splitter cgoSplitter
	for _, character := range list {
		splitter.read(character)
	}
	splitter.finish()
	switch {
	case splitter.quote != noQuote:
		return splitter.arguments, errUnclosedQuote
	case splitter.escaped:
		return splitter.arguments, errUnfinishedEscape
	}
	return splitter.arguments, nil
}

func (splitter *cgoSplitter) read(character rune) {
	switch {
	case splitter.escaped:
		splitter.escaped = false
	case character == '\\':
		splitter.escaped = true
		return
	case splitter.quote != noQuote:
		if character == splitter.quote {
			splitter.quote = noQuote
			return
		}
	case cgoQuoteCharacter[character]:
		splitter.quoted, splitter.quote = true, character
		return
	case unicode.IsSpace(character):
		splitter.finish()
		return
	}
	splitter.current = append(splitter.current, character)
}

// finish ends the current argument, if one was started.
func (splitter *cgoSplitter) finish() {
	started := splitter.quoted || len(splitter.current) > 0
	if !started {
		return
	}
	splitter.arguments = append(splitter.arguments, string(splitter.current))
	splitter.current, splitter.quoted = nil, false
}
