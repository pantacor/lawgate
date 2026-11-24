package review

import (
	"fmt"
	"strconv"
	"strings"
)

// ReportMarker starts every Markdown report, so a CI job can find and update
// its own comment on a pull or merge request instead of adding a new one.
const ReportMarker = "<!-- lawgate-review -->"

// Markdown renders a report for a pull or merge request comment or a
// terminal. blobURL, when set, is the URL files are linked under, for
// example "https://github.com/org/repo/blob/<sha>".
func Markdown(report *Report, blobURL string) string {
	w := &markdownWriter{report: report, blobURL: blobURL, commits: make(map[string]Commit)}
	for _, commit := range report.Commits {
		w.commits[commit.Short] = commit
	}
	w.WriteString(ReportMarker + "\n## Lawgate review\n\n")
	w.summary()
	w.laws()
	w.ruleChanges()
	w.lockChanges()
	w.problems()
	w.notes()
	w.changes()
	w.commitList()
	return w.String()
}

type markdownWriter struct {
	strings.Builder
	report  *Report
	blobURL string
	commits map[string]Commit
}

func (w *markdownWriter) printf(format string, args ...any) {
	fmt.Fprintf(w, format, args...)
}

// link names a file (and line) as code, linked under blobURL when set.
func (w *markdownWriter) link(file string, line int) string {
	label := file
	anchor := ""
	if line > 0 {
		label += ":" + strconv.Itoa(line)
		anchor = "#L" + strconv.Itoa(line)
	}
	if w.blobURL == "" {
		return "`" + label + "`"
	}
	return "[`" + label + "`](" + strings.TrimSuffix(w.blobURL, "/") + "/" + file + anchor + ")"
}

func (w *markdownWriter) summary() {
	r := w.report
	hunks := 0
	for _, file := range r.Files {
		hunks += len(file.Hunks)
	}
	w.printf("%d files, %d changes, %d commits since `%s` (%s)", len(r.Files), hunks, len(r.Commits), r.Base, short(r.MergeBase))
	if len(r.Scope) > 0 {
		w.printf(" in %s", code(r.Scope))
	}
	w.printf(". **%d laws touched**", len(r.Laws))
	if broken := countLaws(r.Laws, OutcomeBroken); broken > 0 {
		w.printf(", **%d broken**", broken)
	}
	w.WriteString(". " + w.ruleSummary())
	if len(r.LockChanges) > 0 {
		w.WriteString(" **The proof lock changes: the owner reviews it.**")
	}
	if len(r.Problems) > 0 {
		w.printf(" **%d declaration problems.**", len(r.Problems))
	}
	w.WriteString("\n\n")
}

func (w *markdownWriter) ruleSummary() string {
	changes := w.report.RuleChanges
	unaudited := 0
	for _, change := range changes {
		if !change.Approved {
			unaudited++
		}
	}
	switch {
	case len(changes) == 0:
		return "No rule changes."
	case unaudited == 0:
		return fmt.Sprintf("%d rule changes, all with an approval trailer.", len(changes))
	}
	return fmt.Sprintf("**%d of %d rule changes have no approval trailer: the owner reviews them.**", unaudited, len(changes))
}

func countLaws(laws []LawImpact, status string) int {
	count := 0
	for _, law := range laws {
		if law.Status == status {
			count++
		}
	}
	return count
}

func (w *markdownWriter) laws() {
	if len(w.report.Laws) == 0 {
		return
	}
	w.WriteString("### Laws this branch touches\n\n| Law | Status | Law text | Where |\n|---|---|---|---|\n")
	for _, law := range w.report.Laws {
		text := ""
		if law.TextChanged {
			text = "changed"
		}
		w.printf("| %s | %s | %s | %s |\n", w.lawName(law), lawStatusText(law), text, whereList(law.Where, 3))
	}
	w.WriteString("\n")
	for _, law := range w.report.Laws {
		w.verdicts(law)
	}
}

func (w *markdownWriter) lawName(law LawImpact) string {
	if law.File == "" {
		return "`" + law.Name + "`"
	}
	return w.link(law.File, law.Line) + " " + law.Name
}

func lawStatusText(law LawImpact) string {
	switch {
	case !law.Known:
		return "❓ not written in a LAWS.md"
	case law.Status == OutcomeUnchecked:
		return "➖ proved by tests or scripts (`lawgate check`)"
	}
	return statusIcon(law.Status) + " " + law.Status
}

// verdicts lists the machines on which a law does not hold.
func (w *markdownWriter) verdicts(law LawImpact) {
	for _, verdict := range law.Verdicts {
		if verdict.Status == OutcomeHolds {
			continue
		}
		w.printf("- %s `%s` on `%s`", statusIcon(verdict.Status), law.Name, verdict.Machine)
		if verdict.Pending != "" {
			w.printf(" (pending: %s)", verdict.Pending)
		}
		w.WriteString("\n")
		for _, violation := range verdict.Violations {
			w.printf("  - %s\n", violation)
		}
	}
}

func (w *markdownWriter) ruleChanges() {
	if len(w.report.RuleChanges) == 0 {
		return
	}
	w.WriteString("### Rule changes\n\nThe owner's review on the platform (CODEOWNERS) approves a rule change; a trailer is an audit note.\n\n")
	for _, change := range w.report.RuleChanges {
		w.printf("- `%s` %s: %s (%s)\n", change.Commit, change.Subject, approvalText(change), strings.Join(change.Files, ", "))
	}
	w.WriteString("\n")
}

func approvalText(change RuleChange) string {
	switch {
	case len(change.ApprovedBy) > 0:
		return "✅ approved by " + strings.Join(change.ApprovedBy, ", ")
	case change.Approved:
		return "✅ approved by a later commit"
	}
	return "⚠️ no `" + RuleChangeTrailer + ":` trailer: needs the owner's review"
}

func (w *markdownWriter) lockChanges() {
	if len(w.report.LockChanges) == 0 {
		return
	}
	w.WriteString("### Proof lock\n\nA proof or subject changed: needs the owner's review.\n\n")
	for _, change := range w.report.LockChanges {
		w.printf("- %s\n", change)
	}
	w.WriteString("\n")
}

func (w *markdownWriter) problems() {
	if len(w.report.Problems) == 0 {
		return
	}
	w.WriteString("### Declaration problems\n\n")
	for _, problem := range w.report.Problems {
		owner := problem.Machine
		if owner == "" {
			owner = problem.Module
		}
		w.printf("- `%s` %s: %s\n", owner, w.link(problem.File, problem.Line), problem.Message)
	}
	w.WriteString("\n")
}

func (w *markdownWriter) notes() {
	if len(w.report.Notes) == 0 {
		return
	}
	w.WriteString("### Notes\n\n")
	for _, note := range w.report.Notes {
		w.printf("- %s\n", note)
	}
	w.WriteString("\n")
}
