package review

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/lawslock"
	"github.com/pantacor/lawgate/rulechanges"
)

// ruleSet is what rulechanges classifies as a rule file in the worktree,
// and the proof lock's path.
type ruleSet struct {
	files map[string]bool
	lock  string
}

// readRuleFiles asks rulechanges which files are rule files. A file it could
// not classify is listed (protected) and its reason noted on the report.
func readRuleFiles(root, lock string, report *Report) ruleSet {
	rules := ruleSet{files: make(map[string]bool), lock: lock}
	listed, err := rulechanges.RuleFiles(root)
	if err != nil {
		report.Notes = append(report.Notes, "rule files: "+err.Error())
	}
	for _, file := range listed.Files {
		rules.files[file] = true
	}
	return rules
}

// contains reports whether a path is a rule file. A path the worktree no
// longer has (a deleted LAWS.md, lock or lawgate.json) is judged by its name.
func (rules ruleSet) contains(file string) bool {
	byName := path.Base(file) == lawsdoc.FileName || file == rules.lock || file == inventory.Filename
	return rules.files[file] || byName
}

// ruleChanges lists the commits that touch a rule file, approved when they
// carry the audit trailer or a later commit approves them.
func ruleChanges(root string, commits []Commit, rules ruleSet) []RuleChange {
	approvedLater := laterApprovals(commits)
	var changes []RuleChange
	for _, commit := range commits {
		files, err := ChangedFiles(root, commit.SHA)
		files = slices.DeleteFunc(files, func(file string) bool { return !rules.contains(file) })
		if err != nil || len(files) == 0 {
			continue
		}
		change := RuleChange{Commit: commit.Short, Subject: commit.Subject, Files: files, ApprovedBy: commit.Trailers[RuleChangeTrailer]}
		change.Approved = len(change.ApprovedBy) > 0 || approvedBy(approvedLater, commit.SHA)
		changes = append(changes, change)
	}
	return changes
}

func laterApprovals(commits []Commit) []string {
	var approved []string
	for _, commit := range commits {
		approved = append(approved, commit.Trailers[RuleApprovesTrailer]...)
	}
	return approved
}

func approvedBy(approved []string, sha string) bool {
	return slices.ContainsFunc(approved, func(prefix string) bool {
		return prefix != "" && strings.HasPrefix(sha, prefix)
	})
}

// lockChanges compares the proof lock at the merge base with the worktree's
// and says which laws' proofs or subjects changed. It is empty when the
// branch does not change the lock.
func lockChanges(root string, h *history, lock string, report *Report) []string {
	if !slices.ContainsFunc(h.diff, func(file FileDiff) bool { return file.Path == lock }) {
		return nil
	}
	current, currentErr := readLock(func() ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(lock))) })
	base, baseErr := readLock(func() ([]byte, error) { return ShowFile(root, h.mergeBase, lock) })
	if err := errors.Join(currentErr, baseErr); err != nil {
		report.Notes = append(report.Notes, fmt.Sprintf("%s: %v", lock, err))
		return []string{lock + " changed: proof or subject changed: needs the owner's review"}
	}
	var changes []string
	for _, problem := range lawslock.Compare(base, current, lock) {
		changes = append(changes, problem.Error())
	}
	if len(changes) == 0 {
		changes = append(changes, lock+" changed without changing a proof or subject fingerprint")
	}
	return changes
}

// readLock decodes a lock; a missing one is an empty lock, so a new lock
// shows every law as changed.
func readLock(read func() ([]byte, error)) (lawslock.Lock, error) {
	data, err := read()
	if err != nil {
		return lawslock.Lock{Version: 1, Laws: []lawslock.Entry{}}, nil
	}
	return lawslock.Decode(data)
}
