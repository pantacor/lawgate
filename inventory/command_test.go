package inventory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCommandEnforcesTimeout(t *testing.T) {
	args := []string{"sh", "-c", "sleep 0.25 & wait"}
	_, _, err := RunCommand(t.TempDir(), args, 25*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("command returned %v, want a timeout", err)
	}
}

func TestCommandBoundsBothOutputStreams(t *testing.T) {
	commands := []string{"head -c 17000000 /dev/zero", "head -c 17000000 /dev/zero >&2"}
	for _, script := range commands {
		assertLimitedCommand(t, script)
	}
}

func assertLimitedCommand(t *testing.T, script string) {
	t.Helper()
	args := []string{"sh", "-c", script}
	stdout, stderr, err := RunCommand(t.TempDir(), args, 5*time.Second)
	missingError := err == nil || !strings.Contains(err.Error(), "output limit")
	if missingError || len(stdout)+len(stderr) != CommandOutputLimit {
		t.Fatalf("stdout %d bytes, stderr %d bytes, err %v", len(stdout), len(stderr), err)
	}
}
