package gen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// Target supplies the declaration and options used to generate Path.
type Target struct {
	Path    string
	Machine *machine.Machine
	Options Options
}

// Check regenerates each target in memory and returns missing or different
// paths in input order. Generation and file errors are returned separately
// from stale paths. Use ReportCheck in a configured freshness command.
func Check(targets ...Target) ([]string, error) {
	var stale []string
	for _, target := range targets {
		status, err := targetStatus(target)
		if err != nil {
			return stale, err
		}
		if status != Fresh {
			stale = append(stale, target.Path)
		}
	}
	return stale, nil
}

// FreshnessRecord identifies a target result on a check command’s stdout.
const FreshnessRecord = "lawgate-freshness"

const (
	Fresh   = "fresh"
	Stale   = "stale"
	Missing = "missing"
)

// ReportCheck compares every declared target and writes its result for the
// gate. It returns an error if any target is missing, stale or unreadable.
func ReportCheck(out io.Writer, targets ...Target) error {
	if len(targets) == 0 {
		return fmt.Errorf("gen check has no targets")
	}
	var problems []error
	for _, target := range targets {
		problems = append(problems, reportTarget(out, target))
	}
	return errors.Join(problems...)
}

func reportTarget(out io.Writer, target Target) error {
	localFile := fs.ValidPath(target.Path) && target.Path != "."
	if !localFile || strings.ContainsAny(target.Path, "\\\t\r\n") {
		return fmt.Errorf("invalid generated target path %q", target.Path)
	}
	status, err := targetStatus(target)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\t%s\t%s\n", FreshnessRecord, status, target.Path)
	if err != nil {
		return err
	}
	if status != Fresh {
		return fmt.Errorf("%s: %s; run %s", target.Path, status, target.Options.Command)
	}
	return nil
}

func targetStatus(target Target) (string, error) {
	if target.Machine == nil {
		return "", fmt.Errorf("gen check %q: no machine", target.Path)
	}
	want, err := Go(target.Machine, target.Options)
	if err != nil {
		return "", fmt.Errorf("gen check %q: %w", target.Path, err)
	}
	have, err := os.ReadFile(target.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return Missing, nil
	}
	if err != nil {
		return "", err
	}
	stale := !bytes.Equal(have, want)
	if stale {
		return Stale, nil
	}
	return Fresh, nil
}
