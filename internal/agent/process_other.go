//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package agent

import (
	"os"
	"os/exec"
)

func configureProcess(_ *exec.Cmd) {}

func stopProcess(proc *os.Process) error {
	return proc.Kill()
}
