package inventory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const CommandTimeout = 5 * time.Minute
const CommandOutputLimit = 16 << 20

// RunCommand bounds the subprocess and each of its two captured streams.
func RunCommand(dir string, args []string, timeout time.Duration) ([]byte, string, error) {
	return runCommand(dir, args, nil, timeout)
}

// RunGateCommand executes a configured command under the same Go selection
// rules as source discovery and ownership.
func RunGateCommand(dir string, args []string, timeouts ...time.Duration) ([]byte, string, error) {
	return RunGateCommandWithEnv(dir, args, nil, timeoutOrDefault(timeouts))
}

// RunGateCommandWithEnv supplies trusted overrides after configured build selection.
func RunGateCommandWithEnv(dir string, args, overrides []string, timeout time.Duration) ([]byte, string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", err
	}
	parsed, err := ParseGoCommand(absolute, args)
	if err != nil {
		return nil, "", err
	}
	if parsed.Subcommand == "" {
		return runCommand(dir, args, append(os.Environ(), overrides...), timeout)
	}
	if err := parsed.repositoryContext(); err != nil {
		return nil, "", err
	}
	_, args = goInvocation(args)
	return runCommand(absolute, args, append(parsed.goEnvironment(), overrides...), timeout)
}

func (command GoCommand) runList(args []string, timeout time.Duration) ([]byte, string, error) {
	if err := command.repositoryContext(); err != nil {
		return nil, "", err
	}
	return runCommand(command.Dir, args, command.goEnvironment(), timeout)
}

func timeoutOrDefault(timeouts []time.Duration) time.Duration {
	if len(timeouts) == 0 {
		return CommandTimeout
	}
	return timeouts[0]
}

func (command GoCommand) goEnvironment() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GOFLAGS", "GOWORK", "GOENV":
			continue
		}
		env = append(env, entry)
	}
	// Keep client-local Go configuration from replacing the committed context.
	env = append(env, "GOENV=off")
	env = append(env, command.environment...)
	if command.Workspace != "" {
		env = append(env, "GOWORK="+command.Workspace)
	}
	return env
}

func runCommand(dir string, args, env []string, timeout time.Duration) ([]byte, string, error) {
	if len(args) == 0 {
		return nil, "", fmt.Errorf("empty command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Dir = dir
	command.Env = env
	command.WaitDelay = time.Second
	isolateCommand(command)
	stdout := limitedOutput{cancel: cancel}
	stderr := limitedOutput{cancel: cancel}
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	switch {
	case stdout.exceeded || stderr.exceeded:
		err = fmt.Errorf("command exceeded output limit of %d bytes per stream", CommandOutputLimit)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		err = fmt.Errorf("command timed out after %s: %w", timeout, context.DeadlineExceeded)
	}
	return stdout.buf.Bytes(), stderr.buf.String(), err
}

type limitedOutput struct {
	buf      bytes.Buffer
	exceeded bool
	cancel   context.CancelFunc
}

func (out *limitedOutput) Write(chunk []byte) (int, error) {
	remaining := CommandOutputLimit - out.buf.Len()
	if len(chunk) > remaining {
		out.exceeded = true
		out.cancel()
	}
	_, _ = out.buf.Write(chunk[:min(remaining, len(chunk))])
	return len(chunk), nil
}
