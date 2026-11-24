package resolve

import (
	"fmt"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// binder ties sites to the tags and lines of one source, each tag to at most
// one site.
type binder struct {
	*source
	vocab   Vocabulary
	claimed []bool
	errs    []error
}

func (b *binder) fail(format string, args ...any) {
	b.errs = append(b.errs, fmt.Errorf(format, args...))
}

func (b *binder) bindSite(st *machine.State, step *machine.Step, site *machine.Site) {
	where := st.ID + "/" + step.ID
	switch {
	case site.Tag != "":
		b.bindTag(where, site)
	case site.Match != "":
		b.bindMatch(where, site)
	default:
		b.fail("%s: site has neither tag nor match", where)
	}
}

func (b *binder) bindTag(where string, site *machine.Site) {
	index := b.findTag(site)
	if index < 0 {
		b.fail("%s: tag %q (match %q) is not in the code", where, site.Tag, site.Match)
		return
	}
	b.claimed[index] = true
	tag := b.tags[index]
	site.Line = tag.Line
	b.checkKind(where, site, tag.Detected)
	b.checkLabels(where, site, tag.Calls)
}

// findTag returns the first unclaimed source tag a site names, or -1. With a
// Match, the tag's line or the line before it must hold the match in code.
func (b *binder) findTag(site *machine.Site) int {
	for i, tag := range b.tags {
		free := !b.claimed[i] && tag.Tag == site.Tag
		if free && b.matchesNear(site.Match, tag.Line) {
			return i
		}
	}
	return -1
}

func (b *binder) matchesNear(match string, line int) bool {
	if match == "" {
		return true
	}
	return b.codeHas(line, match) || b.codeHas(line-1, match)
}

func (b *binder) codeHas(line int, match string) bool {
	if line < 1 || line > len(b.codeLines) {
		return false
	}
	return strings.Contains(b.codeLines[line-1], match)
}

func (b *binder) bindMatch(where string, site *machine.Site) {
	line, start := b.firstCodeMatch(site.Match)
	if line == 0 {
		b.fail("%s: match %q is not in the code", where, site.Match)
		return
	}
	site.Line = line
	calls := b.callsAt(start, start+len(site.Match))
	b.checkKind(where, site, b.vocab.kindOf(calls))
	b.checkLabels(where, site, calls)
}

// firstCodeMatch returns the line and byte offset of the first occurrence
// in code, or zero for both when the match is absent.
func (b *binder) firstCodeMatch(match string) (line, offset int) {
	for i, code := range b.codeLines {
		if column := strings.Index(code, match); column >= 0 {
			return i + 1, offset + column
		}
		offset += len(code) + 1
	}
	return 0, 0
}

func (b *binder) checkKind(where string, site *machine.Site, detected machine.SiteKind) {
	if !compatible(site.Kind, detected) {
		b.fail("%s: site at line %d is declared %s but the code is %s", where, site.Line, site.Kind, detected)
	}
}

// checkLabels checks that a site carrying a label the vocabulary ties to
// calls is bound to one of those calls.
func (b *binder) checkLabels(where string, site *machine.Site, calls []string) {
	for _, label := range site.Labels {
		required, checked := b.vocab.LabelCalls[label]
		if checked && !anyOf(calls, required) {
			b.fail("%s: site at line %d is labelled %s but calls none of %s", where, site.Line, label, strings.Join(required, ", "))
		}
	}
}

func anyOf(have, want []string) bool {
	for _, name := range want {
		for _, call := range have {
			if call == name {
				return true
			}
		}
	}
	return false
}

func (b *binder) reportUnclaimed() {
	for i, tag := range b.tags {
		if !b.claimed[i] {
			b.fail("line %d: tag %q (%s) is not claimed by any site", tag.Line, tag.Tag, kindName(tag.Detected))
		}
	}
}

func kindName(kind machine.SiteKind) string {
	if kind == "" {
		return "no known call"
	}
	return string(kind)
}

func (b *binder) bindFindings(findings []machine.Finding) {
	for i := range findings {
		finding := &findings[i]
		if finding.Match == "" {
			continue
		}
		finding.Line, _ = b.firstCodeMatch(finding.Match)
		if finding.Line == 0 {
			b.fail("finding %q: match %q is not in the code", finding.Text, finding.Match)
		}
	}
}
