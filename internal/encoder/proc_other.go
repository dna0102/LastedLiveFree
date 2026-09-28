//go:build !windows

package encoder

import "os/exec"

func configureProc(cmd *exec.Cmd) {}
func assignToJob(cmd *exec.Cmd)   {}

// killTree kills the process; ffmpeg doesn't start children here.
func killTree(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
