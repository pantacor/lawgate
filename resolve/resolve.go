// Package resolve binds a machine's declared sites to its Go source (layer 2
// of the proof gate): every log tag in the source must be claimed by a site,
// every site must find its tag or match, and the kind a site declares must
// agree with the call the code makes.
//
// It reads the source with go/parser and go/scanner, so a tag or a match in a
// comment never counts. What a tag is, and which call means which site kind,
// come from the product's Vocabulary: the core knows no product names.
package resolve

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/pantacor/lawgate/machine"
)

// Vocabulary is what a product tells the resolver about its code.
type Vocabulary struct {
	// TagPattern finds a log tag inside a string literal. Its first group,
	// or the whole match without a group, is the tag with its prefix; for
	// example `^([A-Z_]+_JOB(?: \d+| \([a-z-]+\))?)`.
	TagPattern *regexp.Regexp
	// Calls maps the name of a reporting call to the kind of site a tag
	// passed to it is. A name is "Func", "pkg.Func" or "recv.Method"; the
	// qualified name is tried before the bare one.
	Calls map[string]machine.SiteKind
	// LabelCalls ties a site label to the calls that make it true: a site
	// with the label must be bound to one of them (for example
	// "stores-done" to "StoreDone"). Labels not listed are not checked here.
	LabelCalls map[string][]string
}

func (v Vocabulary) findTag(text string) (string, bool) {
	match := v.TagPattern.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}
	if len(match) > 1 {
		return match[1], true
	}
	return match[0], true
}

// kindOf is the kind of the innermost call the vocabulary knows.
func (v Vocabulary) kindOf(calls []string) machine.SiteKind {
	for _, name := range calls {
		if kind, ok := v.Calls[name]; ok {
			return kind
		}
	}
	return ""
}

// ErrNoTagPattern is returned when the vocabulary cannot find tags.
var ErrNoTagPattern = errors.New("resolve: the vocabulary has no TagPattern")

// Resolve fills each site's line from src and returns every disagreement:
// source tags no site claims, sites whose tag or match is not in the code,
// sites declared as a kind the code does not show, sites whose labels lack
// their call, findings whose match is gone, and every problem
// machine.Validate finds in the graph. prefix is the machine's own tag
// prefix; tags with another prefix keep their full text, so they stand out.
func Resolve(m *machine.Machine, src []byte, vocab Vocabulary, prefix string) []error {
	if vocab.TagPattern == nil {
		return []error{ErrNoTagPattern}
	}
	parsed, err := parseSource(src, vocab, prefix)
	if err != nil {
		return []error{fmt.Errorf("resolve %s: %w", m.Name, err)}
	}
	b := &binder{source: parsed, vocab: vocab, claimed: make([]bool, len(parsed.tags))}
	m.Sites(b.bindSite)
	b.reportUnclaimed()
	b.bindFindings(m.Findings)
	return append(b.errs, machine.Validate(m)...)
}

// SiteCounts counts sites by kind, the number a reviewer looks at first.
func SiteCounts(m *machine.Machine) map[machine.SiteKind]int {
	counts := map[machine.SiteKind]int{}
	m.Sites(func(_ *machine.State, _ *machine.Step, site *machine.Site) { counts[site.Kind]++ })
	return counts
}

// compatible says whether a declared kind agrees with what the code does.
//
//   - A tag outside every known call (a constant, a variable) says nothing
//     about the kind, so any kind is accepted: a layer-2 limit.
//   - A guard is not a failure path, so it may only sit on a plain log.
//   - A silent log may be declared a fault because the scanner cannot see
//     whether the next statement returns an error.
func compatible(declared, detected machine.SiteKind) bool {
	switch {
	case detected == "", declared == detected:
		return true
	case declared == machine.SiteGuard:
		return detected == machine.SiteLog
	case detected == machine.SiteSilent:
		return declared == machine.SiteFault
	}
	return false
}
