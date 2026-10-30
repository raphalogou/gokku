//go:build !unix

package dokku

import "os/exec"

func setProcessGroup(*exec.Cmd) {}

func terminateGroup(c *exec.Cmd, _ bool) {
	if c.Process != nil {
		_ = c.Process.Kill()
	}
}
