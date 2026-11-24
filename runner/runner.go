// Package runner provides the shared cmd/machine entry point for adopting Go
// modules. Every entry is resolved against its Go source before its laws are
// checked, so a product cannot skip layer-2 drift detection or diverge from the
// lawgate exit-code and JUnit contract.
package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/report"
	"github.com/pantacor/lawgate/resolve"
)

// Entry pairs one declared machine with the source file, tag vocabulary, tag
// prefix, and extra laws checked by the shared runner.
type Entry struct {
	Machine    *machine.Machine
	Source     string // overrides Machine.Source when non-empty
	Vocabulary resolve.Vocabulary
	Prefix     string
	Laws       []machine.Law // checked alongside Machine.Laws
}

// Main runs the standard machine-runner CLI over entries and exits. It
// does one thing per run, picked by its flags; -machine NAME narrows any of
// them to one machine.
//
// With -laws (and -junit FILE) it checks every law:
//   - 0 when every machine resolves and every law holds (or is pending),
//   - 1 when resolution fails or any law breaks (after writing -junit if set),
//   - 2 on flag, usage, or JUnit I/O errors.
//
// With -index FILE (FILE "-" is standard output) it writes the machine index
// lawgate review reads (package review/index) and exits 0 once it is written,
// whatever the laws' verdicts (they are in the index), or 2 on flag, usage or
// I/O errors.
//
// -list lists the machines with their state and law counts. -json writes one
// machine as a MachineView, -brief STATE the brief an agent needs to change
// one state (package render), and with none of these flags it prints each
// machine's outline (-color colours it). These views exit 0 when shown, 1
// when shown but the declaration has drifted from its source (the drift is
// on standard error), and 2 on flag, usage or I/O errors.
func Main(entries []Entry) {
	os.Exit(Run(os.Args[1:], entries, os.Stdout, os.Stderr))
}

// Run parses machine-runner flags from args, runs what they ask for over
// the entries relative to the current working directory, and returns the
// process exit status (0, 1, or 2).
func Run(args []string, entries []Entry, stdout, stderr io.Writer) int {
	return runIn(".", args, entries, stdout, stderr)
}

func runIn(root string, args []string, entries []Entry, stdout, stderr io.Writer) int {
	options, ok := parseArgs(args, stderr)
	if !ok {
		return 2
	}
	selected, err := selectEntries(entries, options.machine)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	out := output{root: root, stdout: stdout, stderr: stderr}
	switch {
	case options.index != "":
		return indexStatus(writeIndex(root, options.index, selected, stdout), stderr)
	case options.laws:
		return out.laws(options.junit, selected)
	case options.list:
		return out.list(selected)
	case options.json:
		return out.json(selected)
	case options.brief != "":
		return out.brief(options.brief, selected)
	}
	return out.outline(options.style(), selected)
}

// output is where a run writes, and the root its source paths are
// relative to.
type output struct {
	root           string
	stdout, stderr io.Writer
}

func (out output) laws(junitPath string, entries []Entry) int {
	runs := CheckIn(out.root, entries)
	if junitPath == "" {
		fmt.Fprint(out.stdout, report.Markdown(runs))
		return exitStatus(runs)
	}
	if err := report.WriteJUnit(junitPath, runs); err != nil {
		fmt.Fprintf(out.stderr, "write junit %s: %v\n", junitPath, err)
		return 2
	}
	return finishWithJUnit(runs, out.stderr)
}

func indexStatus(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "write index: %v\n", err)
		return 2
	}
	return 0
}

func finishWithJUnit(runs []report.LawRun, stderr io.Writer) int {
	status := exitStatus(runs)
	if status != 0 {
		fmt.Fprint(stderr, report.Markdown(runs))
	}
	return status
}

func exitStatus(runs []report.LawRun) int {
	if report.Sum(runs).Outcome().Refuses() {
		return 1
	}
	return 0
}

// Check resolves and checks every entry relative to the current directory.
func Check(entries []Entry) []report.LawRun {
	return CheckIn(".", entries)
}

// CheckIn resolves and checks every entry with relative source paths resolved
// against root. An empty entry list returns a failed run so the runner is never
// green by default.
func CheckIn(root string, entries []Entry) []report.LawRun {
	if len(entries) == 0 {
		return []report.LawRun{report.FailedRun("", "no machines declared")}
	}
	seen := make(map[string]bool, len(entries))
	runs := make([]report.LawRun, 0, len(entries))
	for _, entry := range entries {
		runs = append(runs, checkEntry(root, entry, seen))
	}
	return runs
}

func checkEntry(root string, entry Entry, seen map[string]bool) report.LawRun {
	if entry.Machine == nil {
		return report.FailedRun("", "machine entry is nil")
	}
	name := entry.Machine.Name
	if name == "" {
		return report.FailedRun("", "machine has no name")
	}
	if seen[name] {
		return report.LawRun{Machine: name, Failure: fmt.Sprintf("duplicate machine %q", name)}
	}
	seen[name] = true
	return resolveAndCheck(root, entry)
}

func resolveAndCheck(root string, entry Entry) report.LawRun {
	name := entry.Machine.Name
	source := entrySource(entry)
	if source == "" {
		return report.LawRun{Machine: name, Failure: "machine has no source path"}
	}
	src, err := os.ReadFile(sourceFilePath(root, source))
	if err != nil {
		return report.LawRun{Machine: name, Source: source, Failure: fmt.Sprintf("read source %s: %v", source, err)}
	}
	if errs := resolve.Resolve(entry.Machine, src, entry.Vocabulary, entry.Prefix); len(errs) > 0 {
		return report.LawRun{Machine: name, Source: source, Failure: joinErrors(errs)}
	}
	laws := combinedLaws(entry.Machine.Laws, entry.Laws)
	verdicts, err := machine.CheckLaws(entry.Machine, laws)
	if err != nil {
		return report.LawRun{Machine: name, Source: source, Failure: fmt.Sprintf("check laws: %v", err)}
	}
	return report.LawRun{Machine: name, Source: source, Verdicts: verdicts}
}

func entrySource(entry Entry) string {
	if entry.Source != "" {
		return entry.Source
	}
	return entry.Machine.Source
}

func sourceFilePath(root, source string) string {
	relativeToCwd := root == "" || root == "."
	if filepath.IsAbs(source) || relativeToCwd {
		return source
	}
	return filepath.Join(root, filepath.FromSlash(source))
}

func combinedLaws(machineLaws, extraLaws []machine.Law) []machine.Law {
	out := make([]machine.Law, 0, len(machineLaws)+len(extraLaws))
	out = append(out, slices.Clone(machineLaws)...)
	out = append(out, extraLaws...)
	return out
}

func joinErrors(errs []error) string {
	lines := make([]string, 0, len(errs))
	for _, err := range errs {
		lines = append(lines, err.Error())
	}
	return strings.Join(lines, "\n")
}
