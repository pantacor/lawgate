package rulechanges

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

type history struct {
	root    string
	command func(...string) ([]byte, error)
	blobs   func(string) ([]byte, error)
}

type revision struct {
	id      string
	parents []string
}

type auditTrail struct {
	approvedBy []string
	approves   []string
}

func Audit(root, base, head string, output io.Writer) (err error) {
	batch, err := startBlobs(root)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, batch.close())
	}()
	repo := history{
		root:    root,
		command: func(args ...string) ([]byte, error) { return gitCommand(root, args...) },
		blobs:   batch.read,
	}
	return repo.audit(base, head, output)
}

func (repo history) audit(base, head string, output io.Writer) error {
	revisions, err := repo.revisions(base, head)
	if err != nil {
		return err
	}
	cache := make(map[string]map[string]bool)
	var auditErrors error
	for _, commit := range revisions {
		if err := repo.auditCommit(commit, cache, output); err != nil {
			auditErrors = errors.Join(auditErrors, err)
		}
	}
	return auditErrors
}

func (repo history) revisions(base, head string) ([]revision, error) {
	shallow, err := repo.command("rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(shallow)) != "false" {
		return nil, fmt.Errorf("cannot establish audit range from shallow history")
	}
	baseID, err := repo.commitID(base)
	if err != nil {
		return nil, fmt.Errorf("cannot establish base %q: %w", base, err)
	}
	headID, err := repo.commitID(head)
	if err != nil {
		return nil, fmt.Errorf("cannot establish head %q: %w", head, err)
	}
	bases, err := repo.command("merge-base", "--all", baseID, headID)
	if err != nil || len(strings.Fields(string(bases))) == 0 {
		return nil, errors.Join(fmt.Errorf("cannot establish merge base for %s and %s", base, head), err)
	}
	command := []string{"rev-list", "--reverse", "--topo-order", "--parents", headID, "--not"}
	raw, err := repo.command(append(command, strings.Fields(string(bases))...)...)
	if err != nil {
		return nil, err
	}
	return parseRevisions(raw), nil
}

func parseRevisions(raw []byte) []revision {
	var revisions []revision
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			revisions = append(revisions, revision{fields[0], fields[1:]})
		}
	}
	return revisions
}

var objectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func (repo history) commitID(ref string) (string, error) {
	if ref == "" || strings.ContainsAny(ref, "\r\n\x00") {
		return "", fmt.Errorf("invalid commit reference")
	}
	raw, err := repo.command("rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if !objectID.MatchString(id) {
		return "", fmt.Errorf("invalid resolved commit ID %q", id)
	}
	return id, nil
}

func (repo history) auditCommit(commit revision, cache map[string]map[string]bool, output io.Writer) error {
	files, err := repo.changedRules(commit, cache)
	if err != nil {
		return fmt.Errorf("%s: %w", commit.id, err)
	}
	raw, err := repo.command("show", "-s", "--format=%(trailers:only,unfold)", commit.id)
	if err != nil {
		return err
	}
	trail := parseTrail(raw)
	if err := writeAuditEntry(output, commit.id, files, trail); err != nil {
		return err
	}
	unapprovedNote := len(trail.approves) > 0 && len(trail.approvedBy) == 0
	if unapprovedNote {
		return fmt.Errorf("%s: Rule-change-approves requires a Rule-change-approved-by audit trailer", commit.id)
	}
	return nil
}

func writeAuditEntry(output io.Writer, id string, files []string, trail auditTrail) error {
	if len(files) == 0 && len(trail.approves) == 0 {
		return nil
	}
	notes := strings.Join(trail.approvedBy, ", ")
	if notes == "" {
		notes = "(none)"
	}
	_, err := fmt.Fprintf(output, "%s\n  Rule-change-approved-by (audit): %q\n", id, notes)
	for _, file := range files {
		_, writeErr := fmt.Fprintf(output, "  rule file: %q\n", file)
		err = errors.Join(err, writeErr)
	}
	for _, commit := range trail.approves {
		_, writeErr := fmt.Fprintf(output, "  Rule-change-approves (audit): %q\n", commit)
		err = errors.Join(err, writeErr)
	}
	return err
}

