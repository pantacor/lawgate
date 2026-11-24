package bite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const bitePackage = "github.com/pantacor/lawgate/bite"

// A test switch in production code can be moved to guard something
// meaningless while the law's test keeps passing. The gate mutates the
// subject instead, so no Go file outside the tests may hold such a switch.
func TestGoCodeHasNoTestSwitch(t *testing.T) {
	files := productionGoFiles(t, "..")
	if len(files) == 0 {
		t.Fatal("found no Go files to scan")
	}
	for _, name := range files {
		for _, found := range testSwitches(t, name) {
			t.Errorf("%s holds a test switch", found)
		}
	}
}

func productionGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipDirectory(path, entry.Name(), root)
		}
		if isProductionGo(entry.Name()) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func skipDirectory(path, name, root string) error {
	hidden := strings.HasPrefix(name, ".") && path != root
	notSource := name == "testdata" || name == "vendor" || name == "site"
	if hidden || notSource {
		return fs.SkipDir
	}
	return nil
}

func isProductionGo(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

// testSwitches lists the calls of the removed bite.Off hook and the reads of
// the rerun variable through a build tag in one file.
func testSwitches(t *testing.T, name string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	if tag := biteBuildTag(file); tag != "" {
		found = append(found, name+" (build constraint "+tag+")")
	}
	alias := biteAlias(file)
	if alias == "" {
		return found
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if isOffCall(node, alias) {
			found = append(found, fset.Position(node.Pos()).String())
		}
		return true
	})
	return found
}

func biteBuildTag(file *ast.File) string {
	for _, group := range file.Comments {
		text := group.Text()
		isConstraint := group.Pos() < file.Package && strings.Contains(text, "lawgate_bite")
		if isConstraint {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

// biteAlias is the name the file imports the bite package under, "" when it
// does not import it.
func biteAlias(file *ast.File) string {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != bitePackage {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return "bite"
	}
	return ""
}

func isOffCall(node ast.Node, alias string) bool {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return receiver.Name == alias && selector.Sel.Name == "Off"
}
