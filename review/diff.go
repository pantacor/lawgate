package review

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Hunk is one changed range of a file, in -U0 terms.
type Hunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Header   string   `json:"header,omitempty"` // the function git names
	Added    []string `json:"added,omitempty"`
	Removed  []string `json:"removed,omitempty"`
}

// NewEnd is the last line of the hunk in the new file (NewStart for a pure
// removal).
func (h Hunk) NewEnd() int {
	if h.NewLines == 0 {
		return h.NewStart
	}
	return h.NewStart + h.NewLines - 1
}

// FileDiff is one changed file.
type FileDiff struct {
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
	Status  string `json:"status"` // added, deleted, modified, renamed
	Hunks   []Hunk `json:"hunks"`
}

// Diff returns the changes from base to the working tree (committed and
// not), limited to paths when any are given.
func Diff(root, base string, paths ...string) ([]FileDiff, error) {
	args := []string{"diff", "-U0", "--no-color", "--no-ext-diff", "-M", base, "--"}
	out, err := git(root, append(args, paths...)...)
	if err != nil {
		return nil, err
	}
	return ParseDiff(out), nil
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

// ParseDiff parses a unified diff.
func ParseDiff(out string) []FileDiff {
	var parser diffParser
	for _, line := range strings.Split(out, "\n") {
		parser.line(line)
	}
	parser.flushFile()
	for i := range parser.files {
		if parser.files[i].OldPath == parser.files[i].Path {
			parser.files[i].OldPath = ""
		}
	}
	return parser.files
}

type diffParser struct {
	files   []FileDiff
	current *FileDiff
	hunk    *Hunk
}

func (p *diffParser) line(line string) {
	if strings.HasPrefix(line, "diff --git ") {
		p.startFile(line)
		return
	}
	if p.current == nil {
		return
	}
	if p.hunk != nil && p.hunkLine(line) {
		return
	}
	p.headerLine(line)
}

func (p *diffParser) startFile(line string) {
	p.flushFile()
	p.current = &FileDiff{Status: "modified"}
	old, path, found := strings.Cut(strings.TrimPrefix(line, "diff --git "), " b/")
	if found {
		p.current.OldPath = strings.TrimPrefix(old, "a/")
		p.current.Path = path
	}
}

// hunkLine records an added or removed line of the current hunk.
func (p *diffParser) hunkLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "+"):
		p.hunk.Added = append(p.hunk.Added, line[1:])
	case strings.HasPrefix(line, "-"):
		p.hunk.Removed = append(p.hunk.Removed, line[1:])
	default:
		return false
	}
	return true
}

func (p *diffParser) headerLine(line string) {
	switch {
	case strings.HasPrefix(line, "new file mode"):
		p.current.Status = "added"
	case strings.HasPrefix(line, "deleted file mode"):
		p.current.Status = "deleted"
	case strings.HasPrefix(line, "rename from "):
		p.current.Status = "renamed"
	case strings.HasPrefix(line, "+++ b/"):
		p.current.Path = strings.TrimPrefix(line, "+++ b/")
	case strings.HasPrefix(line, "@@ "):
		p.startHunk(line)
	}
}

func (p *diffParser) startHunk(line string) {
	p.flushHunk()
	match := hunkHeader.FindStringSubmatch(line)
	if match == nil {
		return
	}
	p.hunk = &Hunk{
		OldStart: atoi(match[1]), OldLines: lineCount(match[2]),
		NewStart: atoi(match[3]), NewLines: lineCount(match[4]),
		Header: strings.TrimSpace(match[5]),
	}
}

func (p *diffParser) flushHunk() {
	if p.current != nil && p.hunk != nil {
		p.current.Hunks = append(p.current.Hunks, *p.hunk)
	}
	p.hunk = nil
}

func (p *diffParser) flushFile() {
	p.flushHunk()
	if p.current != nil {
		p.files = append(p.files, *p.current)
	}
	p.current = nil
}

func atoi(text string) int {
	number, _ := strconv.Atoi(text)
	return number
}

// lineCount reads a hunk's line count, which git leaves out when it is 1.
func lineCount(text string) int {
	if text == "" {
		return 1
	}
	return atoi(text)
}

// Untracked lists the files git does not track yet (and does not ignore),
// as added files whose every line is a change: a branch under review often
// has new files nobody has added.
func Untracked(root string, paths ...string) ([]FileDiff, error) {
	args := []string{"ls-files", "-z", "--others", "--exclude-standard", "--"}
	out, err := git(root, append(args, paths...)...)
	if err != nil {
		return nil, err
	}
	var files []FileDiff
	for _, name := range strings.Split(out, "\x00") {
		if name != "" {
			files = append(files, untrackedFile(root, name))
		}
	}
	return files, nil
}

func untrackedFile(root, name string) FileDiff {
	file := FileDiff{Path: name, Status: "untracked"}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil || len(data) == 0 {
		return file
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	hunk := Hunk{NewStart: 1, NewLines: len(lines), Added: lines}
	file.Hunks = []Hunk{hunk}
	return file
}