func parseTrail(raw []byte) auditTrail {
	var trail auditTrail
	for _, line := range strings.Split(string(raw), "\n") {
		trail.consume(line)
	}
	return trail
}

func (trail *auditTrail) consume(line string) {
	key, value, _ := strings.Cut(line, ":")
	value = strings.TrimSpace(value)
	switch strings.ToLower(key) {
	case "rule-change-approved-by":
		if value != "" {
			trail.approvedBy = append(trail.approvedBy, value)
		}
	case "rule-change-approves":
		trail.approves = append(trail.approves, value)
	}
}

func (repo history) changedRules(commit revision, cache map[string]map[string]bool) ([]string, error) {
	// Compare each merge parent so a resolution changed only in the merge is visible.
	raw, err := repo.command("diff-tree", "--no-commit-id", "-r", "-m", "--root", "-M", "--name-status", "-z", commit.id)
	if err != nil {
		return nil, err
	}
	changed, err := parseChanges(raw)
	if err != nil || len(changed) == 0 {
		return nil, err
	}
	trees := append(slices.Clone(commit.parents), commit.id)
	found := make(map[string]bool)
	for _, tree := range trees {
		rules, err := repo.rulesAt(tree, cache)
		if err != nil {
			return nil, err
		}
		markChanges(found, rules, changed)
	}
	return sortedPaths(found), nil
}

type pathChange struct {
	first  string
	second string
}

func parseChanges(raw []byte) ([]pathChange, error) {
	fields := splitNUL(raw)
	var changed []pathChange
	for len(fields) > 0 {
		change, remaining, err := parseChange(fields)
		if err != nil {
			return nil, err
		}
		changed = append(changed, change)
		fields = remaining
	}
	return changed, nil
}

func parseChange(fields []string) (pathChange, []string, error) {
	if len(fields) < 2 || fields[0] == "" {
		return pathChange{}, nil, fmt.Errorf("malformed Git diff-tree entry")
	}
	change := pathChange{first: fields[1]}
	paired := fields[0][0] == 'R' || fields[0][0] == 'C'
	if !paired {
		return change, fields[2:], nil
	}
	if len(fields) < 3 {
		return pathChange{}, nil, fmt.Errorf("Git diff-tree omitted a rename or copy destination")
	}
	change.second = fields[2]
	return change, fields[3:], nil
}

func markChanges(found, rules map[string]bool, changed []pathChange) {
	for _, change := range changed {
		if !rules[change.first] && !rules[change.second] {
			continue
		}
		found[change.first] = true
		if change.second != "" {
			found[change.second] = true
		}
	}
}

func (repo history) rulesAt(commit string, cache map[string]map[string]bool) (map[string]bool, error) {
	if rules, exists := cache[commit]; exists {
		return rules, nil
	}
	tree, err := repo.sourceAt(commit)
	if err != nil {
		return nil, err
	}
	// A work-in-progress commit may hold Go that does not parse; that file
	// is protected, so only problems that hide rules stop the audit.
	result := classify(tree)
	if err := result.fatal(); err != nil {
		return nil, fmt.Errorf("classify tree %s: %w", commit, err)
	}
	cache[commit] = result.rules
	return result.rules, nil
}

func (repo history) sourceAt(commit string) (sourceTree, error) {
	raw, err := repo.command("ls-tree", "-r", "-z", commit)
	if err != nil {
		return sourceTree{}, err
	}
	objects := make(map[string]string)
	links := make(map[string]bool)
	var paths []string
	for _, entry := range splitNUL(raw) {
		header, name, valid := strings.Cut(entry, "\t")
		fields := strings.Fields(header)
		if !valid || len(fields) != 3 {
			return sourceTree{}, fmt.Errorf("malformed ls-tree entry")
		}
		paths = append(paths, name)
		links[name] = fields[0] == "120000"
		if fields[1] == "blob" && fields[0] != "120000" {
			objects[name] = fields[2]
		}
	}
	tree := sourceTree{root: repo.root, paths: paths, links: links, read: func(name string) ([]byte, error) {
		id, ok := objects[name]
		if !ok {
			return nil, fmt.Errorf("rule input %q is not a regular Git blob", name)
		}
		return repo.blobs(id)
	}}
	return tree, nil
}
