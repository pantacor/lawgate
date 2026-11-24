package review

import (
	"fmt"
	"strings"
)

func (w *markdownWriter) changes() {
	w.WriteString("### Changes\n\n")
	plain := 0
	for _, file := range w.report.Files {
		plain += w.fileChanges(file)
	}
	if plain > 0 {
		w.printf("%d other changes touch no declared state, site, subject, test or law.\n\n", plain)
	}
}

// fileChanges writes a file's governed hunks and returns how many it left
// out because nothing governs them.
func (w *markdownWriter) fileChanges(file FileReport) int {
	var governed []HunkReport
	for _, hunk := range file.Hunks {
		if hunk.governed() {
			governed = append(governed, hunk)
		}
	}
	plain := len(file.Hunks) - len(governed)
	flagged := file.RuleFile || file.LockFile
	if len(governed) == 0 && !flagged {
		return plain
	}
	w.printf("<details><summary>%s — %d of %d changes governed</summary>\n\n", fileTitle(file), len(governed), len(file.Hunks))
	w.printf("%s\n\n", w.link(file.Path, 0))
	for _, hunk := range governed {
		w.hunk(file.Path, hunk)
	}
	w.WriteString("\n</details>\n\n")
	return plain
}

func fileTitle(file FileReport) string {
	title := file.Path
	if file.LockFile {
		title += " · proof lock"
	} else if file.RuleFile {
		title += " · rule file"
	}
	if file.Status != "modified" {
		title += " · " + file.Status
	}
	return title
}

func (w *markdownWriter) hunk(file string, hunk HunkReport) {
	w.printf("- %s", w.link(file, hunk.NewStart))
	// git names the enclosing function of code; in Markdown it names a
	// stray line.
	if hunk.Header != "" && !strings.HasSuffix(file, ".md") {
		w.printf(" in `%s`", strings.TrimSpace(hunk.Header))
	}
	w.printf(" (+%d −%d)\n", len(hunk.Added), len(hunk.Removed))
	if len(hunk.States) > 0 {
		w.printf("  - states: %s\n", code(hunk.States))
	}
	for _, site := range hunk.Sites {
		w.printf("  - site `%s` (`%s/%s`) %s: %s\n", siteName(site), site.State, site.Step, site.Kind, site.Effect)
	}
	for _, site := range hunk.Declarations {
		w.printf("  - declaration of site `%s` (`%s/%s`, at %s) changed\n", siteName(site), site.State, site.Step, w.link(site.File, site.Line))
	}
	for _, ref := range hunk.Subjects {
		w.printf("  - subject `%s` of law %s changed: its tests must still catch every mutant\n", ref.Name, w.lawLabels([]string{ref.Law}))
	}
	for _, ref := range hunk.Tests {
		w.printf("  - test `%s`, which proves law %s, changed\n", ref.Name, w.lawLabels([]string{ref.Law}))
	}
	w.hunkLaws(hunk)
	w.why(hunk)
}

func (w *markdownWriter) hunkLaws(hunk HunkReport) {
	if len(hunk.Laws) > 0 {
		w.printf("  - laws: %s\n", w.lawLabels(hunk.Laws))
	}
	if len(hunk.LawText) > 0 {
		w.printf("  - law text changed: %s\n", w.lawLabels(hunk.LawText))
	}
}

func (w *markdownWriter) why(hunk HunkReport) {
	for _, sha := range hunk.Commits {
		w.printf("  - why: `%s` %s\n", sha, w.commits[sha].Subject)
	}
	if hunk.Uncommitted {
		w.WriteString("  - why: not committed yet\n")
	}
}

func (w *markdownWriter) commitList() {
	if len(w.report.Commits) == 0 {
		return
	}
	w.WriteString("<details><summary>Commits</summary>\n\n")
	for _, commit := range w.report.Commits {
		w.printf("- `%s` %s", commit.Short, commit.Subject)
		if commit.MergeRequest != "" {
			w.printf(" ([request](%s))", commit.MergeRequest)
		}
		w.WriteString("\n")
		w.quote(firstParagraph(commit.Body))
	}
	w.WriteString("\n</details>\n")
}

func (w *markdownWriter) quote(text string) {
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		w.printf("  > %s\n", line)
	}
}

// lawLabels names laws by their name; a law whose name another module also
// uses says where it is written.
func (w *markdownWriter) lawLabels(keys []string) string {
	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		labels = append(labels, w.lawLabel(key))
	}
	return strings.Join(labels, ", ")
}

func (w *markdownWriter) lawLabel(key string) string {
	for _, law := range w.report.Laws {
		renamed := law.Key == key && law.Key != law.Name
		if renamed {
			return "`" + law.Name + "` (" + law.File + ")"
		}
	}
	return "`" + key + "`"
}

func statusIcon(status string) string {
	switch status {
	case OutcomeHolds:
		return "✅"
	case OutcomeBroken:
		return "❌"
	case OutcomePending:
		return "⏳"
	case OutcomeUnknown:
		return "❓"
	}
	return "➖"
}

// StatusIcon is the mark a law's outcome is shown with.
func StatusIcon(status string) string {
	return statusIcon(status)
}

func whereList(places []string, limit int) string {
	shown := make([]string, 0, limit+1)
	for i, place := range places {
		if i == limit {
			shown = append(shown, fmt.Sprintf("+%d more", len(places)-limit))
			break
		}
		shown = append(shown, "`"+place+"`")
	}
	return strings.Join(shown, "<br>")
}

func code(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "`" + name + "`"
	}
	return strings.Join(quoted, ", ")
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func firstParagraph(text string) string {
	paragraph, _, _ := strings.Cut(strings.TrimSpace(text), "\n\n")
	return paragraph
}
