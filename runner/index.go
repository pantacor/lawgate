package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/render"
	"github.com/pantacor/lawgate/review/gocode"
	"github.com/pantacor/lawgate/review/index"
)

// Index resolves every entry against its source relative to the current
// directory and returns the machine index `lawgate review` reads.
func Index(entries []Entry) index.Document {
	return IndexIn(".", entries)
}

// IndexIn is Index with relative paths resolved against root. Unlike
// CheckIn it keeps going past drift: the laws are checked on the
// declaration even when a site does not resolve, so a reviewer sees both.
func IndexIn(root string, entries []Entry) index.Document {
	document := index.Document{Version: index.Version, Machines: []index.Machine{}}
	literals := gocode.ScanLiterals(root)
	for _, entry := range entries {
		document.Machines = append(document.Machines, indexEntry(root, entry, literals))
	}
	return document
}

func writeIndex(root, target string, entries []Entry, stdout io.Writer) error {
	data, err := json.MarshalIndent(IndexIn(root, entries), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if target == "-" {
		_, err = stdout.Write(data)
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func indexEntry(root string, entry Entry, literals gocode.Literals) index.Machine {
	if entry.Machine == nil {
		return index.Machine{Failure: "machine entry is nil"}
	}
	source := entrySource(entry)
	indexed := index.Machine{Name: entry.Machine.Name, Source: source, Doc: entry.Machine.Doc}
	b, err := resolveEntry(root, entry)
	if err != nil {
		indexed.Failure = err.Error()
		return indexed
	}
	if len(b.drift) > 0 {
		indexed.Problems = b.problems()
	}
	laws := combinedLaws(entry.Machine.Laws, entry.Laws)
	indexed.Laws, err = indexLaws(entry.Machine, laws)
	if err != nil {
		indexed.Failure = fmt.Sprintf("check laws: %v", err)
	}
	lawNames := machineLawNames(laws)
	indexed.States = indexStates(entry.Machine, lawNames)
	indexed.Sites = indexSites(entry.Machine, source, literals)
	return indexed
}

var problemLine = regexp.MustCompile(`(?:^|\s)line (\d+)`)

// sourceProblem places a resolve error on the source line it names.
func sourceProblem(source string, problem error) index.Problem {
	message := problem.Error()
	placed := index.Problem{File: source, Message: message}
	if match := problemLine.FindStringSubmatch(message); match != nil {
		placed.Line, _ = strconv.Atoi(match[1])
	}
	return placed
}

func indexLaws(m *machine.Machine, laws []machine.Law) ([]index.Law, error) {
	verdicts, err := machine.CheckLaws(m, laws)
	if err != nil {
		return []index.Law{}, err
	}
	indexed := make([]index.Law, 0, len(verdicts))
	for _, verdict := range verdicts {
		indexed = append(indexed, indexLaw(verdict))
	}
	return indexed, nil
}

func indexLaw(verdict machine.Verdict) index.Law {
	law := index.Law{Name: verdict.Law.Name, Doc: verdict.Law.Doc, Status: index.StatusHolds, Violations: verdict.Violations}
	switch {
	case verdict.Broken():
		law.Status = index.StatusBroken
	case verdict.IsPending():
		law.Status = index.StatusPending
		law.Pending = verdict.Law.Pending
	}
	return law
}

func machineLawNames(laws []machine.Law) []string {
	names := make([]string, 0, len(laws))
	for _, law := range laws {
		names = append(names, law.Name)
	}
	return names
}

func indexStates(m *machine.Machine, machineLaws []string) []index.State {
	states := make([]index.State, 0, len(m.States))
	for _, state := range m.States {
		states = append(states, indexState(state, machineLaws))
	}
	return states
}

func indexState(state machine.State, machineLaws []string) index.State {
	indexed := index.State{
		ID: state.ID, Func: state.Func, Status: state.Status, Doc: state.Doc,
		When: state.When, WokenBy: state.WokenBy, Terminal: state.Terminal, Wait: state.Wait,
	}
	texts := []string{state.Doc, state.When, state.WokenBy}
	for _, edge := range state.Edges {
		indexed.Edges = append(indexed.Edges, edgeText(edge))
		texts = append(texts, edge.Guard)
	}
	for _, step := range state.Steps {
		texts = append(texts, step.Doc)
	}
	named := index.LawRefs(texts...)
	indexed.Laws = append(append([]string{}, machineLaws...), withoutAny(named, machineLaws)...)
	return indexed
}

func edgeText(edge machine.Edge) string {
	text := edge.On + " → " + edge.To
	if edge.Guard != "" {
		text += " (" + edge.Guard + ")"
	}
	return text
}

func withoutAny(names, drop []string) []string {
	var kept []string
	for _, name := range names {
		if !containsName(drop, name) {
			kept = append(kept, name)
		}
	}
	return kept
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func indexSites(m *machine.Machine, source string, literals gocode.Literals) []index.Site {
	sites := []index.Site{}
	file := path.Clean(filepath.ToSlash(source))
	claimed := map[gocode.Literal]bool{}
	m.Sites(func(state *machine.State, step *machine.Step, site *machine.Site) {
		indexed := index.Site{
			State: state.ID, Status: state.Status, Step: step.ID, StepDoc: step.Doc,
			Tag: site.Tag, Match: site.Match, Kind: string(site.Kind), Effect: site.Effect,
			File: file, Line: site.Line, Laws: index.LawRefs(site.Effect, step.Doc),
			Caught: caughtText(m, site.Kind),
		}
		if declared, found := literals.FindFree(m.Name, site.Tag, site.Match, claimed); found {
			indexed.DeclFile, indexed.DeclLine = declared.File, declared.Line
			claimed[declared] = true
		}
		sites = append(sites, indexed)
	})
	return sites
}

// caughtText says where the catch rules for kind take a failure: each
// target state, or "carries on (<note>)" for a catch that carries on.
func caughtText(m *machine.Machine, kind machine.SiteKind) string {
	var targets []string
	for _, catch := range m.Catches {
		if catch.On == kind {
			targets = append(targets, catchTarget(catch))
		}
	}
	return strings.Join(targets, ", ")
}

func catchTarget(catch machine.Catch) string {
	if !catch.CarriesOn() {
		return catch.To
	}
	if catch.Note == "" {
		return render.CarriesOn
	}
	return render.CarriesOn + " (" + catch.Note + ")"
}
