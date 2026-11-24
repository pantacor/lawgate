// Package lawsdoc reads a module's LAWS.md (layer 1 of the proof gate) and
// checks it against the code. Each law is a "## name" or "### name" section
// whose name is kebab-case; any other heading groups the laws below it:
//
//	### law-name
//
//	One sentence: what the law forbids.
//
//	- Source: <doc section or dated owner decision>
//	- Enforced by: machine `<machine-name>` | test `TestName` | script `path` | not yet. <why> | proposed
//	- Status: agreed <YYYY-MM-DD> | proposed
//	- Bites: not yet. <why> (optional gap for a test or script)
//
// "not yet" records a known gap: an agreed law nothing enforces, with the
// reason. Humans own the file; this package only checks that it and the code
// agree.
package lawsdoc

import (
	"regexp"
	"strings"

	"github.com/pantacor/lawgate/bite"
)

// FileName is the name of a module's laws file, at the module root.
const FileName = "LAWS.md"

// The fields a law section carries.
const (
	FieldSource     = "Source"
	FieldEnforcedBy = "Enforced by"
	FieldStatus     = "Status"
	FieldBites      = "Bites"
)

var (
	lawHeadingRE = regexp.MustCompile("^#{2,3} `?([a-z0-9]+(?:-[a-z0-9]+)*)`?\\s*$")
	// A heading under the framework's prefix is read as a law so that it
	// can be refused: as a grouping heading it would pass unnoticed.
	reservedHeadingRE = regexp.MustCompile("^#{2,3} `?(" + regexp.QuoteMeta(bite.FrameworkPrefix) + "[^`\\s]*)`?\\s*$")
	headingRE         = regexp.MustCompile(`^#{1,6} `)
	fields            = []string{FieldSource, FieldEnforcedBy, FieldStatus, FieldBites}
)

// Section is one law of a LAWS.md.
type Section struct {
	Name   string
	Line   int // 1-based line of the heading
	Fields map[string]string
}

// Parse splits a LAWS.md into its law sections, in order. A field's value
// runs over its indented continuation lines; fenced code blocks are skipped.
func Parse(doc string) []Section {
	p := &docParser{}
	for i, line := range strings.Split(doc, "\n") {
		p.line(i+1, strings.TrimRight(line, " \r"))
	}
	return p.sections
}

// Names lists the laws a LAWS.md documents, in order.
func Names(doc string) []string {
	var names []string
	for _, section := range Parse(doc) {
		names = append(names, section.Name)
	}
	return names
}

type docParser struct {
	sections []Section
	current  *Section
	field    string
	fenced   bool
}

func (p *docParser) line(number int, line string) {
	if strings.HasPrefix(strings.TrimSpace(line), "```") {
		p.fenced = !p.fenced
		p.field = ""
		return
	}
	if p.fenced {
		return
	}
	if p.heading(number, line) || p.current == nil {
		return
	}
	if item, ok := strings.CutPrefix(line, "- "); ok {
		p.bullet(item)
		return
	}
	p.continuation(line)
}

// heading starts a law section, or ends the current one at a grouping
// heading. It reports whether line was a heading.
func (p *docParser) heading(number int, line string) bool {
	if match := lawHeading(line); match != nil {
		values := map[string]string{}
		section := Section{Name: match[1], Line: number, Fields: values}
		p.sections = append(p.sections, section)
		p.current = &p.sections[len(p.sections)-1]
		p.field = ""
		return true
	}
	if headingRE.MatchString(line) {
		p.current, p.field = nil, ""
		return true
	}
	return false
}

func lawHeading(line string) []string {
	if match := lawHeadingRE.FindStringSubmatch(line); match != nil {
		return match
	}
	return reservedHeadingRE.FindStringSubmatch(line)
}

// Reserved reports whether the law's name is under the prefix the framework
// keeps for its own bite selectors.
func (section Section) Reserved() bool {
	return strings.HasPrefix(section.Name, bite.FrameworkPrefix)
}

func (p *docParser) bullet(item string) {
	p.field = ""
	for _, key := range fields {
		if value, ok := strings.CutPrefix(item, key+":"); ok {
			p.setField(key, strings.TrimSpace(value))
			return
		}
	}
}

func (p *docParser) setField(key, value string) {
	p.field = key
	if previous, dup := p.current.Fields[key]; dup {
		p.current.Fields[key] = previous + "\n" + value
		return
	}
	p.current.Fields[key] = value
}

// continuation adds an indented line to the field above it; anything else
// ends the field.
func (p *docParser) continuation(line string) {
	indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
	text := strings.TrimSpace(line)
	continues := p.field != "" && indented && text != ""
	if !continues {
		p.field = ""
		return
	}
	p.current.Fields[p.field] = strings.TrimSpace(p.current.Fields[p.field] + " " + text)
}
