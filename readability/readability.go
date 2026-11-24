package readability

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pantacor/lawgate/bite"
)

const (
	// RuleNesting reports statements nested more than 2 block levels deep.
	RuleNesting = "nesting"
	// RuleInlineNesting reports struct/map literals nested inside struct/map literals.
	RuleInlineNesting = "inline-nesting"
	// RuleFuncLength reports functions or methods whose body spans more than 40 lines.
	RuleFuncLength = "func-length"
	// RuleCondition reports unnamed conditions with 3+ && / || operands.
	RuleCondition = "condition"
	// RuleBoolParam reports function parameters declared as bool.
	RuleBoolParam = "bool-param"
)

// Finding describes a single readability violation.
type Finding struct {
	Rule    string
	Path    string
	Line    int
	Func    string
	Message string
}

// CheckFile parses a single Go source file and returns all readability findings.
// If src is nil, the file is read from path.
func CheckFile(path string, src []byte) ([]Finding, error) {
	if bite.Off("lawgate/readable-laws-code") {
		return nil, nil
	}
	fset := token.NewFileSet()
	var srcArg any
	if src != nil {
		srcArg = src
	}
	file, err := parser.ParseFile(fset, path, srcArg, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var findings []Finding
	findings = append(findings, checkNesting(fset, path, file)...)
	findings = append(findings, checkInlineNesting(fset, path, file)...)
	findings = append(findings, checkFuncLength(fset, path, file)...)
	findings = append(findings, checkConditions(fset, path, file)...)
	findings = append(findings, checkBoolParams(fset, path, file)...)
	sortFindings(findings)
	return findings, nil
}

// CheckDir walks root and checks all Go source files (including _test.go files).
// It skips vendor, testdata, hidden directories, and nested go.mod modules.
// If include is non-nil, only files for which include(relSlashPath) returns true
// are checked.
func CheckDir(root string, include func(path string) bool) ([]Finding, error) {
	var all []Finding
	err := filepath.WalkDir(root, func(full string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return skipDirIfNeeded(root, full, d.Name())
		}
		return checkDirFile(root, full, d.Name(), include, &all)
	})
	if err != nil {
		return nil, err
	}
	sortFindings(all)
	return all, nil
}

func skipDirIfNeeded(root, full, name string) error {
	if full == root {
		return nil
	}
	if isIgnoredDirName(name) {
		return filepath.SkipDir
	}
	if hasGoMod(full) {
		return filepath.SkipDir
	}
	return nil
}

func isIgnoredDirName(name string) bool {
	switch name {
	case "vendor", "testdata":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func hasGoMod(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func checkDirFile(root, full, name string, include func(string) bool, out *[]Finding) error {
	if !strings.HasSuffix(name, ".go") {
		return nil
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	if include != nil && !include(rel) {
		return nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	findings, err := CheckFile(rel, data)
	if err != nil {
		return err
	}
	*out = append(*out, findings...)
	return nil
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
}

func funcDeclName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	recv := formatRecvType(d.Recv.List[0].Type)
	return recv + "." + d.Name.Name
}

func formatRecvType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return "(*" + formatRecvType(t.X) + ")"
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return formatRecvType(t.X)
	case *ast.IndexListExpr:
		return formatRecvType(t.X)
	default:
		return "?"
	}
}

func funcLitName(parent string) string {
	if parent == "" {
		return "<func literal>"
	}
	return parent + ".<func literal>"
}
