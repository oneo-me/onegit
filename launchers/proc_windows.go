package launchers

import (
	"os/exec"
	"syscall"
)

// hideConsole keeps an editor's console shim from opening a window.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// HideWindow alone can still let a console briefly appear.
	const createNoWindow = 0x08000000 // CREATE_NO_WINDOW
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
