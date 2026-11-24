package lsp

import (
	"fmt"
	"maps"
	"strings"

	"github.com/pantacor/lawgate/review"
	"github.com/pantacor/lawgate/review/index"
)

type diagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

// LSP diagnostic severities.
const (
	severityError   = 1
	severityWarning = 2
	severityInfo    = 3
)

// diagnostics collects the diagnostics of one publication, by URI.
type diagnostics struct {
	server *server
	byURI  map[string][]diagnostic
}

func (d *diagnostics) add(file string, line, severity int, message string) {
	uri := d.server.uri(file)
	found := diagnostic{Range: lineRange(line), Severity: severity, Source: "lawgate", Message: message}
	d.byURI[uri] = append(d.byURI[uri], found)
}

// publishAll sends the diagnostics: declaration problems on their line,
// broken and pending laws and unresolved subjects or tests on their LAWS.md
// heading, and references in open documents to laws no LAWS.md writes.
func (s *server) publishAll() {
	idx := s.index()
	found := &diagnostics{server: s, byURI: make(map[string][]diagnostic)}
	for _, problem := range idx.Problems {
		found.add(problem.File, problem.Line, severityError, problemText(problem))
	}
	for _, law := range idx.Laws {
		lawDiagnostics(found, law)
	}
	s.mu.Lock()
	open := maps.Clone(s.docs)
	s.mu.Unlock()
	for uri, text := range open {
		s.unwrittenLaws(found, idx, uri, text)
	}
	s.send(found.byURI)
}

func problemText(problem review.Problem) string {
	if problem.Machine == "" {
		return problem.Message
	}
	return fmt.Sprintf("machine %s: %s", problem.Machine, problem.Message)
}

func lawDiagnostics(found *diagnostics, law *review.Law) {
	if law.File == "" {
		return
	}
	for _, verdict := range law.Verdicts {
		switch verdict.Status {
		case review.OutcomeBroken:
			found.add(law.File, law.Line, severityError, fmt.Sprintf("law %s is broken on %s: %s", law.Name, verdict.Machine, strings.Join(verdict.Violations, "; ")))
		case review.OutcomePending:
			found.add(law.File, law.Line, severityInfo, fmt.Sprintf("law %s is pending on %s: %s", law.Name, verdict.Machine, verdict.Pending))
		}
	}
	for _, ref := range append(law.Subjects, law.Tests...) {
		if ref.Problem != "" {
			found.add(law.File, law.Line, severityWarning, ref.Problem)
		}
	}
}

func (s *server) unwrittenLaws(found *diagnostics, idx *review.Index, uri, text string) {
	file, inside := s.rel(uri)
	if !inside {
		return
	}
	for number, line := range strings.Split(text, "\n") {
		for _, name := range unwritten(idx, file, index.LawRefs(line)) {
			found.add(file, number+1, severityWarning, fmt.Sprintf("law %s is not written in any LAWS.md", name))
		}
	}
}

func unwritten(idx *review.Index, file string, names []string) []string {
	var missing []string
	for _, name := range names {
		if idx.LawFor(name, file) == nil {
			missing = append(missing, name)
		}
	}
	return missing
}

// send publishes the diagnostics and clears those of URIs that had some.
func (s *server) send(byURI map[string][]diagnostic) {
	s.mu.Lock()
	previous := s.published
	s.published = make(map[string]bool, len(byURI))
	for uri := range byURI {
		s.published[uri] = true
	}
	s.mu.Unlock()
	for uri, found := range byURI {
		s.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "diagnostics": found})
	}
	for uri := range previous {
		if _, still := byURI[uri]; !still {
			s.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "diagnostics": []diagnostic{}})
		}
	}
}
