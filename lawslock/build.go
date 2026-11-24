package lawslock

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/mutate"
)

type moduleBuild struct {
	tests    lawsdoc.Tests
	packages map[string]gosource.Package
	read     Reader
}

// Build fingerprints the active Go proof and subject for each agreed law.
func Build(root string, laws []Law, read Reader) (Lock, error) {
	lock := Lock{Version: 1, Laws: make([]Entry, 0, len(laws))}
	builds := make(map[string]moduleBuild)
	seen := make(map[ID]bool)
	for _, law := range laws {
		if seen[law.ID] {
			return Lock{}, fmt.Errorf("duplicate agreed law %s (%s, %s)", law.Law, law.Module, law.Document)
		}
		seen[law.ID] = true
		entry, err := buildEntry(root, law, read, builds)
		if err != nil {
			return Lock{}, fmt.Errorf("law %s (%s, %s): %w", law.Law, law.Module, law.Document, err)
		}
		lock.Laws = append(lock.Laws, entry)
	}
	slices.SortFunc(lock.Laws, func(left, right Entry) int {
		return cmp.Or(cmp.Compare(left.Module, right.Module), cmp.Compare(left.Document, right.Document), cmp.Compare(left.Law, right.Law))
	})
	return lock, nil
}

func buildEntry(root string, law Law, read Reader, builds map[string]moduleBuild) (Entry, error) {
	entry := Entry{ID: law.ID}
	if len(law.Tests) == 0 {
		return entry, nil
	}
	build, exists := builds[law.Module]
	if !exists {
		var err error
		build, err = loadModuleBuild(filepath.Join(root, law.Module), law.ModuleConfig, read)
		if err != nil {
			return Entry{}, err
		}
		builds[law.Module] = build
	}
	return build.lockEntry(root, law)
}

func loadModuleBuild(dir string, module inventory.Module, read Reader) (moduleBuild, error) {
	packages, err := inventory.GoPackages(dir, module.TestCommand, module.Timeout())
	if err != nil {
		return moduleBuild{}, err
	}
	tests, err := lawsdoc.DiscoverCommandTests(dir, module.TestCommand, module.Timeout())
	if err != nil {
		return moduleBuild{}, err
	}
	build := moduleBuild{tests: tests, packages: make(map[string]gosource.Package), read: read}
	for _, pkg := range packages {
		build.packages[pkg.ImportPath] = pkg
	}
	return build, nil
}

func (build moduleBuild) lockEntry(root string, law Law) (Entry, error) {
	entry := Entry{ID: law.ID}
	for _, name := range law.Tests {
		if err := build.addTest(root, &entry, name); err != nil {
			return Entry{}, err
		}
	}
	slices.SortFunc(entry.Tests, func(left, right Test) int {
		return cmp.Compare(left.QualifiedName(), right.QualifiedName())
	})
	entry.Tests = slices.Compact(entry.Tests)
	slices.Sort(entry.TestFiles)
	entry.TestFiles = slices.Compact(entry.TestFiles)
	var err error
	entry.Proof, err = Proof(entry, build.read)
	if err != nil {
		return entry, err
	}
	for _, subject := range law.Subjects {
		hash, err := build.subjectFingerprint(root, law, subject)
		if err != nil {
			return Entry{}, err
		}
		anchor := Subject{Reference: subject.String(), Fingerprint: hash}
		entry.Subjects = append(entry.Subjects, anchor)
	}
	slices.SortFunc(entry.Subjects, func(left, right Subject) int {
		return cmp.Compare(left.Reference, right.Reference)
	})
	entry.Subjects = slices.Compact(entry.Subjects)
	return entry, nil
}

func (build moduleBuild) addTest(root string, entry *Entry, name string) error {
	found, err := build.tests.EnforcingTest(entry.Law, name)
	if err != nil {
		return err
	}
	file, err := relativeFile(root, found.File)
	if err != nil {
		return err
	}
	entry.Tests = append(entry.Tests, Test{Package: found.Package, Name: found.Name, File: file})
	for _, path := range build.packages[found.Package].Tests() {
		file, err := relativeFile(root, path)
		if err != nil {
			return err
		}
		entry.TestFiles = append(entry.TestFiles, file)
	}
	return nil
}

func relativeFile(root, name string) (string, error) {
	relative, err := filepath.Rel(root, name)
	slash := filepath.ToSlash(relative)
	if err != nil || !localPath(slash) {
		return "", fmt.Errorf("source file %q is outside the repository", name)
	}
	return slash, nil
}

func (build moduleBuild) subjectFingerprint(root string, law Law, subject mutate.Subject) (string, error) {
	if pkg, found := build.packages[subject.ImportPath]; found {
		return hashSubject(root, subject, pkg, build.read)
	}
	module := law.ModuleConfig
	packages, err := inventory.GoTestPackages(filepath.Join(root, module.Path), module.TestArgs(), module.Timeout())
	if err != nil {
		return "", err
	}
	for _, pkg := range packages {
		if pkg.ForTest == "" && pkg.ImportPath == subject.ImportPath {
			return hashSubject(root, subject, pkg, build.read)
		}
	}
	return "", fmt.Errorf("subject %s is not in the active Go build", subject)
}
