package resolve

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"strconv"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// SourceTag is a log tag found in a string literal of the source.
type SourceTag struct {
	Tag      string           // normalised: machine prefix stripped, "-" for the bare prefix
	Line     int              // 1-based
	Detected machine.SiteKind // kind of the nearest enclosing call the vocabulary knows; "" if none
	Calls    []string         // names of the enclosing calls, innermost first
}

// source is one parsed file: its tags, its lines with comments blanked, and
// the byte spans of calls in innermost-first order.
type source struct {
	tags      []SourceTag
	codeLines []string
	calls     []sourceCall
}

type sourceCall struct {
	start, end int
	names      []string
}

// callsAt finds the enclosing calls of the matched substring. Calls
// elsewhere on the same line cannot identify the matched site.
func (s *source) callsAt(start, end int) []string {
	var names []string
	for _, call := range s.calls {
		if call.start <= start && end <= call.end {
			names = append(names, call.names...)
		}
	}
	return names
}

func parseSource(src []byte, vocab Vocabulary, prefix string) (*source, error) {
	fset := token.NewFileSet()
	// Comments are not parsed at all, so a tag in a comment is never seen.
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	scan := &tagScanner{fset: fset, vocab: vocab, prefix: prefix}
	ast.Inspect(file, scan.visit)
	parsed := &source{tags: scan.tags, codeLines: codeLines(src), calls: scan.calls}
	return parsed, nil
}

// tagScanner walks the syntax tree and keeps the chain of enclosing nodes,
// so a tag literal knows which calls it is an argument of.
type tagScanner struct {
	fset   *token.FileSet
	vocab  Vocabulary
	prefix string
	stack  []ast.Node
	tags   []SourceTag
	calls  []sourceCall
}

func (s *tagScanner) visit(node ast.Node) bool {
	if node == nil {
		s.leave()
		return true
	}
	s.stack = append(s.stack, node)
	if lit, ok := node.(*ast.BasicLit); ok {
		s.literal(lit)
	}
	return true
}

// Record calls on exit so a nested call is considered before its parent.
func (s *tagScanner) leave() {
	last := len(s.stack) - 1
	node := s.stack[last]
	s.stack = s.stack[:last]
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return
	}
	file := s.fset.File(call.Pos())
	span := sourceCall{start: file.Offset(call.Pos()), end: file.Offset(call.End()), names: callNames(call)}
	s.calls = append(s.calls, span)
}

func (s *tagScanner) literal(lit *ast.BasicLit) {
	if lit.Kind != token.STRING {
		return
	}
	text, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	tag, ok := s.vocab.findTag(text)
	if !ok {
		return
	}
	calls := s.enclosingCalls()
	found := SourceTag{
		Tag:      normaliseTag(tag, s.prefix),
		Line:     s.fset.Position(lit.Pos()).Line,
		Detected: s.vocab.kindOf(calls),
		Calls:    calls,
	}
	s.tags = append(s.tags, found)
}

// enclosingCalls lists the names of the calls around the current node,
// innermost first.
func (s *tagScanner) enclosingCalls() []string {
	var names []string
	for i := len(s.stack) - 1; i >= 0; i-- {
		if call, ok := s.stack[i].(*ast.CallExpr); ok {
			names = append(names, callNames(call)...)
		}
	}
	return names
}

// callNames names a call the ways a vocabulary may list it: "pkg.Func" (or
// "recv.Method" when the receiver is a plain name) first, then "Func".
func callNames(call *ast.CallExpr) []string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return []string{fun.Name}
	case *ast.SelectorExpr:
		return selectorNames(fun)
	}
	return nil
}

func selectorNames(sel *ast.SelectorExpr) []string {
	if x, ok := sel.X.(*ast.Ident); ok {
		return []string{x.Name + "." + sel.Sel.Name, sel.Sel.Name}
	}
	return []string{sel.Sel.Name}
}

func normaliseTag(tag, prefix string) string {
	tag = strings.TrimSpace(tag)
	if prefix == "" || !strings.HasPrefix(tag, prefix) {
		return tag
	}
	tag = strings.TrimSpace(strings.TrimPrefix(tag, prefix))
	if tag == "" {
		return "-"
	}
	return tag
}

// codeLines returns the source split in lines with every comment replaced by
// spaces, so a Match never binds to a comment.
func codeLines(src []byte) []string {
	code := []byte(string(src))
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var scan scanner.Scanner
	scan.Init(file, src, nil, scanner.ScanComments)
	for {
		pos, tok, lit := scan.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			start := file.Offset(pos)
			blank(code[start:min(start+len(lit), len(code))])
		}
	}
	return strings.Split(string(code), "\n")
}

// blank replaces every byte but newlines with a space.
func blank(text []byte) {
	for i := range text {
		if text[i] != '\n' {
			text[i] = ' '
		}
	}
}
