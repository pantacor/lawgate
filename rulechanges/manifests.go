package rulechanges

import (
	"bufio"
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type goManifest struct {
	module       string
	goVersion    string
	replacements []moduleReplace
	uses         []string
}

type moduleReplace struct {
	module    string
	version   string
	directory string
}

// manifestToken is one go.mod or go.work token: an interpreted or raw Go
// string, a comment start, a bare word or a parenthesis.
var manifestToken = regexp.MustCompile("^\"([^\"\\\\]|\\\\.)*\"|^`[^`]*`|^//|^[^\\s()]+|^[()]")

func manifestFields(line string) ([]string, error) {
	var fields []string
	for line = strings.TrimSpace(line); line != ""; line = strings.TrimSpace(line) {
		token := manifestToken.FindString(line)
		if token == "//" {
			break
		}
		if !quotedToken(token) {
			token, _, _ = strings.Cut(token, "//")
		}
		value, err := manifestValue(token)
		if err != nil {
			return nil, err
		}
		fields = append(fields, value)
		line = strings.TrimPrefix(line, token)
	}
	return fields, nil
}

func manifestValue(token string) (string, error) {
	if quotedToken(token) {
		return strconv.Unquote(token)
	}
	return token, nil
}

// quotedToken is an interpreted or raw string: "//" inside it is text.
func quotedToken(token string) bool {
	return strings.HasPrefix(token, "\"") || strings.HasPrefix(token, "`")
}

func parseManifest(name string, data []byte) (goManifest, error) {
	parser := manifestParser{base: path.Dir(name)}
	lines := bufio.NewScanner(bytes.NewReader(data))
	for lines.Scan() {
		if err := parser.line(lines.Text()); err != nil {
			return parser.manifest, err
		}
	}
	if parser.block != "" {
		return parser.manifest, fmt.Errorf("unclosed %s block", parser.block)
	}
	return parser.manifest, lines.Err()
}

// manifestParser reads go.mod or go.work one line at a time; block is the
// directive of the parenthesised block it is inside, if any.
type manifestParser struct {
	manifest goManifest
	base     string
	block    string
}

func (parser *manifestParser) line(text string) error {
	fields, err := manifestFields(text)
	if err != nil || len(fields) == 0 {
		return err
	}
	opensBlock := len(fields) == 2 && fields[1] == "(" && parser.block == ""
	closesBlock := len(fields) == 1 && fields[0] == ")" && parser.block != ""
	switch {
	case opensBlock:
		parser.block = fields[0]
		return nil
	case closesBlock:
		parser.block = ""
		return nil
	}
	directive, args := manifestDirective(parser.block, fields)
	return parser.manifest.addDirective(parser.base, directive, args)
}

func manifestDirective(block string, fields []string) (string, []string) {
	if block != "" {
		return block, fields
	}
	return fields[0], fields[1:]
}

func (manifest *goManifest) addDirective(base, directive string, args []string) error {
	switch directive {
	case "module":
		if len(args) != 1 {
			return fmt.Errorf("invalid module directive")
		}
		manifest.module = args[0]
	case "go":
		if len(args) != 1 {
			return fmt.Errorf("invalid go directive")
		}
		manifest.goVersion = args[0]
	case "use":
		if len(args) != 1 {
			return fmt.Errorf("invalid use directive")
		}
		manifest.uses = append(manifest.uses, manifestDirectory(base, args[0]))
	case "replace":
		replacement, err := parseReplacement(base, args)
		if err != nil {
			return err
		}
		manifest.replacements = append(manifest.replacements, replacement)
	}
	return nil
}

func parseReplacement(base string, args []string) (moduleReplace, error) {
	if len(args) < 3 || len(args) > 5 {
		return moduleReplace{}, fmt.Errorf("invalid replace directive")
	}
	arrow := 1
	replacement := moduleReplace{module: args[0]}
	if args[arrow] != "=>" {
		replacement.version = args[arrow]
		arrow++
	}
	validLength := len(args) > arrow+1 && len(args) <= arrow+3
	if args[arrow] != "=>" || !validLength {
		return moduleReplace{}, fmt.Errorf("invalid replace directive")
	}
	if len(args) == arrow+2 {
		replacement.directory = manifestDirectory(base, args[arrow+1])
	}
	return replacement, nil
}

func manifestDirectory(base, target string) string {
	if path.IsAbs(target) {
		return path.Clean(target)
	}
	return path.Join(base, target)
}

func (sources *goSources) readManifest(tree sourceTree, name string) error {
	data, err := tree.read(name)
	if err != nil {
		return err
	}
	manifest, err := parseManifest(name, data)
	if err != nil {
		return err
	}
	manifest.relativize(tree.root)
	directory := path.Dir(name)
	if path.Base(name) == "go.work" {
		sources.workspaces[directory] = manifest
	} else if manifest.module != "" {
		sources.modules[directory] = manifest
	}
	return nil
}

func (manifest *goManifest) relativize(root string) {
	for i, directory := range manifest.uses {
		manifest.uses[i] = repositoryDirectory(root, directory)
	}
	for i, replace := range manifest.replacements {
		manifest.replacements[i].directory = repositoryDirectory(root, replace.directory)
	}
}

func repositoryDirectory(root, directory string) string {
	if root == "" || !filepath.IsAbs(directory) {
		return directory
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return directory
	}
	relative, err := filepath.Rel(base, directory)
	if err != nil || !filepath.IsLocal(relative) {
		return directory
	}
	return filepath.ToSlash(relative)
}
