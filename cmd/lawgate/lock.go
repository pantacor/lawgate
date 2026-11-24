package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawslock"
)

func lockCommand(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("lock", flag.ContinueOnError)
	rootFlag := flags.String("root", ".", "repository root")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("lock accepts flags only")
	}
	root, err := gosource.CanonicalRoot(*rootFlag)
	if err != nil {
		return err
	}
	config, err := inventory.Load(root)
	if err != nil {
		return err
	}
	read := checkedSource(root)
	laws, err := lawslock.Collect(config, read)
	if err != nil {
		return err
	}
	lock, err := lawslock.Build(root, laws, read)
	if err != nil {
		return err
	}
	if err := writeProofLock(root, config.LockPath(), lock); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, config.LockPath())
	return err
}

func checkedSource(root string) lawslock.Reader {
	return func(name string) ([]byte, error) {
		if err := refuseLinkedPath(root, name); err != nil {
			return nil, err
		}
		if err := requireFile(root, name); err != nil {
			return nil, err
		}
		return os.ReadFile(filepath.Join(root, name))
	}
}

func writeProofLock(root, name string, lock lawslock.Lock) error {
	previous, err := lockDestination(root, name)
	if err != nil {
		return err
	}
	lock.PreserveRemovalApprovals(previous)
	data, err := lock.Encode()
	if err != nil {
		return err
	}
	path := filepath.Join(root, name)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".lawgate-lock-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	if err := os.Chmod(file.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func lockDestination(root, name string) (lawslock.Lock, error) {
	data, err := checkedSource(root)(name)
	if errors.Is(err, fs.ErrNotExist) {
		return lawslock.Lock{}, nil
	}
	if err != nil {
		return lawslock.Lock{}, err
	}
	lock, err := lawslock.Decode(data)
	if err != nil {
		return lock, fmt.Errorf("%s: refusing to replace a file that is not a proof lock: %w", name, err)
	}
	return lock, nil
}

func checkAnchoredProofs(root, base string, config inventory.Config, result *moduleResult) {
	read := checkedSource(root)
	laws, collectErr := lawslock.Collect(config, read)
	name := config.LockPath()
	data, err := read(name)
	missing := errors.Is(err, fs.ErrNotExist)
	// Existing local inventories may opt in by naming a subject or a gap.
	legacy := config.Lock == "" && base == "" && !slices.ContainsFunc(laws, requiresLock)
	if missing && legacy {
		return
	}
	checkProofLock(root, name, data, laws, errors.Join(err, collectErr), result)
	if base != "" && collectErr == nil {
		checkBaseProofs(root, base, laws, result)
	}
}

func requiresLock(law lawslock.Law) bool {
	declared := len(law.Subjects) != 0 || law.SubjectGap != ""
	return len(law.Tests) != 0 && declared
}

func checkProofLock(root, name string, data []byte, laws []lawslock.Law, err error, result *moduleResult) {
	const phase = "proof lock"
	if err != nil {
		result.Verdicts = append(result.Verdicts, verdict{phase, failed, fmt.Sprintf("%s: %v (run lawgate lock; the update needs the owner's review)", name, err)})
		return
	}
	locked, err := lawslock.Decode(data)
	if err != nil {
		result.Verdicts = append(result.Verdicts, verdict{phase, failed, fmt.Sprintf("%s: %v", name, err)})
		return
	}
	current, err := lawslock.Build(root, laws, checkedSource(root))
	if err != nil {
		result.Verdicts = append(result.Verdicts, verdict{phase, failed, err.Error()})
		return
	}
	problems := lawslock.Compare(locked, current, name)
	for _, problem := range problems {
		result.Verdicts = append(result.Verdicts, verdict{phase, failed, problem.Error()})
	}
	if len(problems) == 0 {
		result.Verdicts = append(result.Verdicts, verdict{phase, passed, name})
	}
}
