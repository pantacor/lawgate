package gosource

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// GeneratedFiles lists repository-relative paths with a standard Go generation
// header in the source tree, including inactive files and test sources.
func GeneratedFiles(root string) ([]string, error) {
	root, err := CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	var files []string
	var problems error
	err = walk(root, repository, func(file string) {
		if !strings.HasSuffix(file, ".go") {
			return
		}
		generated, err := generatedFile(file)
		if err != nil {
			problems = errors.Join(problems, err)
		}
		if generated {
			relative, _ := filepath.Rel(root, file)
			files = append(files, filepath.ToSlash(relative))
		}
	})
	return files, errors.Join(problems, err)
}

func generatedFile(file string) (bool, error) {
	flags := parser.PackageClauseOnly | parser.ParseComments
	syntax, err := parser.ParseFile(token.NewFileSet(), file, nil, flags)
	if syntax == nil {
		return false, err
	}
	// The header is independent of whether the package clause is valid.
	return ast.IsGenerated(syntax), nil
}
