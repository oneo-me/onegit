package gitview

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// interactiveRebaseDialog edits the commits from a base to the checked out
// branch and the action each one takes.
type interactiveRebaseDialog struct {
	open, loading bool
	generation    uint64
	base          string
	rows          []RebaseCommit
	err           string
	autostash     bool
	selected      int
}

// rebaseActionLabel names an action.
func (v *View) rebaseActionLabel(action RebaseAction) string {
	switch action {
	case RebaseReword:
		return v.text("rebase_reword")
	case RebaseEdit:
		return v.text("rebase_edit")
	case RebaseSquash:
		return v.text("rebase_squash")
	case RebaseFixup:
		return v.text("rebase_fixup")
	case RebaseDrop:
		return v.text("rebase_drop")
	}
	return v.text("rebase_pick")
}

// openInteractiveRebase reads the commits from base to HEAD and opens the
// action list.
func (v *View) openInteractiveRebase(base string) {
	if !v.CanSync() || v.tracking.Branch == "" || v.git.Host != nil {
		return
	}
	d := &v.interactive
	d.open, d.loading, d.base, d.err, d.rows = true, true, base, "", nil
	d.autostash = v.state.RebaseAutostash
	d.generation++
	generation, git, path := d.generation, v.git, v.path
	v.invalidate()
	go func() {
		commits, err := git.RebaseCommits(path, base)
		v.dispatch(func() {
			if generation != d.generation || v.closed {
				return
			}
			d.loading = false
			if err != nil {
				d.err = err.Error()
			} else {
				for _, commit := range commits {
					d.rows = append(d.rows, RebaseCommit{Commit: commit, Action: RebasePick, Message: fullMessage(commit)})
				}
				d.selected = 0
			}
			v.invalidate()
		})
	}()
}

// runInteractiveRebase replays the dialog's rows.
func (v *View) runInteractiveRebase() {
	d := &v.interactive
	if len(d.rows) == 0 {
		return
	}
	rows, base, autostash := append([]RebaseCommit(nil), d.rows...), d.base, d.autostash
	if autostash != v.state.RebaseAutostash {
		v.state.RebaseAutostash = autostash
		v.saveState()
	}
	d.open = false
	v.Run(opRebase, v.text("ref_rebase_failed"), func(git Git, path string) error {
		return git.InteractiveRebase(path, base, rows, autostash)
	})
}

// interactiveRebaseDialogView shows the commits and the action each takes.
func (v *View) interactiveRebaseDialogView(cx *core.Scope) core.Node {
	d := &v.interactive
	ready := func() bool { return !d.loading && len(d.rows) > 0 && v.CanSync() }
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 600, height: 600, testID: "interactive-rebase-dialog",
		title: func() string {
			return v.textf("interactive_rebase_title", map[string]any{"Current": v.checkedOutName(), "Name": v.commitLabel(d.base)})
		},
		description: func() string { return v.text("interactive_rebase_hint") },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			key := func() string { v.track(); return fmt.Sprint(d.loading, d.err, len(d.rows), d.generation) }
			list := region(cx, key, func(cx *core.Scope) core.Node {
				switch {
				case d.loading:
					return centered(spinner(24))
				case d.err != "":
					return dialogNote(d.err, 13, colors.Danger)
				case len(d.rows) == 0:
					return muted(cx, v.text("interactive_rebase_empty"), 13)
				}
				var rows []core.Node
				for i := range d.rows {
					rows = append(rows, v.interactiveRebaseRow(cx, i))
				}
				return ui.ScrollView{Style: ui.FillRest, ContentStyle: core.Style{Gap: 2}, Children: rows}
			})
			// The selected reworded commit's message is edited below the list.
			message := region(cx, func() string {
				v.track()
				if d.selected >= 0 && d.selected < len(d.rows) && d.rows[d.selected].Action == RebaseReword {
					return fmt.Sprint(d.selected, d.rows[d.selected].Commit.Hash)
				}
				return ""
			}, func(cx *core.Scope) core.Node {
				if d.selected < 0 || d.selected >= len(d.rows) || d.rows[d.selected].Action != RebaseReword {
					return nil
				}
				r := &d.rows[d.selected]
				return column(4, muted(cx, v.textf("interactive_rebase_message", map[string]any{"Name": r.Commit.Subject}), 12),
					ui.Input{Multiline: true, Label: v.text("commit_message_label"), TestID: "reword-message",
						Value: func() string { v.watch(); return r.Message }, OnChange: func(s string) { r.Message = s; v.edited() },
						Style: core.Style{FontSize: 12, MinHeight: core.Pt(48), MaxHeight: core.Pt(120)}})
			})
			return []core.Node{list, message, v.toggle(cx, &d.autostash, v.text("rebase_autostash"), nil)}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("rebase_confirm"), ready, v.runInteractiveRebase, "confirm")}
		}})
}

// interactiveRebaseRow is a commit's row with its action, which its menu
// chooses, and buttons moving it up and down.
func (v *View) interactiveRebaseRow(cx *core.Scope, i int) core.Node {
	colors := ui.ColorsOf(cx)
	d := &v.interactive
	r := &d.rows[i]
	action := line(v.rebaseActionLabel(r.Action), 11, ui.Bold, colors.Accent)
	action.Style.Width, action.Style.FlexShrink = core.Pt(56), 0
	hash := line(r.Commit.Short, 11, ui.Regular, colors.Muted)
	hash.Style.Width, hash.Style.FlexShrink, hash.Style.FontFamily = core.Pt(70), 0, ui.Mono
	subject := line(r.Commit.Subject, 12.5, ui.Regular, colors.Text)
	subject.Style.FlexGrow = 1
	move := func(to int) func() {
		return func() {
			if to < 0 || to >= len(d.rows) {
				return
			}
			d.rows[i], d.rows[to] = d.rows[to], d.rows[i]
			d.selected = to
			v.invalidate()
		}
	}
	row := row8(action, hash, subject,
		v.iconButton(v.text("rebase_move_up"), lucide.ChevronUp, 20, nil, func() bool { return i == 0 }, move(i-1), fmt.Sprint("move-up-", i)),
		v.iconButton(v.text("rebase_move_down"), lucide.ChevronDown, 20, nil, func() bool { return i+1 >= len(d.rows) }, move(i+1), fmt.Sprint("move-down-", i)))
	row.Style.Height, row.Style.CornerRadius = core.Pt(38), 6
	row.TestID = fmt.Sprint("rebase-row-", i)
	row.Selected = func() bool { v.track(); return d.selected == i }
	row.StyleFunc = func() core.Style { return core.Style{Selected: &core.Style{Background: ui.Alpha(colors.Text(), 0.1)}} }
	row.OnPress = func(core.PressEvent) {
		d.selected = i
		v.invalidate()
	}
	row.OnContextMenu = v.menuOn(cx, func() []core.MenuItem {
		var items []core.MenuItem
		for _, a := range []RebaseAction{RebasePick, RebaseReword, RebaseEdit, RebaseSquash, RebaseFixup, RebaseDrop} {
			disabled := i == 0 && (a == RebaseSquash || a == RebaseFixup)
			items = append(items, core.MenuItem{Label: v.rebaseActionLabel(a), Checkable: true, Checked: r.Action == a, Disabled: disabled, OnSelect: func() {
				r.Action = a
				d.selected = i
				v.invalidate()
			}})
		}
		return items
	})
	return row
}
