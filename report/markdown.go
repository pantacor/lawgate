package report

import (
	"fmt"
	"os"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// Markdown renders the runs for a job summary page: the verdict in one line,
// a table of the machines, what broke or could not be checked, then, under
// their own heading, the items that wait on a decision.
func Markdown(runs []LawRun) string {
	totals := Sum(runs)
	var b strings.Builder
	b.WriteString("## Laws\n\n")
	b.WriteString(markdownHeadline(totals))
	b.WriteString(markdownTable(runs))
	if totals.Pending > 0 {
		b.WriteString("\n`…` marks a law that is checked but does not fail yet: it waits on a decision.\n")
	}
	b.WriteString(markdownSection("What broke", runs, brokenDetails))
	b.WriteString(markdownSection("Pending", runs, pendingDetails))
	return b.String()
}

func markdownHeadline(t Totals) string {
	if t.Checks > 0 {
		return markdownGateHeadline(t)
	}
	machines := plural(t.Machines, "machine", "machines")
	laws := plural(t.Checked, "law", "laws")
	switch t.Outcome() {
	case OutcomeFailed:
		return fmt.Sprintf("**✗ Infrastructure failure: %d %s could not be checked, %d %s checked.**\n\n",
			t.Failed, plural(t.Failed, "module", "modules"), t.Checked, laws)
	case OutcomeBroken:
		return fmt.Sprintf("**✗ %d of %d %s over %d %s broke.**\n\n", t.Broken, t.Checked, laws, t.Machines, machines)
	case OutcomePending:
		return fmt.Sprintf("**✓ %d %s, %d %s hold; %d %s on a decision.**\n\n",
			t.Machines, machines, t.Checked, laws, t.Pending, plural(t.Pending, "waits", "wait"))
	}
	return fmt.Sprintf("**✓ %d %s, %d %s hold.**\n\n", t.Machines, machines, t.Checked, laws)
}

func markdownGateHeadline(totals Totals) string {
	counts := fmt.Sprintf("%d %s, %d %s, %d %s",
		totals.Machines, plural(totals.Machines, "machine", "machines"),
		totals.Checked, plural(totals.Checked, "machine law", "machine laws"),
		totals.Checks, plural(totals.Checks, "gate check", "gate checks"))
	switch totals.Outcome() {
	case OutcomeFailed:
		return fmt.Sprintf("**✗ Infrastructure failure. %s.**\n\n", counts)
	case OutcomeBroken:
		return fmt.Sprintf("**✗ %d %s. %s.**\n\n", totals.Broken, plural(totals.Broken, "failure", "failures"), counts)
	case OutcomePending:
		return fmt.Sprintf("**… %d pending. %s.**\n\n", totals.Pending, counts)
	}
	return fmt.Sprintf("**✓ All checks passed. %s.**\n\n", counts)
}

func markdownTable(runs []LawRun) string {
	var b strings.Builder
	b.WriteString("| Module | Check group | Verdicts | ✓ | ✗ | … |\n| --- | --- | --: | --: | --: | --: |\n")
	for _, run := range runs {
		b.WriteString(markdownRow(run))
	}
	return b.String()
}

func markdownRow(run LawRun) string {
	if run.failure() != "" {
		return fmt.Sprintf("| `%s` | `%s` | — | — | infrastructure failure | — |\n", run.Module, run.Machine)
	}
	holding := len(run.Verdicts) - run.Broken() - run.Pending()
	return fmt.Sprintf("| `%s` | `%s` | %d | %d | %d | %d |\n",
		run.Module, run.Machine, len(run.Verdicts), holding, run.Broken(), run.Pending())
}

// markdownSection lists one kind of detail under its heading, or nothing
// when no run has any.
func markdownSection(heading string, runs []LawRun, details func(LawRun) string) string {
	var b strings.Builder
	for _, run := range runs {
		b.WriteString(details(run))
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n### " + heading + "\n" + b.String()
}

func brokenDetails(run LawRun) string {
	name := moduleMachine(run, "/")
	if reason := run.failure(); reason != "" {
		return fmt.Sprintf("\n- **`%s`** could not be checked: %s\n", name, reason)
	}
	var b strings.Builder
	for _, verdict := range run.Verdicts {
		if !verdict.Broken() {
			continue
		}
		markdownBrokenLaw(&b, name, verdict.Law)
		markdownViolations(&b, verdict)
	}
	return b.String()
}

func pendingDetails(run LawRun) string {
	if run.failure() != "" {
		return ""
	}
	name := moduleMachine(run, "/")
	var b strings.Builder
	for _, verdict := range run.Verdicts {
		if !verdict.IsPending() {
			continue
		}
		fmt.Fprintf(&b, "\n- `%s` · `%s` is pending: %s\n", name, verdict.Law.Name, verdict.Law.Pending)
		markdownViolations(&b, verdict)
	}
	return b.String()
}

func markdownViolations(b *strings.Builder, verdict machine.Verdict) {
	for _, detail := range visibleViolations(verdict) {
		markdownDetail(b, strings.TrimSpace(detail))
	}
}

func markdownBrokenLaw(b *strings.Builder, name string, law machine.Law) {
	fmt.Fprintf(b, "\n- **`%s` · `%s`**", name, law.Name)
	doc := strings.TrimSpace(law.Doc)
	switch {
	case doc == "":
		b.WriteByte('\n')
	case strings.Contains(doc, "\n"):
		b.WriteByte('\n')
		markdownDetail(b, doc)
	default:
		fmt.Fprintf(b, " — %s\n", doc)
	}
}

func markdownDetail(b *strings.Builder, text string) {
	fmt.Fprintf(b, "  - %s\n", strings.ReplaceAll(text, "\n", "\n    "))
}

// plural picks the word that agrees with n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// WriteMarkdown stores the markdown report.
func WriteMarkdown(path string, runs []LawRun) error {
	return os.WriteFile(path, []byte(Markdown(runs)), 0o644)
}
