package gitview

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// operationLabel names the operation that stopped in the worktree.
func (v *View) operationLabel(kind OperationKind) string {
	switch kind {
	case OperationMerge:
		return v.text("operation_merge")
	case OperationRebase:
		return v.text("operation_rebase")
	case OperationAm:
		return v.text("operation_am")
	case OperationCherryPick:
		return v.text("operation_cherry_pick")
	case OperationRevert:
		return v.text("operation_revert")
	case OperationSquash:
		return v.text("operation_squash")
	}
	return ""
}

// conflictCount counts the conflicted files in the shown worktree's changes.
func (v *View) conflictCount() int {
	count := 0
	if v.current == nil {
		return 0
	}
	for _, f := range v.current.changes.Files {
		if f.Status == 'U' {
			count++
		}
	}
	return count
}

// operationBanner shows the stopped operation above the section, with the
// conflicts left and the actions that finish or undo it.
func (v *View) operationBanner(c *ui.Context) {
	kind := v.repo.operation.Kind
	if kind == OperationNone || kind == OperationBisect {
		return
	}
	t := c.Theme()
	conflicts := v.conflictCount()
	label := v.operationLabel(kind)
	ui.Column(c).Key("operation").FillWidth().Shrink(0).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Padding(t.Space(1.5), t.Space(2)).
			Background(t.Warning.Alpha(.12)).Children(func() {
			components.Icon(c, lucide.TriangleAlert).Size(15, 15).Shrink(0).TextColor(t.Warning)
			components.Text(c, label).FontSize(12.5).Bold().Shrink(0)
			message := v.text("operation_resolved")
			if conflicts > 0 {
				message = v.textf("operation_conflicts", map[string]any{"Count": conflicts})
			}
			components.Text(c, message).FontSize(12).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").TextColor(t.TextMuted)
			canContinue := v.CanSync() && v.current != nil && v.current.loaded && conflicts == 0
			if components.Button(c, v.text("operation_continue"), components.ButtonSecondary).Height(24).FontSize(12).
				Padding(0, t.Space(2)).Disabled(!canContinue).Clicked() && canContinue {
				v.continueOperation()
			}
			if components.Button(c, v.text("operation_abort"), components.ButtonSecondary).Height(24).FontSize(12).
				Padding(0, t.Space(2)).Disabled(!v.CanSync()).Clicked() {
				v.abortingOperation = true
				v.invalidate()
			}
		})
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Warning.Alpha(.35))
	})
}

// continueOperation finishes the operation that stopped.
func (v *View) continueOperation() {
	if !v.CanSync() {
		return
	}
	v.syncRepository("continue", v.text("operation_continue_failed"), func(git Git, path string) error {
		return git.ContinueOperation(path)
	})
}

// abortOperationDialog asks before undoing the operation that stopped.
func (v *View) abortOperationDialog(c *ui.Context) {
	if !v.abortingOperation {
		return
	}
	kind := v.repo.operation.Kind
	label := v.operationLabel(kind)
	title := v.textf("operation_abort_title", map[string]any{"Operation": strings.ToLower(label)})
	components.Dialog(c, &v.abortingOperation, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Role(ui.RoleAlertDialog)
		components.Text(c, title).Bold().Wrap().FillWidth()
		components.Text(c, v.text("operation_abort_message")).Wrap().FillWidth().TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).AutoFocus().Clicked() {
				v.abortingOperation = false
			}
			if components.Button(c, v.text("operation_abort"), components.ButtonPrimary).Disabled(!v.CanSync()).Clicked() {
				v.abortingOperation = false
				v.syncRepository("abort", v.text("operation_abort_failed"), func(git Git, path string) error {
					return git.AbortOperation(path)
				})
			}
		})
	})
}
