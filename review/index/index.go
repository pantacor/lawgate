// Package index is the machine index an adopting module's runner writes with
// `-index FILE` and `lawgate review` reads: every declared machine with its
// states, its sites resolved to source lines, its laws' verdicts and the
// drift between the declaration and the code.
//
// The runner and the review tool run in different processes (the runner is
// built from the adopter's module, the review tool from lawgate), so they
// share this JSON document, not Go values. Version says which shape it is.
// Paths are relative to the module root the runner runs in.
package index

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
)

// Version is the document version this package writes and reads.
const Version = 1

// The verdict statuses of a law on one machine.
const (
	StatusHolds   = "holds"
	StatusBroken  = "broken"
	StatusPending = "pending"
)

// Document is what `-index` writes.
type Document struct {
	Version  int       `json:"version"`
	Machines []Machine `json:"machines"`
}

// Machine is one declared machine, resolved against its source.
type Machine struct {
	Name     string    `json:"name"`
	Source   string    `json:"source"`
	Doc      string    `json:"doc,omitempty"`
	States   []State   `json:"states"`
	Sites    []Site    `json:"sites"`
	Laws     []Law     `json:"laws"`
	Problems []Problem `json:"problems,omitempty"`
	// Failure says why the machine could not be indexed, or why its laws
	// could not be checked. Whatever was indexed before it is kept.
	Failure string `json:"failure,omitempty"`
}

// State is a declared state.
type State struct {
	ID       string   `json:"id"`
	Func     string   `json:"func,omitempty"`
	Status   string   `json:"status,omitempty"`
	Doc      string   `json:"doc,omitempty"`
	When     string   `json:"when,omitempty"`
	WokenBy  string   `json:"woken_by,omitempty"`
	Terminal bool     `json:"terminal,omitempty"`
	Wait     bool     `json:"wait,omitempty"`
	Edges    []string `json:"edges,omitempty"` // "on → to (guard)"
	// Laws are the machine's laws (they are checked over every state) and
	// the laws the state's text names as "law <name>".
	Laws []string `json:"laws,omitempty"`
}

// Site is a declared site and the line resolve bound it to.
type Site struct {
	State   string   `json:"state"`
	Status  string   `json:"status,omitempty"` // the state's status
	Step    string   `json:"step"`
	StepDoc string   `json:"step_doc,omitempty"`
	Tag     string   `json:"tag,omitempty"`
	Match   string   `json:"match,omitempty"`
	Kind    string   `json:"kind"`
	Effect  string   `json:"effect,omitempty"`
	File    string   `json:"file"`
	Line    int      `json:"line,omitempty"` // 0 when resolve could not bind it
	Laws    []string `json:"laws,omitempty"` // named as "law <name>" in its effect or step
	// Caught says where the machine's catch rules take a failure of this
	// site: the target states, or "carries on (<note>)" for a catch that
	// leaves the pass where it is. Empty when no catch covers its kind.
	Caught string `json:"caught,omitempty"`
	// DeclFile and DeclLine point at the site's declaration (its Tag or
	// Match literal), when it can be found.
	DeclFile string `json:"decl_file,omitempty"`
	DeclLine int    `json:"decl_line,omitempty"`
}

// Law is how one machine law fared.
type Law struct {
	Name       string   `json:"name"`
	Doc        string   `json:"doc,omitempty"`
	Status     string   `json:"status"`
	Pending    string   `json:"pending,omitempty"`
	Violations []string `json:"violations,omitempty"`
}

// Problem is a disagreement between a declaration and its source, on a line
// when the message names one.
type Problem struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// Decode reads a document and refuses a version it does not know.
func Decode(reader io.Reader) (Document, error) {
	var document Document
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return Document{}, fmt.Errorf("machine index: %w", err)
	}
	if document.Version != Version {
		return Document{}, fmt.Errorf("machine index version %d, want %d: rebuild the runner with this lawgate", document.Version, Version)
	}
	return document, nil
}

var lawReference = regexp.MustCompile(`\blaws? ` + "`?" + `([a-z0-9]+(?:-[a-z0-9]+)+)`)

// LawRefs returns the law names texts refer to as "law <name>", in order of
// first mention.
func LawRefs(texts ...string) []string {
	var names []string
	seen := map[string]bool{}
	for _, text := range texts {
		for _, match := range lawReference.FindAllStringSubmatch(text, -1) {
			names = appendNew(names, seen, match[1])
		}
	}
	return names
}

func appendNew(names []string, seen map[string]bool, name string) []string {
	if seen[name] {
		return names
	}
	seen[name] = true
	return append(names, name)
}
