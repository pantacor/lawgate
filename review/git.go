package review

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Commit is why a line changed: the commit that last touched it.
type Commit struct {
	SHA      string              `json:"sha"`
	Short    string              `json:"short"`
	Author   string              `json:"author"`
	Date     string              `json:"date"`
	Subject  string              `json:"subject"`
	Body     string              `json:"body,omitempty"`
	Trailers map[string][]string `json:"trailers,omitempty"`
	// Uncommitted is set for a line changed in the working tree.
	Uncommitted bool `json:"uncommitted,omitempty"`
	// MergeRequest is the pull or merge request that brought the commit,
	// when it was looked up.
	MergeRequest string `json:"merge_request,omitempty"`
}

// The audit trailers a rule change carries (rulechanges.Audit reads them).
// Approval itself is the owner's review on the platform (CODEOWNERS).
const (
	RuleChangeTrailer   = "Rule-change-approved-by"
	RuleApprovesTrailer = "Rule-change-approves"
)

const gitTimeout = 20 * time.Second

// git runs a read-only git command in root.
func git(root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = root
	out, err := command.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(exit.Stderr)))
	}
	return string(out), err
}

var zeroSHA = regexp.MustCompile(`^0{40}$`)

// Blame returns the commit that last changed a line of a file (relative to
// root), or an uncommitted marker for a line changed in the working tree.
func Blame(root, file string, line int) (*Commit, error) {
	out, err := git(root, "blame", "--porcelain", "-L", fmt.Sprintf("%d,%d", line, line), "--", file)
	if err != nil {
		return nil, err
	}
	first, _, _ := strings.Cut(out, "\n")
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return nil, fmt.Errorf("git blame: no output for %s:%d", file, line)
	}
	if zeroSHA.MatchString(fields[0]) {
		return &Commit{Uncommitted: true, Subject: "not committed yet"}, nil
	}
	return CommitInfo(root, fields[0])
}

const commitFormat = "%H%x00%h%x00%an%x00%as%x00%s%x00%b%x00%(trailers:only,unfold)%x1e"

// CommitInfo reads one commit.
func CommitInfo(root, sha string) (*Commit, error) {
	out, err := git(root, "show", "-s", "--format="+commitFormat, sha, "--")
	if err != nil {
		return nil, err
	}
	commits := parseCommits(out)
	if len(commits) == 0 {
		return nil, fmt.Errorf("git show %s: no commit", sha)
	}
	return &commits[0], nil
}

func parseCommits(out string) []Commit {
	var commits []Commit
	for _, record := range strings.Split(out, "\x1e") {
		fields := strings.Split(strings.TrimLeft(record, "\n"), "\x00")
		if len(fields) < 7 {
			continue
		}
		commit := Commit{SHA: fields[0], Short: fields[1], Author: fields[2], Date: fields[3], Subject: fields[4]}
		commit.Trailers = parseTrailers(fields[6])
		commit.Body = strings.TrimSpace(stripTrailers(fields[5], fields[6]))
		commits = append(commits, commit)
	}
	return commits
}

func parseTrailers(text string) map[string][]string {
	trailers := map[string][]string{}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		isTrailer := ok && key != "" && !strings.Contains(key, " ")
		if isTrailer {
			trailers[key] = append(trailers[key], strings.TrimSpace(value))
		}
	}
	if len(trailers) == 0 {
		return nil
	}
	return trailers
}

func stripTrailers(body, trailers string) string {
	block := strings.TrimSpace(trailers)
	if block == "" {
		return body
	}
	return strings.TrimSuffix(strings.TrimSpace(body), block)
}

// Commits lists the commits in base..head, newest first.
func Commits(root, base, head string) ([]Commit, error) {
	out, err := git(root, "log", "--format="+commitFormat, base+".."+head, "--")
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

// CommitsTouching lists the SHAs of the commits in base..HEAD that changed
// lines start..end of file (as the file is at HEAD), newest first.
func CommitsTouching(root, base, file string, start, end int) ([]string, error) {
	end = max(end, start)
	out, err := git(root, "log", "--format=@@@%H", "-L", fmt.Sprintf("%d,%d:%s", start, end, file), base+"..HEAD")
	if err != nil {
		return nil, err
	}
	var shas []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		if sha, ok := strings.CutPrefix(scanner.Text(), "@@@"); ok {
			shas = append(shas, sha)
		}
	}
	return shas, scanner.Err()
}

// MergeBase returns the merge base of base and HEAD.
func MergeBase(root, base string) (string, error) {
	out, err := git(root, "merge-base", base, "HEAD")
	return strings.TrimSpace(out), err
}

// Head returns HEAD's SHA.
func Head(root string) (string, error) {
	out, err := git(root, "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

// ChangedFiles lists the files a commit changes (renames by their new
// name; a pure rename changes no content and is left out).
func ChangedFiles(root, sha string) ([]string, error) {
	out, err := git(root, "diff-tree", "--no-commit-id", "--root", "-r", "-M", "--name-status", sha)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 || fields[0] == "R100" {
			continue
		}
		files = append(files, fields[len(fields)-1])
	}
	return files, nil
}

// ShowFile reads a file as it is at a commit.
func ShowFile(root, rev, file string) ([]byte, error) {
	out, err := git(root, "show", rev+":"+file)
	return []byte(out), err
}

// DefaultBase is the branch a review compares against when none is given:
// the remote's default branch (refs/remotes/origin/HEAD), else the first of
// origin/main, main, origin/master and master that exists.
func DefaultBase(root string) string {
	if out, err := git(root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimSpace(out)
	}
	for _, candidate := range []string{"origin/main", "main", "origin/master", "master"} {
		if _, err := git(root, "rev-parse", "--verify", "--quiet", candidate+"^{commit}"); err == nil {
			return candidate
		}
	}
	return "origin/main"
}
