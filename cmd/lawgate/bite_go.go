package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/pantacor/lawgate/bite"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

func (phase bitePhase) goTarget(item biteCase, dir string) (biteTestTarget, bool) {
	inputs, err := phase.goTests()
	if err != nil && !hasDiscoveryFailure(phase.result.Verdicts) {
		phase.result.recordError(testDiscovery, err)
	}
	if inputs == nil {
		return biteTestTarget{}, false
	}
	test, err := inputs.tests.EnforcingTest(item.law, item.enforcer)
	if err != nil {
		phase.add(verdict{Name: item.name(), Status: failed, Detail: err.Error()})
		return biteTestTarget{}, false
	}
	command := inputs.build.SelectTest(test.Name, biteTag, goBiteOperands(inputs.build, test))
	target := biteTestTarget{name: test.QualifiedName(), dir: dir, command: command, tagged: true}
	selector, found, err := phase.findGoSelector(inputs, item, test)
	if err != nil {
		phase.result.recordError(item.name(), err)
		return biteTestTarget{}, false
	}
	if !found {
		detail := fmt.Sprintf("bite hook missing in reachable non-test Go code: add bite.Off(%q) or document a bite gap", selector)
		phase.add(verdict{Name: item.name(), Status: failed, Detail: detail})
		return biteTestTarget{}, false
	}
	target.selector = selector
	return target, true
}

func (phase bitePhase) findGoSelector(inputs *goBiteInputs, item biteCase, test lawsdoc.TestFunc) (string, bool, error) {
	root, err := goBiteModuleRoot(phase.root, filepath.Dir(test.File))
	if err != nil {
		return "", false, err
	}
	hooks, err := inputs.reachableHooks(root, test)
	if err != nil {
		return "", false, err
	}
	selector := goBiteSelector(inputs.framework, root, test, item.law)
	return selector, hooks[selector], nil
}

// hookScope is what decides which hooks a test reaches: its package variant.
type hookScope struct {
	pkg  string
	file string
}

// reachableHooks scans a test package's closure once, however many laws its
// tests enforce.
func (inputs *goBiteInputs) reachableHooks(root string, test lawsdoc.TestFunc) (map[string]bool, error) {
	scope := hookScope{pkg: test.Package, file: test.File}
	if hooks, scanned := inputs.hooks[scope]; scanned {
		return hooks, nil
	}
	packages, err := inputs.closure()
	if err != nil {
		return nil, fmt.Errorf("list tagged Go bite packages: %w", err)
	}
	hooks, err := lawsdoc.GoBiteHooks(root, inputs.framework+"/bite", test, packages)
	if err != nil {
		return nil, fmt.Errorf("scan Go bite hooks: %w", err)
	}
	inputs.hooks[scope] = hooks
	return hooks, nil
}

// frameworkModule is the module path this binary was built from, as its
// go.mod declares it: a fork or a later major version knows its own hooks.
//
// Without it every hook lookup would search for the wrong package and report
// each bite as a missing hook, so the phase stops with the real reason.
func frameworkModule() (string, error) {
	info, ok := readBuildInfo()
	if !ok || info.Main.Path == "" {
		return "", errNoBuildInfo
	}
	return info.Main.Path, nil
}

// readBuildInfo is a variable so that a test can run as a binary without it.
var readBuildInfo = debug.ReadBuildInfo

var errNoBuildInfo = errors.New(
	"lawgate binary carries no Go build information and cannot identify its own bite hooks: " +
		"no Go bite was checked (build with go build, go install or go run in module mode)",
)

func goBiteModuleRoot(repo, dir string) (string, error) {
	rel, err := filepath.Rel(repo, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("test package %q is outside the repository", dir)
	}
	for ; ; dir = filepath.Dir(dir) {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if dir == repo {
			return "", fmt.Errorf("test package has no Go module within the repository: %w", err)
		}
	}
}

// goBiteSelector reserves the framework prefix for the framework's own tests.
func goBiteSelector(framework, root string, test lawsdoc.TestFunc, law string) string {
	dir, _ := filepath.Rel(root, filepath.Dir(test.File))
	frameworkPackage := framework
	if dir != "." {
		frameworkPackage += "/" + filepath.ToSlash(dir)
	}
	if framework != "" && test.Package == frameworkPackage {
		return bite.FrameworkPrefix + law
	}
	return law
}

func goBiteOperands(build inventory.GoCommand, test lawsdoc.TestFunc) []string {
	if test.Package == "command-line-arguments" {
		return build.Operands
	}
	return []string{test.Package}
}
