//go:build !windows

package gitview

import "os/exec"

func hideConsole(*exec.Cmd) {}
