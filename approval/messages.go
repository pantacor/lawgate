package approval

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// maxListedFiles keeps a message readable when a change touches many rule
// files; the audit output above it lists them all.
const maxListedFiles = 6

// ruleFiles lists the distinct rule files of changes, sorted.
func ruleFiles(changes []Change) string {
	var files []string
	for _, change := range changes {
		files = append(files, change.Files...)
	}
	slices.Sort(files)
	files = slices.Compact(files)
	if len(files) <= maxListedFiles {
		return strings.Join(files, ", ")
	}
	listed := strings.Join(files[:maxListedFiles], ", ")
	return fmt.Sprintf("%s and %d more", listed, len(files)-maxListedFiles)
}

// shortID is the abbreviated commit ID people see on the platform.
func shortID(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// approvalMissing says which approval is missing, why, and how to get it.
func approvalMissing(changes []Change, owner, commit string, judged verdict, fix string) error {
	return fmt.Errorf("rule files changed (%s): approval by @%s on %s is missing (%s); %s",
		ruleFiles(changes), owner, shortID(commit), judged.reason, fix)
}

// reviewMissing names a rule-file commit that reached the default branch
// without a pull request or merge request. The owner's own commits are no
// exception: the approval is what the check reads.
func reviewMissing(change Change, owner, review string) error {
	return fmt.Errorf("rule files changed (%s): commit %s reached the default branch without a %s; every rule-file change needs a %s that @%s approves, even a commit @%s wrote; revert it and open a %s",
		ruleFiles([]Change{change}), shortID(change.Commit), review, review, owner, owner, review)
}

func approvedLine(changes []Change, owner, where string) string {
	return fmt.Sprintf("rule files changed (%s): approved by @%s %s", ruleFiles(changes), owner, where)
}

// passLine says why a pull request or merge request passes: the owner's
// approval, or the owner's authorship.
func passLine(changes []Change, owner string, judged verdict, where string) string {
	if judged.reason == "" {
		return approvedLine(changes, owner, where)
	}
	return fmt.Sprintf("rule files changed (%s): %s, %s", ruleFiles(changes), judged.reason, where)
}

// authoredNotes names the judged requests that passed by the owner's
// authorship, after a push line; label formats one request's number.
func authoredNotes(verdicts map[int]verdict, label string) string {
	var notes []string
	for _, number := range slices.Sorted(maps.Keys(verdicts)) {
		judged := verdicts[number]
		if judged.approved && judged.reason != "" {
			notes = append(notes, fmt.Sprintf(label, number)+" "+judged.reason)
		}
	}
	if len(notes) == 0 {
		return ""
	}
	return "; " + strings.Join(notes, "; ")
}
