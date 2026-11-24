//go:build unix

package inventory

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func isolateCommand(command *exec.Cmd) {
	// Test runners and shell scripts can leave descendants holding the pipes.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
