//go:build !unix

package agent

import (
	"os"
	"os/exec"
)

func configureProcess(_ *exec.Cmd) {}

func stopProcess(proc *os.Process) error {
	return proc.Kill()
}
