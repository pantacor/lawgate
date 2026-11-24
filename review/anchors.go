package review

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/mutate"
	"github.com/pantacor/lawgate/review/gocode"
)

// goModule is a Go module of the repository: its import path and directory.
type goModule struct {
	importPath string
	dir        string
}

// packageDirs finds the directory of an import path among the repository's
// Go modules, without running the go tool.
type packageDirs []goModule

func readPackageDirs(root string) packageDirs {
	dirs, _ := gosource.Modules(root)
	var modules packageDirs
	for _, dir := range dirs {
		if importPath := modulePath(filepath.Join(root, filepath.FromSlash(dir), "go.mod")); importPath != "" {
			modules = append(modules, goModule{importPath: importPath, dir: dir})
		}
	}
	return modules
}

// modulePath reads the module directive of a go.mod.
func modulePath(goMod string) string {
	file, err := os.Open(goMod)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}

// dirFor is the repository directory of an import path: under the module
// with the longest matching path. ok is false outside every module.
func (modules packageDirs) dirFor(importPath string) (string, bool) {
	best := goModule{}
	for _, module := range modules {
		inside := importPath == module.importPath || strings.HasPrefix(importPath, module.importPath+"/")
		if inside && len(module.importPath) > len(best.importPath) {
			best = module
		}
	}
	if best.importPath == "" {
		return "", false
	}
	rest := strings.TrimPrefix(importPath, best.importPath)
	return path.Join(best.dir, strings.TrimPrefix(rest, "/")), true
}

// resolveAnchors places the subjects and enforcing tests of the laws module
// writes on the lines of their functions.
func (b *builder) resolveAnchors(module inventory.Module) {
	laws := b.idx.writtenBy(module.Path)
	for _, law := range laws {
		b.resolveSubjects(law)
	}
	if !anyTests(laws) {
		return
	}
	dir := filepath.Join(b.idx.Root, filepath.FromSlash(module.Path))
	tests, err := lawsdoc.DiscoverCommandTests(dir, module.TestCommand, module.Timeout())
	if err != nil {
		b.problem(Problem{Module: module.Path, File: module.Laws, Message: "find tests: " + err.Error()})
		return
	}
	for _, law := range laws {
		b.resolveTests(law, tests)
	}
}

// writtenBy lists the laws module's own LAWS.md files write.
func (idx *Index) writtenBy(module string) []*Law {
	var laws []*Law
	for _, law := range idx.Laws {
		if law.Module == module && law.File != "" {
			laws = append(laws, law)
		}
	}
	return laws
}

func anyTests(laws []*Law) bool {
	for _, law := range laws {
		if len(law.testNames) > 0 {
			return true
		}
	}
	return false
}

func (b *builder) resolveSubjects(law *Law) {
	for _, subject := range law.subjects {
		ref := CodeRef{Name: subject.String(), Law: law.Key}
		lines, problem := b.subjectLines(subject)
		ref.Problem = problem
		ref.File, ref.Line, ref.EndLine = lines.File, lines.Start, lines.End
		law.Subjects = append(law.Subjects, ref)
	}
}

func (b *builder) subjectLines(subject mutate.Subject) (gocode.Range, string) {
	dir, found := b.packages.dirFor(subject.ImportPath)
	if !found {
		return gocode.Range{}, "package " + subject.ImportPath + " is not in a Go module of the repository"
	}
	name := subject.Func
	if subject.Receiver != "" {
		name = subject.Receiver + "." + subject.Func
	}
	lines, found := gocode.Functions(b.idx.Root, dir)[name]
	if !found {
		return gocode.Range{}, name + " is not declared in " + dir
	}
	return lines, ""
}

func (b *builder) resolveTests(law *Law, tests lawsdoc.Tests) {
	for _, name := range law.testNames {
		ref := CodeRef{Name: name, Law: law.Key}
		found, err := tests.Resolve(name)
		if err != nil {
			ref.Problem = "law " + law.Name + " " + err.Error()
			law.Tests = append(law.Tests, ref)
			continue
		}
		ref.File, ref.Line, ref.EndLine = b.testLines(found)
		law.Tests = append(law.Tests, ref)
	}
}

func (b *builder) testLines(test lawsdoc.TestFunc) (string, int, int) {
	file, err := filepath.Rel(b.idx.Root, test.File)
	if err != nil {
		return "", 0, 0
	}
	file = filepath.ToSlash(file)
	lines, found := gocode.FunctionAt(b.idx.Root, file, test.Line)
	if !found {
		return file, test.Line, test.Line
	}
	return file, lines.Start, lines.End
}
