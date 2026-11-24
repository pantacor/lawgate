package review

import (
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/review/gocode"
	"github.com/pantacor/lawgate/review/index"
)

// checkReportFlags are the flags inventory.Module.MachineArgs adds for the
// check's report. The index run keeps the configured command and replaces
// them, so `lawgate check` and `lawgate review` run the same machine command.
var checkReportFlags = []string{"-laws", "-junit", ""}

// IndexArgs is the module's machine command with -index target instead of
// the check's report flags.
func IndexArgs(module inventory.Module, target string) ([]string, error) {
	args := module.MachineArgs("")
	command := len(args) - len(checkReportFlags)
	if command < 1 || !slices.Equal(args[command:], checkReportFlags) {
		return nil, fmt.Errorf("machine command %q does not end with the check's report flags", args)
	}
	return append(slices.Clip(args[:command]), "-index", target), nil
}

// readMachines runs the module's machine command with -index and adds what
// it describes.
func (b *builder) readMachines(module inventory.Module) {
	document, err := b.runIndex(module)
	if err != nil {
		b.problem(Problem{Module: module.Path, File: module.Laws, Message: fmt.Sprintf("machine index: %v", err)})
		return
	}
	for _, machine := range document.Machines {
		b.addMachine(module.Path, machine)
	}
}

func (b *builder) runIndex(module inventory.Module) (index.Document, error) {
	file, err := os.CreateTemp("", "lawgate-index-*.json")
	if err != nil {
		return index.Document{}, err
	}
	target := file.Name()
	file.Close()
	defer os.Remove(target)
	args, err := IndexArgs(module, target)
	if err != nil {
		return index.Document{}, err
	}
	stdout, stderr, runErr := b.execute(b.idx.Root, module.Path, args, module.Timeout())
	if runErr != nil {
		return index.Document{}, commandError(runErr, stdout, stderr)
	}
	written, err := os.Open(target)
	if err != nil {
		return index.Document{}, err
	}
	defer written.Close()
	return index.Decode(written)
}

// commandError keeps the tail of what the command said, where go run and a
// runner put their reasons.
func commandError(err error, stdout []byte, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(string(stdout))
	}
	const maxDetail = 2048
	if len(detail) > maxDetail {
		detail = detail[len(detail)-maxDetail:]
	}
	return fmt.Errorf("%w: %s", err, detail)
}

// addMachine adds one indexed machine, with module-relative paths made
// relative to the repository root.
func (b *builder) addMachine(module string, machine index.Machine) {
	source := repoPath(module, machine.Source)
	if machine.Failure != "" {
		b.problem(Problem{Module: module, Machine: machine.Name, File: source, Message: machine.Failure})
	}
	for _, problem := range machine.Problems {
		placed := Problem{Module: module, Machine: machine.Name, File: repoPath(module, problem.File), Line: problem.Line, Message: problem.Message}
		b.problem(placed)
	}
	sites := b.addSites(module, machine)
	b.addStates(module, machine, source, sites)
	for _, law := range machine.Laws {
		b.addVerdict(module, machine.Name, law)
	}
}

func repoPath(module, file string) string {
	if file == "" {
		return ""
	}
	return path.Join(module, file)
}

func (b *builder) addSites(module string, machine index.Machine) []Site {
	stateDocs := map[string]string{}
	for _, state := range machine.States {
		stateDocs[state.ID] = state.Doc
	}
	var sites []Site
	for _, indexed := range machine.Sites {
		site := Site{Module: module, Machine: machine.Name, Site: indexed, StateDoc: stateDocs[indexed.State]}
		site.File = repoPath(module, indexed.File)
		site.DeclFile = repoPath(module, indexed.DeclFile)
		site.Laws = b.lawKeys(module, indexed.Laws)
		sites = append(sites, site)
	}
	for _, site := range sites {
		if site.Line > 0 {
			b.idx.Sites = append(b.idx.Sites, site)
		}
	}
	return sites
}

// lawKeys turns law names into the keys of the laws module documents; a
// name no LAWS.md writes stays as it is.
func (b *builder) lawKeys(module string, names []string) []string {
	keys := make([]string, 0, len(names))
	for _, name := range names {
		key := name
		if law := b.idx.moduleLaw(module, name); law != nil {
			key = law.Key
		}
		keys = append(keys, key)
	}
	return keys
}

func (b *builder) addStates(module string, machine index.Machine, source string, sites []Site) {
	functions := gocode.Functions(b.idx.Root, path.Dir(source))
	for _, indexed := range machine.States {
		state := State{Module: module, Machine: machine.Name, State: indexed}
		state.Laws = b.lawKeys(module, indexed.Laws)
		if lines, found := functions[indexed.Func]; found {
			state.File, state.StartLine, state.EndLine = lines.File, lines.Start, lines.End
			state.Spans = append(state.Spans, lines)
		}
		state.Spans = addSiteSpans(state.Spans, functions, sites, indexed.ID)
		b.idx.States = append(b.idx.States, state)
	}
}

// addSiteSpans adds the function around each of the state's sites.
func addSiteSpans(spans []gocode.Range, functions map[string]gocode.Range, sites []Site, state string) []gocode.Range {
	for _, site := range sites {
		if site.State != state || site.Line == 0 {
			continue
		}
		lines, found := gocode.Enclosing(functions, site.File, site.Line)
		if found && !slices.Contains(spans, lines) {
			spans = append(spans, lines)
		}
	}
	return spans
}

// addVerdict attaches a machine's verdict to the law its module documents,
// or to a law known only from code.
func (b *builder) addVerdict(module, machine string, verdict index.Law) {
	law := b.idx.moduleLaw(module, verdict.Name)
	if law == nil {
		law = b.codeLaw(module, verdict.Name)
	}
	if law.CodeDoc == "" {
		law.CodeDoc = verdict.Doc
	}
	recorded := Verdict{Machine: machine, Status: verdict.Status, Pending: verdict.Pending, Violations: verdict.Violations}
	law.Verdicts = append(law.Verdicts, recorded)
}

// codeLaw is the law module checks in code but no LAWS.md writes, made once
// however many of its machines check it.
func (b *builder) codeLaw(module, name string) *Law {
	for _, law := range b.idx.Laws {
		codeOnly := law.File == "" && law.Module == module
		if codeOnly && law.Name == name {
			return law
		}
	}
	law := &Law{Name: name, Module: module}
	b.idx.addLaw(law)
	return law
}
