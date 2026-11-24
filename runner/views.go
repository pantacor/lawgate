package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/render"
	"github.com/pantacor/lawgate/resolve"
	"github.com/pantacor/lawgate/review/index"
)

// MachineView is what -json writes for one machine, for a viewer that draws
// the machine beside its code: the declaration as package machine encodes
// it, each site carrying the line resolve bound it to, then its Summary,
// the Drift between the declaration and the source, the Laws' verdicts and
// the source's Lines. Failure says why the laws could not be checked.
type MachineView struct {
	*machine.Machine
	Summary string          `json:"summary"`
	Drift   []index.Problem `json:"drift"`
	Laws    []index.Law     `json:"laws"`
	Failure string          `json:"failure,omitempty"`
	Lines   []string        `json:"lines"`
}

// selectEntries keeps every entry, or with a name only the machine of that
// name.
func selectEntries(entries []Entry, name string) ([]Entry, error) {
	if name == "" {
		return entries, nil
	}
	for _, entry := range entries {
		if entry.Machine != nil && entry.Machine.Name == name {
			return []Entry{entry}, nil
		}
	}
	return nil, fmt.Errorf("no machine %q (see -list)", name)
}

// oneEntry is the single entry a one-machine view shows.
func oneEntry(entries []Entry, view string) (Entry, error) {
	if len(entries) != 1 {
		return Entry{}, fmt.Errorf("%s shows one machine: pass -machine NAME (see -list)", view)
	}
	return entries[0], nil
}

// bound is an entry's machine resolved against its source: its sites carry
// their lines, and drift lists every disagreement resolve found.
type bound struct {
	machine *machine.Machine
	source  string
	src     []byte
	drift   []error
}

// resolveEntry reads an entry's source and resolves its machine against it.
// The error says why the source could not be read; drift is not an error.
func resolveEntry(root string, entry Entry) (bound, error) {
	if entry.Machine == nil {
		return bound{}, errors.New("machine entry is nil")
	}
	source := entrySource(entry)
	if source == "" {
		return bound{}, errors.New("machine has no source path")
	}
	src, err := os.ReadFile(sourceFilePath(root, source))
	if err != nil {
		return bound{}, fmt.Errorf("read source %s: %v", source, err)
	}
	drift := resolve.Resolve(entry.Machine, src, entry.Vocabulary, entry.Prefix)
	return bound{machine: entry.Machine, source: source, src: src, drift: drift}, nil
}

func (b bound) problems() []index.Problem {
	problems := make([]index.Problem, 0, len(b.drift))
	for _, err := range b.drift {
		problems = append(problems, sourceProblem(b.source, err))
	}
	return problems
}

func (out output) list(entries []Entry) int {
	if !out.declared(entries) {
		return 2
	}
	for _, entry := range entries {
		if entry.Machine == nil {
			continue
		}
		m := entry.Machine
		laws := len(combinedLaws(m.Laws, entry.Laws))
		counts := counted(len(m.States), "state") + ", " + counted(laws, "law")
		fmt.Fprintf(out.stdout, "%-34s %-40s %s\n", m.Name, entrySource(entry), counts)
	}
	return 0
}

// declared says on standard error when there is no machine to show: a
// runner with no machines is never quietly fine.
func (out output) declared(entries []Entry) bool {
	if len(entries) == 0 {
		fmt.Fprintln(out.stderr, "no machines declared")
	}
	return len(entries) > 0
}

func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func (out output) json(entries []Entry) int {
	entry, b, ok := out.resolveOne(entries, "-json")
	if !ok {
		return 2
	}
	return out.writeJSON(entry, b)
}

func (out output) writeJSON(entry Entry, b bound) int {
	view := MachineView{Machine: b.machine, Summary: render.Summary(b.machine), Drift: b.problems()}
	view.Lines = strings.Split(string(b.src), "\n")
	var err error
	view.Laws, err = indexLaws(b.machine, combinedLaws(b.machine.Laws, entry.Laws))
	if err != nil {
		view.Failure = fmt.Sprintf("check laws: %v", err)
	}
	encoder := json.NewEncoder(out.stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(view); err != nil {
		fmt.Fprintf(out.stderr, "write json: %v\n", err)
		return 2
	}
	return out.driftStatus(b)
}

func (out output) brief(state string, entries []Entry) int {
	_, b, ok := out.resolveOne(entries, "-brief")
	if !ok {
		return 2
	}
	return out.writeBrief(state, b)
}

func (out output) writeBrief(state string, b bound) int {
	if err := render.Brief(out.stdout, b.machine, state); err != nil {
		fmt.Fprintln(out.stderr, err)
		return 2
	}
	return out.driftStatus(b)
}

// resolveOne resolves the single entry a one-machine view shows, and says
// on standard error why when it cannot.
func (out output) resolveOne(entries []Entry, view string) (Entry, bound, bool) {
	entry, err := oneEntry(entries, view)
	if err != nil {
		fmt.Fprintln(out.stderr, err)
		return Entry{}, bound{}, false
	}
	b, err := resolveEntry(out.root, entry)
	if err != nil {
		fmt.Fprintln(out.stderr, err)
		return Entry{}, bound{}, false
	}
	return entry, b, true
}

// outline prints each machine's outline and summary. A machine whose source
// cannot be read is reported and skipped; the others are still shown.
func (out output) outline(style render.Style, entries []Entry) int {
	if !out.declared(entries) {
		return 2
	}
	status := 0
	for _, entry := range entries {
		b, err := resolveEntry(out.root, entry)
		if err != nil {
			fmt.Fprintln(out.stderr, err)
			status = 2
			continue
		}
		status = max(status, out.writeOutline(style, b))
	}
	return status
}

func (out output) writeOutline(style render.Style, b bound) int {
	render.Text(out.stdout, b.machine, style)
	fmt.Fprintln(out.stdout, render.Summary(b.machine))
	return out.driftStatus(b)
}

// driftStatus writes the machine's drift to standard error and returns 1
// when there is any: the view was shown, but the declaration and the source
// disagree.
func (out output) driftStatus(b bound) int {
	for _, err := range b.drift {
		fmt.Fprintf(out.stderr, "drift: %s: %v\n", b.machine.Name, err)
	}
	if len(b.drift) > 0 {
		return 1
	}
	return 0
}
