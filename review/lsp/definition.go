package lsp

import (
	"encoding/json"
	"strings"

	"github.com/pantacor/lawgate/review"
)

// definition jumps from a law name to its LAWS.md section, from a site to
// its declaration and back, from a subject or test to its law, and from a
// subject or test named in LAWS.md to its function.
func (s *server) definition(params json.RawMessage) (any, error) {
	var p documentPosition
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	at, inside := s.place(p)
	if !inside {
		return nil, nil
	}
	if law := lawAt(at.idx, at.file, at.text, p.Position.Character); law != nil {
		elsewhere := law.File != at.file || law.Line != at.line
		if law.File != "" && elsewhere {
			return s.locate(law.File, law.Line), nil
		}
	}
	locations := s.siteJumps(at)
	locations = append(locations, s.lawJumps(at)...)
	locations = append(locations, s.refJumps(at, wordAt(at.text, p.Position.Character))...)
	if len(locations) == 0 {
		return nil, nil
	}
	return locations, nil
}

func (s *server) locate(file string, line int) location {
	return location{URI: s.uri(file), Range: lineRange(line)}
}

func (s *server) siteJumps(at hoverPlace) []location {
	var locations []location
	for _, site := range at.idx.SitesAt(at.file, at.line) {
		if site.DeclFile != "" {
			locations = append(locations, s.locate(site.DeclFile, site.DeclLine))
		}
	}
	for _, site := range at.idx.DeclsAt(at.file, at.line) {
		locations = append(locations, s.locate(site.File, site.Line))
	}
	return locations
}

// lawJumps go from a subject's or a test's declaration line to its law.
func (s *server) lawJumps(at hoverPlace) []location {
	refs := refsStartingAt(at.idx.SubjectsIn(at.file, at.line, at.line), at.line)
	refs = append(refs, refsStartingAt(at.idx.TestsIn(at.file, at.line, at.line), at.line)...)
	var locations []location
	for _, ref := range refs {
		if law := at.idx.Laws[ref.Law]; law != nil && law.File != "" {
			locations = append(locations, s.locate(law.File, law.Line))
		}
	}
	return locations
}

// refJumps go from a subject or test a LAWS.md section names to its
// function.
func (s *server) refJumps(at hoverPlace, word string) []location {
	if word == "" || !strings.HasSuffix(at.file, "LAWS.md") {
		return nil
	}
	var refs []review.CodeRef
	for _, law := range at.idx.LawsIn(at.file, at.line, at.line) {
		refs = append(append(refs, law.Subjects...), law.Tests...)
	}
	var locations []location
	for _, ref := range refs {
		if namesRef(word, ref) {
			locations = append(locations, s.locate(ref.File, ref.Line))
		}
	}
	return locations
}

// namesRef reports whether a word of LAWS.md names a resolved reference: its
// whole name, or its last dotted part.
func namesRef(word string, ref review.CodeRef) bool {
	last := ref.Name[strings.LastIndex(ref.Name, ".")+1:]
	named := word == ref.Name || word == last
	return named && ref.File != ""
}

// wordAt is the reference-like word (letters, digits, _ . / -) under a
// position of a line.
func wordAt(line string, char16 int) string {
	column := byteColumn(line, char16)
	start, end := column, column
	for start > 0 && isWordByte(line[start-1]) {
		start--
	}
	for end < len(line) && isWordByte(line[end]) {
		end++
	}
	return strings.Trim(line[start:end], ".")
}

func isWordByte(c byte) bool {
	lower := 'a' <= c && c <= 'z'
	upper := 'A' <= c && c <= 'Z'
	digit := '0' <= c && c <= '9'
	alphanumeric := lower || upper || digit
	return alphanumeric || strings.IndexByte("_./-", c) >= 0
}
