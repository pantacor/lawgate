//go:build !unix

package inventory

import "os/exec"

func isolateCommand(_ *exec.Cmd) {}
