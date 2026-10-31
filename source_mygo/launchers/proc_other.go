//go:build !windows

package launchers

import "os/exec"

func hideConsole(*exec.Cmd) {}
