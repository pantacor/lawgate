// Package lawslock fingerprints and replays the Go proofs of agreed laws.
package lawslock

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
	"github.com/pantacor/lawgate/mutate"
)

// ID identifies a law even when a shared document is used by several modules.
type ID struct {
	Module   string `json:"module"`
	Document string `json:"document"`
	Law      string `json:"law"`
}

type Test struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	File    string `json:"file"`
}

func (test Test) QualifiedName() string { return test.Package + "/" + test.Name }

type Subject struct {
	Reference   string `json:"reference"`
	Fingerprint string `json:"fingerprint"`
}

type Entry struct {
	ID
	AllowRemoval bool      `json:"allow_removal,omitempty"`
	Proof        string    `json:"proof,omitempty"`
	Subjects     []Subject `json:"subjects,omitempty"`
	Tests        []Test    `json:"tests,omitempty"`
	TestFiles    []string  `json:"test_files,omitempty"`
}

type Lock struct {
	Version int     `json:"version"`
	Laws    []Entry `json:"laws"`
}

type Law struct {
	ID
	ModuleConfig inventory.Module
	Tests        []string
	SubjectGap   string
	Subjects     []mutate.Subject
}

type Reader func(string) ([]byte, error)

// Collect reads the agreed laws from worktree or Git blobs without running code.
func Collect(config inventory.Config, read Reader) ([]Law, error) {
	var laws []Law
	for _, module := range config.Modules {
		moduleLaws, err := collectModule(module, read)
		if err != nil {
			return nil, err
		}
		laws = append(laws, moduleLaws...)
	}
	return laws, nil
}

func collectModule(module inventory.Module, read Reader) ([]Law, error) {
	var laws []Law
	for _, file := range append([]string{module.Laws}, module.SharedLaws...) {
		data, err := read(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		laws = append(laws, agreedLaws(module, file, string(data))...)
	}
	return laws, nil
}

func agreedLaws(module inventory.Module, file, document string) []Law {
	var laws []Law
	for _, plan := range lawsdoc.Plan(document) {
		if !plan.Agreed() {
			continue
		}
		id := ID{Module: module.Path, Document: file, Law: plan.Name}
		law := Law{ID: id, ModuleConfig: module}
		if module.Language == "go" {
			law.Tests = testNames(plan)
			law.Subjects = plan.Subjects
			law.SubjectGap = plan.SubjectGap
		}
		laws = append(laws, law)
	}
	return laws
}

func testNames(plan lawsdoc.LawPlan) []string {
	var names []string
	for _, enforcer := range plan.Enforcers {
		if enforcer.Kind == lawsdoc.Test {
			names = append(names, enforcer.Name)
		}
	}
	return names
}

func Decode(data []byte) (Lock, error) {
	var lock Lock
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lock); err != nil {
		return Lock{}, err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return Lock{}, errors.New("expected one proof lock object")
	}
	if lock.Version != 1 || lock.Laws == nil {
		return Lock{}, errors.New("unsupported or missing proof lock version or laws")
	}
	seen := make(map[ID]bool)
	for _, entry := range lock.Laws {
		if seen[entry.ID] {
			return Lock{}, fmt.Errorf("duplicate law %s in proof lock (%s, %s)", entry.Law, entry.Module, entry.Document)
		}
		seen[entry.ID] = true
	}
	return lock, nil
}

func (lock Lock) Encode() ([]byte, error) {
	data, err := json.MarshalIndent(lock, "", "  ")
	return append(data, '\n'), err
}

func (lock Lock) index() map[ID]Entry {
	entries := make(map[ID]Entry)
	for _, entry := range lock.Laws {
		entries[entry.ID] = entry
	}
	return entries
}

// PreserveRemovalApprovals keeps owner decisions when regenerating fingerprints.
func (lock *Lock) PreserveRemovalApprovals(previous Lock) {
	entries := previous.index()
	for index, entry := range lock.Laws {
		lock.Laws[index].AllowRemoval = entries[entry.ID].AllowRemoval
	}
}

// Compare reports all differences, including removed and unanchored laws.
func Compare(locked, current Lock, path string) []error {
	before, after := locked.index(), current.index()
	var problems []error
	for _, law := range current.Laws {
		previous, found := before[law.ID]
		if !found || !sameProof(previous, law) {
			problems = append(problems, changed(law.ID, "proof", path))
		}
		if found && !slices.Equal(previous.Subjects, law.Subjects) {
			problems = append(problems, changed(law.ID, "subject", path))
		}
	}
	for _, law := range locked.Laws {
		if _, found := after[law.ID]; !found {
			problems = append(problems, changed(law.ID, "proof", path))
		}
	}
	return problems
}

func sameProof(left, right Entry) bool {
	sameInputs := slices.Equal(left.Tests, right.Tests) && slices.Equal(left.TestFiles, right.TestFiles)
	return left.Proof == right.Proof && sameInputs
}

func changed(id ID, field, path string) error {
	return fmt.Errorf("%s of %s changed (%s, %s): update %s, which needs the owner's review", field, id.Law, id.Module, id.Document, path)
}

func localPath(name string) bool {
	native := filepath.FromSlash(name)
	clean := filepath.ToSlash(filepath.Clean(native)) == name
	return filepath.IsLocal(native) && clean
}
