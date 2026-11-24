package lawslock

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/pantacor/lawgate/inventory"
)

type ApprovedLaw struct {
	Law
	Entry Entry
}

// Approved reads the base inventory, law documents and lock from the same commit.
func Approved(read Reader) ([]ApprovedLaw, error) {
	configData, err := read(inventory.Filename)
	if err != nil {
		return nil, err
	}
	config, err := inventory.Decode(bytes.NewReader(configData))
	if err != nil {
		return nil, err
	}
	laws, err := Collect(config, read)
	if err != nil {
		return nil, err
	}
	data, err := read(config.LockPath())
	if err != nil {
		return nil, fmt.Errorf("base proof lock %s: %w", config.LockPath(), err)
	}
	lock, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("base proof lock %s: %w", config.LockPath(), err)
	}
	return approvedEntries(laws, lock)
}

func approvedEntries(laws []Law, lock Lock) ([]ApprovedLaw, error) {
	index := lock.index()
	var approved []ApprovedLaw
	for _, law := range laws {
		entry, found := index[law.ID]
		if !found || !matchesReferences(law.Tests, entry.Tests) {
			return nil, fmt.Errorf("base proof lock does not match the agreed enforcers of %s (%s, %s)", law.Law, law.Module, law.Document)
		}
		approved = append(approved, ApprovedLaw{Law: law, Entry: entry})
		delete(index, law.ID)
	}
	for _, entry := range lock.Laws {
		if _, extra := index[entry.ID]; extra {
			return nil, fmt.Errorf("base proof lock lists law %s (%s, %s), which is no longer agreed", entry.Law, entry.Module, entry.Document)
		}
	}
	return approved, nil
}

func matchesReferences(references []string, tests []Test) bool {
	for _, name := range references {
		if !slices.ContainsFunc(tests, func(test Test) bool { return matchesReference(name, test) }) {
			return false
		}
	}
	for _, test := range tests {
		if !slices.ContainsFunc(references, func(name string) bool { return matchesReference(name, test) }) {
			return false
		}
	}
	return true
}

func matchesReference(name string, test Test) bool {
	qualified := name == test.QualifiedName() || name == test.Package+"."+test.Name
	return name == test.Name || qualified
}
