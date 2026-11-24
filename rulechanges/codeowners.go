package rulechanges

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/pantacor/lawgate/bite"
	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
)

// ownerName accepts a user (@name, GitLab allows "." and "_"), a GitHub team
// (@org/team) and a GitLab group or subgroup (@group/sub/team).
var ownerName = regexp.MustCompile(`^@[A-Za-z0-9_][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_][A-Za-z0-9_.-]*)*$`)

// ValidOwner reports an error unless owner is a CODEOWNERS user or team handle.
func ValidOwner(owner string) error {
	if owner == "" {
		return errors.New(`no CODEOWNER: set "owner" in lawgate.json or pass --owner`)
	}
	if !ownerName.MatchString(owner) {
		return fmt.Errorf("invalid CODEOWNER %q", owner)
	}
	return nil
}

func WriteOwners(output io.Writer, files []string, owner string) error {
	if err := ValidOwner(owner); err != nil {
		return err
	}
	for _, file := range files {
		if strings.ContainsAny(file, "*?[]#!\\ \t\r\n") {
			return fmt.Errorf("cannot express %q as a literal CODEOWNERS path", file)
		}
	}
	if _, err := fmt.Fprintln(output, "# lawgate rule files"); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := fmt.Fprintf(output, "/%s %s\n", file, owner); err != nil {
			return err
		}
	}
	return nil
}

// CheckOwners reports the files the active CODEOWNERS does not give to
// owner. The root is canonicalized like RuleFiles does, so a root reached
// through a symbolic link is not mistaken for a symlinked CODEOWNERS.
func CheckOwners(root string, files []string, owner, platform string) error {
	if err := ValidOwner(owner); err != nil {
		return err
	}
	root, err := gosource.CanonicalRoot(root)
	if err != nil {
		return err
	}
	name, data, err := activeOwners(platform, func(name string) ([]byte, error) {
		return readWorktreeFile(root, name)
	})
	if err != nil {
		return err
	}

	return checkCoverage(name, data, files, owner, platform)
}

func checkCoverage(name string, data []byte, files []string, owner, platform string) error {
	if platform == "github" && len(data) >= 3_000_000 {
		return fmt.Errorf("%s exceeds the supported GitHub CODEOWNERS size", name)
	}
	sections, err := parseOwners(string(data), platform)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	var uncovered []string
	if bite.Off("lawgate/rule-changes-are-reviewed") {
		return nil
	}
	for _, file := range files {
		if !sections.require(file, owner) {
			uncovered = append(uncovered, file)
		}
	}
	if len(uncovered) > 0 {
		return fmt.Errorf("%s does not require %s for: %s", name, owner, strings.Join(uncovered, ", "))
	}
	return nil
}

func CheckOwnersAt(root, ref string, files []string, owner, platform string) error {
	if err := ValidOwner(owner); err != nil {
		return err
	}
	repo := gitHistory(root)
	id, err := repo.commitID(ref)
	if err != nil {
		return err
	}
	name, data, err := activeOwners(platform, func(name string) ([]byte, error) {
		return readBlobAt(repo, id, name)
	})
	if err != nil {
		return fmt.Errorf("CODEOWNERS at %s: %w", ref, err)
	}
	return checkCoverage(name+" at "+ref, data, files, owner, platform)
}

// ConfigAt reads lawgate.json as committed at ref. Checking a candidate
// against the base's owner keeps a change from choosing its own reviewer.
func ConfigAt(root, ref string) (inventory.Config, error) {
	repo := gitHistory(root)
	id, err := repo.commitID(ref)
	if err != nil {
		return inventory.Config{}, err
	}
	data, err := readBlobAt(repo, id, inventory.Filename)
	if err != nil {
		return inventory.Config{}, fmt.Errorf("%s at %s: %w", inventory.Filename, ref, err)
	}
	return inventory.Decode(bytes.NewReader(data))
}

func gitHistory(root string) history {
	return history{command: func(args ...string) ([]byte, error) { return gitCommand(root, args...) }}
}

func readBlobAt(repo history, commit, name string) ([]byte, error) {
	raw, err := repo.command("ls-tree", "--full-tree", "-z", commit, "--", name)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, os.ErrNotExist
	}
	header, _, _ := strings.Cut(string(raw), "\t")
	fields := strings.Fields(header)
	valid := len(fields) == 3 && fields[1] == "blob" && fields[0] != "120000"
	if !valid {
		return nil, fmt.Errorf("%s is not a regular Git blob", name)
	}
	return repo.command("cat-file", "blob", fields[2])
}

func activeOwners(platform string, read func(string) ([]byte, error)) (string, []byte, error) {
	var locations []string
	switch platform {
	case "github":
		locations = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}
	case "gitlab":
		locations = []string{"CODEOWNERS", "docs/CODEOWNERS", ".gitlab/CODEOWNERS"}
	default:
		return "", nil, fmt.Errorf("unsupported CODEOWNERS platform %q", platform)
	}
	for _, name := range locations {
		data, err := read(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		return name, data, err
	}
	return "", nil, errors.New("CODEOWNERS file is missing")
}

type ownerRule struct {
	pattern *regexp.Regexp
	names   []string
}

type ownerSection struct {
	optional bool
	defaults []string
	rules    []ownerRule
}

type ownerSections map[string]*ownerSection

func parseOwners(content, platform string) (ownerSections, error) {
	key := ""
	unnamed := &ownerSection{}
	sections := ownerSections{"": unnamed}
	for index, line := range strings.Split(content, "\n") {
		line, _, _ = strings.Cut(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		err := applyOwnerLine(sections, &key, line, platform)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", index+1, err)
		}
	}
	return sections, nil
}

func applyOwnerLine(sections ownerSections, key *string, line, platform string) error {
	header := strings.HasPrefix(line, "[") || strings.HasPrefix(line, "^[")
	if header && platform == "gitlab" {
		return applySection(sections, key, line)
	}
	fields := strings.Fields(line)
	pattern, err := compilePattern(fields[0])
	if err != nil {
		return err
	}
	section := sections[*key]
	owners := fields[1:]
	if len(owners) == 0 {
		owners = section.defaults
	}
	section.rules = append(section.rules, ownerRule{pattern, owners})
	return nil
}

var sectionHeader = regexp.MustCompile(`^(\^)?\[([^\]]+)\](?:\[([1-9][0-9]*)\])?(?:\s+(.*))?$`)

func applySection(sections ownerSections, key *string, line string) error {
	match := sectionHeader.FindStringSubmatch(line)
	if match == nil {
		return fmt.Errorf("invalid GitLab CODEOWNERS section")
	}
	*key = strings.ToLower(match[2])
	optional := match[1] != ""
	owners := strings.Fields(match[4])
	section, exists := sections[*key]
	if !exists {
		sections[*key] = &ownerSection{optional: optional, defaults: owners}
		return nil
	}
	section.optional = section.optional && optional
	if len(owners) > 0 {
		section.defaults = owners
	}
	return nil
}

func (sections ownerSections) require(file, owner string) bool {
	for _, section := range sections {
		if !section.optional && section.requires(file, owner) {
			return true
		}
	}
	return false
}

func (section *ownerSection) requires(file, owner string) bool {
	for index := len(section.rules) - 1; index >= 0; index-- {
		rule := section.rules[index]
		if !rule.pattern.MatchString(file) {
			continue
		}
		// A list of alternative owners does not guarantee this person's review.
		return len(rule.names) == 1 && strings.EqualFold(rule.names[0], owner)
	}
	return false
}
