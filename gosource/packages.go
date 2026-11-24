package gosource

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
)

// PackageFields names the go list fields needed for source discovery.
const PackageFields = "Dir,ImportPath,GoFiles,CgoFiles,InvalidGoFiles,TestGoFiles,XTestGoFiles,Imports,ImportMap,Standard,ForTest,Error"

// Package describes the files and imports selected by go list.
type Package struct {
	Dir            string
	ImportPath     string
	GoFiles        []string
	CgoFiles       []string
	InvalidGoFiles []string
	TestGoFiles    []string
	XTestGoFiles   []string
	Imports        []string
	ImportMap      map[string]string
	Standard       bool
	ForTest        string
	Error          *struct{ Err string }
}

// Sources lists the selected non-test Go inputs in lexical order.
func (pkg Package) Sources() []string {
	names := slices.Concat(pkg.GoFiles, pkg.CgoFiles, pkg.InvalidGoFiles)
	return uniquePaths(pkg.Dir, names)
}

// Tests lists the selected internal and external test files in lexical order.
func (pkg Package) Tests() []string {
	names := slices.Concat(pkg.TestGoFiles, pkg.XTestGoFiles)
	return uniquePaths(pkg.Dir, names)
}

func uniquePaths(dir string, names []string) []string {
	slices.Sort(names)
	for index, name := range names {
		if !filepath.IsAbs(name) {
			names[index] = filepath.Join(dir, name)
		}
	}
	return slices.Compact(names)
}

// ReadPackages reads go list metadata. A non-nil file set filters the results.
func ReadPackages(data io.Reader, files map[string]bool) ([]Package, error) {
	var packages []Package
	decoder := json.NewDecoder(data)
	for {
		var pkg Package
		err := decoder.Decode(&pkg)
		switch err {
		case io.EOF:
			return packages, nil
		case nil:
		default:
			return packages, err
		}
		if pkg.Dir == "" && pkg.Error != nil {
			return packages, fmt.Errorf("go list: %s", pkg.Error.Err)
		}
		if files != nil {
			pkg = pkg.selected(files)
		}
		packages = append(packages, pkg)
	}
}

func (pkg Package) selected(files map[string]bool) Package {
	inactive := func(name string) bool { return !files[filepath.Join(pkg.Dir, name)] }
	pkg.GoFiles = slices.DeleteFunc(pkg.GoFiles, inactive)
	pkg.CgoFiles = slices.DeleteFunc(pkg.CgoFiles, inactive)
	pkg.InvalidGoFiles = slices.DeleteFunc(pkg.InvalidGoFiles, inactive)
	pkg.TestGoFiles = slices.DeleteFunc(pkg.TestGoFiles, inactive)
	pkg.XTestGoFiles = slices.DeleteFunc(pkg.XTestGoFiles, inactive)
	return pkg
}
