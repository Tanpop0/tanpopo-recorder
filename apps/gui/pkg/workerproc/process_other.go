//go:build !windows

package workerproc

import "os/exec"

func prepareCommand(cmd *exec.Cmd) {}
