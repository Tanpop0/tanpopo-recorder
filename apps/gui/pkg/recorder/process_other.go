//go:build !windows

package recorder

import "os/exec"

func prepareCommand(cmd *exec.Cmd) {}
