package gocode

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/pantacor/lawgate/gosource"
)

// Literal is a composite literal that declares a site: its Tag and Match
// keyed string values.
type Literal struct {
	File  string
	Line  int
	Tag   string
	Match string
}

// Literals are the site literals of a module, with the directories that
// declare each machine name (a `Name: "<machine>"` keyed value).
type Literals struct {
	Sites        []Literal
	machineFiles map[string][]string // machine name → files that name it
}

// ScanLiterals reads every non-test Go file of the module at root (not
// nested modules, vendor, testdata or hidden directories). A file that does
// not parse is skipped.
func ScanLiterals(root string) Literals {
	scan := literalScan{root: root}
	scan.literals.machineFiles = make(map[string][]string)
	_ = filepath.WalkDir(root, scan.visit)
	return scan.literals
}

type literalScan struct {
	root     string
	literals Literals
}

func (scan *literalScan) visit(full string, entry fs.DirEntry, err error) error {
	if err != nil {
		return nil // an unreadable directory is skipped, not fatal
	}
	relative, _ := filepath.Rel(scan.root, full)
	relative = filepath.ToSlash(relative)
	if entry.IsDir() {
		return scan.directory(full, relative)
	}
	if isSourceFile(entry.Name()) {
		scan.file(relative)
	}
	return nil
}

func (scan *literalScan) directory(full, relative string) error {
	if relative == "." {
		return nil
	}
	if gosource.Skipped(relative + "/x.go") {
		return filepath.SkipDir
	}
	if fileExists(filepath.Join(full, "go.mod")) {
		return filepath.SkipDir
	}
	return nil
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

func (scan *literalScan) file(relative string) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, filepath.Join(scan.root, relative), nil, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		if literal, ok := node.(*ast.CompositeLit); ok {
			scan.literal(fset, relative, literal)
		}
		return true
	})
}

func (scan *literalScan) literal(fset *token.FileSet, file string, literal *ast.CompositeLit) {
	values := keyedStrings(literal)
	if name, ok := values["Name"]; ok {
		scan.literals.machineFiles[name.text] = append(scan.literals.machineFiles[name.text], file)
	}
	tag, hasTag := values["Tag"]
	match, hasMatch := values["Match"]
	if !hasTag && !hasMatch {
		return
	}
	// The site is placed on its Tag (or its Match, for an untagged site),
	// which a multi-line literal may write below the brace.
	at := tag.at
	if !hasTag {
		at = match.at
	}
	site := Literal{File: file, Line: fset.Position(at).Line, Tag: tag.text, Match: match.text}
	scan.literals.Sites = append(scan.literals.Sites, site)
}

// keyedString is a `Key: "text"` element and where it is.
type keyedString struct {
	text string
	at   token.Pos
}

// keyedStrings returns the literal's `Key: "string"` elements.
func keyedStrings(literal *ast.CompositeLit) map[string]keyedString {
	values := map[string]keyedString{}
	for _, element := range literal.Elts {
		key, value, ok := stringElement(element)
		if ok {
			values[key] = value
		}
	}
	return values
}

func stringElement(element ast.Expr) (string, keyedString, bool) {
	pair, ok := element.(*ast.KeyValueExpr)
	if !ok {
		return "", keyedString{}, false
	}
	key, keyed := pair.Key.(*ast.Ident)
	text, quoted := pair.Value.(*ast.BasicLit)
	isString := quoted && text.Kind == token.STRING
	if !keyed || !isString {
		return "", keyedString{}, false
	}
	value, err := strconv.Unquote(text.Value)
	return key.Name, keyedString{text: value, at: pair.Pos()}, err == nil
}

// Find returns the literal that declares a site of machine: Tag and Match
// both equal, else (a Match written apart from its Tag) the Tag alone. When
// several literals qualify, one in a directory that names the machine wins,
// so two machines that reuse a tag are told apart.
func (literals Literals) Find(machineName, tag, match string) (Literal, bool) {
	exact := literals.matching(func(site Literal) bool { return site.Tag == tag && site.Match == match })
	if len(exact) == 0 && tag != "" {
		exact = literals.matching(func(site Literal) bool { return site.Tag == tag })
	}
	if len(exact) == 0 {
		return Literal{}, false
	}
	return literals.preferMachine(machineName, exact), true
}

func (literals Literals) matching(keep func(Literal) bool) []Literal {
	var found []Literal
	for _, site := range literals.Sites {
		if keep(site) {
			found = append(found, site)
		}
	}
	return found
}

func (literals Literals) preferMachine(machineName string, candidates []Literal) Literal {
	var dirs []string
	for _, file := range literals.machineFiles[machineName] {
		dirs = append(dirs, path.Dir(file))
	}
	for _, candidate := range candidates {
		if slices.Contains(dirs, path.Dir(candidate.File)) {
			return candidate
		}
	}
	return candidates[0]
}
