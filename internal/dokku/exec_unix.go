//go:build unix

package dokku

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateGroup signals the command's whole process group: SIGTERM, or SIGKILL if kill.
func terminateGroup(c *exec.Cmd, kill bool) {
	if c.Process == nil {
		return
	}
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-c.Process.Pid, sig)
}
