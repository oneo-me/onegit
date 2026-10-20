package gitview

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
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
	list          components.ListState
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
	d.list.Selected = &d.selected
	d.list.Key = func(i int) any { return d.rows[i].Commit.Hash }
	d.list.Label = func(i int) string { return d.rows[i].Commit.Subject }
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
func (v *View) interactiveRebaseDialogView(c *ui.Context) {
	d := &v.interactive
	if !d.open {
		return
	}
	title := v.textf("interactive_rebase_title", map[string]any{"Current": v.checkedOutName(), "Name": v.commitLabel(d.base)})
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		w, h := c.Size()
		panel.Width(min(t.Space(150), max(0, w-2*v.dialogInset(c)))).Height(min(t.Space(150), h*.82)).Padding(t.Space(3))
		ui.Column(c).FillWidth().FillHeight().MinHeight(0).Gap(t.Space(2)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			components.Text(c, v.text("interactive_rebase_hint")).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted)
			switch {
			case d.loading:
				ui.Column(c).Fill().Center().Children(func() { components.Spinner(c, components.SpinnerRing) })
				return
			case d.err != "":
				components.Text(c, d.err).Wrap().FillWidth().TextColor(t.Danger)
			case len(d.rows) == 0:
				components.Text(c, v.text("interactive_rebase_empty")).Wrap().FillWidth().TextColor(t.TextMuted)
			default:
				components.List(c, &d.list, len(d.rows), func(i int) {
					v.interactiveRebaseRow(c, i)
				}).Fill().MinHeight(0)
				if d.selected >= 0 && d.selected < len(d.rows) && d.rows[d.selected].Action == RebaseReword {
					components.Text(c, v.textf("interactive_rebase_message", map[string]any{"Name": d.rows[d.selected].Commit.Subject})).
						FontSize(12).TextColor(t.TextMuted)
					components.TextBox(c, &d.rows[d.selected].Message, components.TextBoxOptions{Multiline: true}).
						FillWidth().Lines(2, 5).FontSize(12)
				}
			}
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				components.Switch(c, &d.autostash, v.text("rebase_autostash"))
				ui.Spacer(c)
				if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
					d.open = false
				}
				if components.Button(c, v.text("rebase_confirm"), components.ButtonPrimary).Disabled(d.loading || len(d.rows) == 0 || !v.CanSync()).Clicked() {
					v.runInteractiveRebase()
				}
			})
		})
	})
}

func (v *View) interactiveRebaseRow(c *ui.Context, i int) {
	t := c.Theme()
	d := &v.interactive
	row := &d.rows[i]
	ui.Row(c).FillWidth().Height(38).AlignItems(ui.Center).Gap(t.Space(2)).Padding(0, t.Space(1)).
		ContextMenu(func(m *ui.Menu) {
			for _, action := range []RebaseAction{RebasePick, RebaseReword, RebaseEdit, RebaseSquash, RebaseFixup, RebaseDrop} {
				disabled := i == 0 && (action == RebaseSquash || action == RebaseFixup)
				if m.Item(v.rebaseActionLabel(action)).Checked(row.Action == action).Disabled(disabled).Chosen() && !disabled {
					row.Action = action
					d.selected = i
					v.invalidate()
				}
			}
		}).Children(func() {
		components.Text(c, v.rebaseActionLabel(row.Action)).FontSize(11).Bold().Width(56).Shrink(0).TextColor(t.Accent)
		components.Text(c, row.Commit.Short).Font("SF Mono, Menlo, monospace").FontSize(11).Width(70).Shrink(0).TextColor(t.TextMuted).SingleLine()
		components.Text(c, row.Commit.Subject).FontSize(12.5).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if iconButton(c, v.text("rebase_move_up"), lucide.ChevronUp, 20, false).Disabled(i == 0).Clicked() && i > 0 {
			d.rows[i-1], d.rows[i] = d.rows[i], d.rows[i-1]
			d.selected = i - 1
			v.invalidate()
		}
		if iconButton(c, v.text("rebase_move_down"), lucide.ChevronDown, 20, false).Disabled(i+1 >= len(d.rows)).Clicked() && i+1 < len(d.rows) {
			d.rows[i], d.rows[i+1] = d.rows[i+1], d.rows[i]
			d.selected = i + 1
			v.invalidate()
		}
	})
}
