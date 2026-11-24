package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawslock"
	"github.com/pantacor/lawgate/rulechanges"
)

const replayPhase = "base proof replay"

type baseReplay struct {
	root      string
	result    *moduleResult
	candidate map[lawslock.ID]bool
	read      lawslock.Reader
}

func checkBaseProofs(root, base string, laws []lawslock.Law, result *moduleResult) {
	replay := baseReplay{root: root, result: result, candidate: make(map[lawslock.ID]bool)}
	for _, law := range laws {
		replay.candidate[law.ID] = true
	}
	err := rulechanges.ReadSnapshot(root, base, func(read func(string) ([]byte, error)) error {
		replay.read = read
		return replay.run()
	})
	if err != nil {
		result.recordError(replayPhase, err)
	}
}

func (replay baseReplay) run() error {
	approved, err := lawslock.Approved(replay.read)
	if err != nil {
		return err
	}
	for _, law := range approved {
		replay.check(law)
	}
	return nil
}

func (replay baseReplay) check(law lawslock.ApprovedLaw) {
	if !replay.candidate[law.Entry.ID] {
		replay.checkRemoval(law.Entry)
		return
	}
	if len(law.Entry.Tests) == 0 {
		return
	}
	if err := replay.runTests(law); err != nil {
		detail := fmt.Sprintf("the approved proof of %s fails against this change (%s, %s): %v", law.Entry.Law, law.Entry.Module, law.Entry.Document, err)
		replay.result.Verdicts = append(replay.result.Verdicts, verdict{replayPhase, failed, detail})
		return
	}
	detail := fmt.Sprintf("%s (%s, %s)", law.Entry.Law, law.Entry.Module, law.Entry.Document)
	replay.result.Verdicts = append(replay.result.Verdicts, verdict{replayPhase, passed, detail})
}

func (replay baseReplay) checkRemoval(entry lawslock.Entry) {
	if entry.AllowRemoval {
		return
	}
	detail := fmt.Sprintf("agreed law %s (%s, %s) was removed or renamed; the owner must first allow removal in the base proof lock", entry.Law, entry.Module, entry.Document)
	replay.result.Verdicts = append(replay.result.Verdicts, verdict{replayPhase, failed, detail})
}

func (replay baseReplay) runTests(law lawslock.ApprovedLaw) error {
	proof, err := lawslock.Proof(law.Entry, replay.read)
	if err != nil {
		return err
	}
	if proof != law.Entry.Proof {
		return errors.New("the base test sources differ from the base proof lock")
	}
	dir, err := os.MkdirTemp("", "lawgate-replay-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	overlay, err := replay.makeOverlay(dir, law.Entry.TestFiles)
	if err != nil {
		return err
	}
	for _, test := range law.Entry.Tests {
		if err := runApprovedTest(replay.root, law.ModuleConfig, test, overlay); err != nil {
			return err
		}
	}
	return nil
}

func runApprovedTest(root string, module inventory.Module, test lawslock.Test, overlay string) error {
	dir := filepath.Join(root, module.Path)
	build, err := inventory.ParseGoCommand(dir, module.TestArgs())
	if err != nil {
		return err
	}
	if build.Subcommand != "test" {
		return errors.New("base replay requires a direct Go test command")
	}
	args, err := replayTestArgs(root, build, test)
	if err != nil {
		return err
	}
	flag, err := overlayGoFlag(overlay)
	if err != nil {
		return err
	}
	stdout, stderr, err := inventory.RunGateCommandWithEnv(dir, args, []string{flag}, module.Timeout())
	if err != nil {
		return fmt.Errorf("%s: %w", test.QualifiedName(), commandError(err, []byte(goTestText(stdout)), stderr))
	}
	return checkReplayEvidence(test, stdout)
}

func replayTestArgs(root string, build inventory.GoCommand, test lawslock.Test) ([]string, error) {
	pkg, err := filepath.Rel(build.Dir, filepath.Join(root, path.Dir(test.File)))
	if err != nil {
		return nil, err
	}
	operands := []string{"./" + filepath.ToSlash(pkg)}
	if test.Package == "command-line-arguments" {
		operands = build.Operands
	}
	args := build.SelectTest(test.Name, "", operands)
	index := slices.Index(args, "test")
	// Run the verifier's Go tool even if the base config uses a named wrapper.
	args[index-1] = "go"
	return args, nil
}

func checkReplayEvidence(test lawslock.Test, output []byte) error {
	evidence, err := goEvidence(output)
	if err != nil {
		return err
	}
	for _, item := range evidence.Verdicts {
		if item.Name == test.QualifiedName() && item.Status == passed {
			return nil
		}
	}
	return fmt.Errorf("approved test %s did not pass to completion", test.QualifiedName())
}

func overlayGoFlag(file string) (string, error) {
	flag := "-overlay=" + file
	for _, quote := range []string{"'", string('"')} {
		if !strings.Contains(flag, quote) {
			return "GOFLAGS=" + quote + flag + quote, nil
		}
	}
	return "", errors.New("temporary overlay path contains both single and double quotes")
}
