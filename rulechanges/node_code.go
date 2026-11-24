package rulechanges

import (
	"bytes"
	"strings"
)

// A Node enforcer binds to the file whose test(...) call names it, so a
// call spelled inside a comment, a string or a regular expression must not
// count: it would bind the enforcer to a file the runner never executes
// as that test and leave the real one unowned. nodeCode blanks those
// literals first. Its reading of a regular expression is a heuristic; a
// misreading can only blank code, which leaves an enforcer unbound and
// reported, never bound to the wrong file.

// regexKeywords are the keywords after which "/" opens a regular
// expression rather than dividing.
var regexKeywords = map[string]bool{
	"await": true, "case": true, "delete": true, "do": true, "else": true, "in": true,
	"instanceof": true, "new": true, "of": true, "return": true, "throw": true,
	"typeof": true, "void": true, "yield": true,
}

// regexOperators are the code bytes after which "/" opens a regular
// expression rather than dividing.
const regexOperators = "(,=:[!&|?{};+-*%<>~^"

// nodeCode is source with the body of every comment, string, template and
// regular expression literal blanked, offsets kept, so a pattern run over
// it matches code only. A string's quotes stay, so a call's title can be
// found here and read from source.
func nodeCode(source []byte) []byte {
	lexer := &nodeLexer{source: source, code: bytes.Clone(source)}
	for lexer.position < len(source) {
		lexer.step()
	}
	return lexer.code
}

type nodeLexer struct {
	source   []byte
	code     []byte
	position int
	// substitutions holds, per open template ${...}, the braces opened
	// inside it, so the brace that closes it resumes the template.
	substitutions []int
	// previous is the last code byte that is not white space.
	previous byte
}

func (lexer *nodeLexer) step() {
	current := lexer.source[lexer.position]
	switch {
	case lexer.at("//"):
		lexer.skipComment("\n")
	case lexer.at("/*"):
		lexer.skipComment("*/")
	case current == '"' || current == '\'':
		lexer.skipString(current)
	case current == '`':
		lexer.position++
		lexer.skipTemplate()
	case current == '/' && lexer.regexAllowed():
		lexer.skipRegex()
	default:
		lexer.codeByte(current)
	}
}

func (lexer *nodeLexer) at(prefix string) bool {
	return bytes.HasPrefix(lexer.source[lexer.position:], []byte(prefix))
}

// blank replaces code[from:to] with spaces, keeping line breaks.
func (lexer *nodeLexer) blank(from, to int) {
	to = min(to, len(lexer.code))
	for index := from; index < to; index++ {
		if lexer.code[index] != '\n' {
			lexer.code[index] = ' '
		}
	}
}

// skipComment blanks a comment up to its end, which a line comment keeps.
func (lexer *nodeLexer) skipComment(end string) {
	start := lexer.position
	length := bytes.Index(lexer.source[start+2:], []byte(end))
	stop := len(lexer.source)
	if length >= 0 {
		stop = start + 2 + length + len(end)
	}
	lexer.blank(start, stop)
	lexer.position = stop
}

// skipString blanks a quoted string's body; a line break ends an
// unterminated one, as it ends the statement.
func (lexer *nodeLexer) skipString(quote byte) {
	start := lexer.position
	lexer.position++
	for lexer.position < len(lexer.source) {
		current := lexer.source[lexer.position]
		if current == quote || current == '\n' {
			break
		}
		lexer.position += escapeWidth(current)
	}
	lexer.blank(start+1, lexer.position)
	if lexer.position < len(lexer.source) && lexer.source[lexer.position] == quote {
		lexer.position++
	}
	lexer.previous = quote
}

// escapeWidth is how far one literal character reaches: two bytes for an
// escape, one otherwise.
func escapeWidth(current byte) int {
	if current == '\\' {
		return 2
	}
	return 1
}

// skipTemplate blanks a template's text from the current position to its
// closing backtick, or to a ${ whose expression is code.
func (lexer *nodeLexer) skipTemplate() {
	start := lexer.position
	for lexer.position < len(lexer.source) {
		if lexer.source[lexer.position] == '`' {
			lexer.blank(start, lexer.position)
			lexer.position++
			lexer.previous = '`'
			return
		}
		if lexer.at("${") {
			lexer.blank(start, lexer.position)
			lexer.position += 2
			lexer.substitutions = append(lexer.substitutions, 0)
			lexer.previous = '{'
			return
		}
		lexer.position += escapeWidth(lexer.source[lexer.position])
	}
	lexer.blank(start, lexer.position)
}

// skipRegex blanks a regular expression literal's body; a "/" inside a
// character class does not end it.
func (lexer *nodeLexer) skipRegex() {
	start := lexer.position
	lexer.position++
	inClass := false
	for lexer.position < len(lexer.source) {
		current := lexer.source[lexer.position]
		ends := current == '\n' || (current == '/' && !inClass)
		if ends {
			break
		}
		inClass = (inClass || current == '[') && current != ']'
		lexer.position += escapeWidth(current)
	}
	lexer.blank(start+1, lexer.position)
	lexer.position++
	lexer.previous = ')'
}

// regexAllowed reports whether a "/" here opens a regular expression:
// at the start, after an operator or after a keyword such as return.
func (lexer *nodeLexer) regexAllowed() bool {
	if lexer.previous == 0 || strings.IndexByte(regexOperators, lexer.previous) >= 0 {
		return true
	}
	return regexKeywords[lexer.wordBefore()]
}

// wordBefore is the identifier that ends just before the current position.
func (lexer *nodeLexer) wordBefore() string {
	end := len(bytes.TrimRight(lexer.code[:lexer.position], " \t\r\n"))
	start := end
	for start > 0 && identifierByte(lexer.code[start-1]) {
		start--
	}
	return string(lexer.code[start:end])
}

func identifierByte(current byte) bool {
	letter := current|0x20 >= 'a' && current|0x20 <= 'z'
	digit := current >= '0' && current <= '9'
	alphanumeric := letter || digit
	return alphanumeric || strings.IndexByte("_$", current) >= 0
}

// codeByte passes one code byte, tracking the braces of an open template
// substitution: the one that closes it resumes the template's text.
func (lexer *nodeLexer) codeByte(current byte) {
	lexer.position++
	if current == '}' && lexer.closesSubstitution() {
		lexer.skipTemplate()
		return
	}
	if current == '{' && len(lexer.substitutions) > 0 {
		lexer.substitutions[len(lexer.substitutions)-1]++
	}
	if !strings.ContainsRune(" \t\r\n", rune(current)) {
		lexer.previous = current
	}
}

func (lexer *nodeLexer) closesSubstitution() bool {
	last := len(lexer.substitutions) - 1
	if last < 0 {
		return false
	}
	if lexer.substitutions[last] == 0 {
		lexer.substitutions = lexer.substitutions[:last]
		return true
	}
	lexer.substitutions[last]--
	return false
}
