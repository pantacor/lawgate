package readability

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Baseline maps relative file paths to allowed finding counts.
type Baseline map[string]int

// LoadBaseline reads a baseline file formatted as "<count> <path>" per line.
func LoadBaseline(path string) (Baseline, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	base := make(Baseline)
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if err := parseBaselineLine(scanner.Text(), lineNo, base); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return base, nil
}

func parseBaselineLine(raw string, lineNo int, base Baseline) error {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	countStr, filePath, ok := strings.Cut(line, " ")
	if !ok {
		return fmt.Errorf("baseline line %d: expected '<count> <path>'", lineNo)
	}
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 0 {
		return fmt.Errorf("baseline line %d: invalid count %q", lineNo, countStr)
	}
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return fmt.Errorf("baseline line %d: empty file path", lineNo)
	}
	base[filePath] = count
	return nil
}

// WriteBaseline writes b as a sorted, stable text file with "<count> <path>" per line.
// Entries with count <= 0 are omitted.
func WriteBaseline(path string, b Baseline) error {
	paths := make([]string, 0, len(b))
	for p, count := range b {
		if count <= 0 {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var sb strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&sb, "%d %s\n", b[p], p)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// CountByFile aggregates findings into a per-file Baseline map.
func CountByFile(findings []Finding) Baseline {
	counts := make(Baseline)
	for _, f := range findings {
		counts[f.Path]++
	}
	return counts
}

// Ratchet compares current findings against base and returns one problem line
// per file whose count grew or that is new with any finding.
// Files whose counts stayed equal or shrank produce no problem lines.
func Ratchet(current []Finding, base Baseline) []string {
	counts := CountByFile(current)
	paths := sortedKeys(counts)
	var problems []string
	for _, p := range paths {
		cur := counts[p]
		prev, existed := base[p]
		if !existed {
			problems = append(problems, fmt.Sprintf("%s: %d findings (new file, baseline 0)", p, cur))
			continue
		}
		if cur > prev {
			problems = append(problems, fmt.Sprintf("%s: %d findings (grew from baseline %d)", p, cur, prev))
		}
	}
	return problems
}

// Shrinkable reports files whose current finding count is lower than base.
// Each line uses the phrasing "baseline can shrink to N".
func Shrinkable(current []Finding, base Baseline) []string {
	counts := CountByFile(current)
	paths := sortedKeys(base)
	var notes []string
	for _, p := range paths {
		prev := base[p]
		cur := counts[p]
		if cur < prev {
			notes = append(notes, fmt.Sprintf("%s: baseline can shrink to %d (was %d)", p, cur, prev))
		}
	}
	return notes
}

func sortedKeys(m Baseline) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
