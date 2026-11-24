package rulechanges

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// compilePattern turns a CODEOWNERS path into a regular expression with the
// GitHub and GitLab semantics. Where the platforms could be read two ways,
// it matches less, so a coverage check never claims a review that the
// platform would not require.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	if strings.ContainsAny(pattern, "[]!\\") {
		return nil, fmt.Errorf("unsupported CODEOWNERS pattern %q", pattern)
	}
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return nil, fmt.Errorf("empty CODEOWNERS path")
	}
	var expression strings.Builder
	expression.WriteString(patternStart(pattern))
	writeGlob(&expression, trimmed)
	expression.WriteString(patternEnd(pattern))
	return regexp.Compile(expression.String())
}

// rootedPattern reports whether pattern is relative to the repository root:
// a leading slash, or a slash anywhere but at the end, makes it so.
func rootedPattern(pattern string) bool {
	return strings.HasPrefix(pattern, "/") || strings.Contains(strings.Trim(pattern, "/"), "/")
}

func patternStart(pattern string) string {
	if rootedPattern(pattern) {
		return "^"
	}
	return "(?:^|/)"
}

// patternEnd decides what may follow the match. "docs/*" and "/*" name the
// entries of one directory, not the files below them, so a rooted pattern
// whose last segment has a wildcard must match the whole path.
func patternEnd(pattern string) string {
	lastWildcard := strings.ContainsAny(path.Base(strings.Trim(pattern, "/")), "*?")
	switch {
	case strings.HasSuffix(pattern, "/"):
		return "/"
	case rootedPattern(pattern) && lastWildcard:
		return "$"
	default:
		return "(?:/|$)"
	}
}

func writeGlob(expression *strings.Builder, pattern string) {
	for index := 0; index < len(pattern); {
		token, width := globToken(pattern, index)
		expression.WriteString(token)
		index += width
	}
}

// globToken translates the glob token at index. "**" crosses directories
// only as a whole segment; inside a name it is a plain "*".
func globToken(pattern string, index int) (string, int) {
	remaining := pattern[index:]
	segmentStart := index == 0 || pattern[index-1] == '/'
	switch {
	case segmentStart && strings.HasPrefix(remaining, "**/"):
		return "(?:.*/)?", 3
	case segmentStart && remaining == "**":
		return ".*", 2
	case strings.HasPrefix(remaining, "**"):
		return "[^/]*", 2
	case remaining[0] == '*':
		return "[^/]*", 1
	case remaining[0] == '?':
		return "[^/]", 1
	}
	literal := leadingLiteral(remaining)
	return regexp.QuoteMeta(literal), len(literal)
}

func leadingLiteral(pattern string) string {
	end := strings.IndexAny(pattern, "*?")
	if end < 0 {
		return pattern
	}
	return pattern[:end]
}
