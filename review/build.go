package review

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

// Execute runs a module command the way `lawgate check` runs it: args in
// the module directory (relative to root), bounded by timeout. It returns
// standard output, standard error and the error.
type Execute func(root, module string, args []string, timeout time.Duration) ([]byte, string, error)

// Options tune how an index is built.
type Options struct {
	// Execute runs the machine commands. Nil runs them with
	// inventory.RunGateCommand; `lawgate review` passes the check's own
	// runner, so both refuse the same inputs.
	Execute Execute
}

func (options Options) execute() Execute {
	if options.Execute != nil {
		return options.Execute
	}
	return func(root, module string, args []string, timeout time.Duration) ([]byte, string, error) {
		return inventory.RunGateCommand(filepath.Join(root, module), args, timeout)
	}
}

// FindRoot returns the repository root for dir: the nearest directory at or
// above it that holds a lawgate.json.
func FindRoot(dir string) (string, error) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, inventory.Filename)); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fs.ErrNotExist
		}
		current = parent
	}
}

// Build reads the repository at root: every module's LAWS.md, the index of
// every module with machines (through its machine command, with -index), and
// the lines of every subject and enforcing test. What cannot be read is
// recorded as a Problem; only an unreadable lawgate.json is an error.
func Build(root string, options Options) (*Index, error) {
	absolute, err := gosource.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	config, err := inventory.Load(absolute)
	if err != nil {
		return nil, err
	}
	builder := newBuilder(absolute, options)
	for _, module := range config.Modules {
		builder.module(module)
	}
	return builder.finish(), nil
}

// builder carries what building an index shares between modules.
type builder struct {
	idx     *Index
	execute Execute
	// documents are the LAWS.md files already read: a shared one is read
	// once, for the first module that lists it.
	documents map[string]bool
	packages  packageDirs
}

func newBuilder(root string, options Options) *builder {
	idx := &Index{Root: root, Laws: make(map[string]*Law), Sites: []Site{}, States: []State{}, Problems: []Problem{}}
	return &builder{idx: idx, execute: options.execute(), documents: make(map[string]bool), packages: readPackageDirs(root)}
}

func (b *builder) module(module inventory.Module) {
	b.idx.modules = append(b.idx.modules, module.Path)
	b.readDocuments(module)
	if indexesMachines(module) {
		b.readMachines(module)
	}
	if module.Language == "go" {
		b.resolveAnchors(module)
	}
}

func indexesMachines(module inventory.Module) bool {
	declares := module.Machines != nil && *module.Machines
	return declares && module.Language == "go"
}

func (b *builder) problem(problem Problem) {
	b.idx.Problems = append(b.idx.Problems, problem)
}

func (b *builder) finish() *Index {
	slices.SortStableFunc(b.idx.Sites, func(left, right Site) int {
		return cmp.Or(strings.Compare(left.File, right.File), cmp.Compare(left.Line, right.Line))
	})
	return b.idx
}

// moduleOf is the lawgate.json module that holds a repository file: the
// longest module path it is under.
func (idx *Index) moduleOf(file string) string {
	best := ""
	found := false
	for _, module := range idx.modules {
		under := module == "." || file == module || strings.HasPrefix(file, module+"/")
		longer := !found || len(module) > len(best)
		if under && longer {
			best, found = module, true
		}
	}
	return best
}
