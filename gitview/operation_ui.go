package gitview

import (
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
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
func (v *View) operationBanner(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	shown := func() bool {
		v.track()
		kind := v.repo.operation.Kind
		return kind != OperationNone && kind != OperationBisect
	}
	return core.Show{When: shown, Then: func(cx *core.Scope) core.Node {
		label := core.Text{ContentFunc: func() string { v.track(); return v.operationLabel(v.repo.operation.Kind) }, MaxLines: 1,
			Style: core.Style{FontSize: 12.5, FontWeight: ui.Bold, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Text)}
		message := core.Text{ContentFunc: func() string {
			v.track()
			if conflicts := v.conflictCount(); conflicts > 0 {
				return v.textf("operation_conflicts", map[string]any{"Count": conflicts})
			}
			return v.text("operation_resolved")
		}, MaxLines: 1, Style: core.Style{FontSize: 12, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Muted)}
		canContinue := func() bool { return v.CanSync() && v.current != nil && v.current.loaded && v.conflictCount() == 0 }
		continueButton := ui.Button{Text: v.text("operation_continue"), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "operation-continue",
			Disabled: v.is(func() bool { return !canContinue() }), OnPress: func() {
				if canContinue() {
					v.continueOperation()
				}
			}}
		abortButton := ui.Button{Text: v.text("operation_abort"), Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "operation-abort",
			Disabled: v.is(func() bool { return !v.CanSync() }), OnPress: func() {
				v.abortingOperation = true
				v.invalidate()
			}}
		bar := row(8, ui.Icon(lucide.TriangleAlert, 15, colors.Warn), label, message, continueButton, abortButton)
		bar.Style.Padding = core.Symmetric(6, 8)
		bar.StyleFunc = func() core.Style { return core.Style{Background: ui.Alpha(colors.Warn(), 0.12)} }
		edge := core.View{Style: core.Style{Height: core.Pt(1)}, StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Warn(), 0.35)} }}
		return core.View{Style: core.Style{FlexShrink: 0}, Children: []core.Node{bar, edge}}
	}}
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
func (v *View) abortOperationDialog(cx *core.Scope) core.Node {
	run := func() {
		v.abortingOperation = false
		v.syncRepository("abort", v.text("operation_abort_failed"), func(git Git, path string) error {
			return git.AbortOperation(path)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &v.abortingOperation, alert: true, testID: "abort-dialog",
		title: func() string {
			return v.textf("operation_abort_title", map[string]any{"Operation": strings.ToLower(v.operationLabel(v.repo.operation.Kind))})
		},
		description: func() string { return v.text("operation_abort_message") },
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&v.abortingOperation), v.confirmButton(v.text("operation_abort"), v.CanSync, run, "confirm")}
		}})
}
