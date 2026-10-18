package gitview

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// rebaseDialog is the rebase dialog of a ref's or commit's menu: the
// checked out branch, the revision it is replayed onto, the local branches
// pointing into the replay range and whether to move them, and whether to
// set local changes aside. loading holds reading those; generation discards
// a read the user replaced.
type rebaseDialog struct {
	open       bool
	target     refTarget
	generation uint64
	loading    bool
	err        string
	branches   []string
	check      RebaseCheck
	autostash  bool
	updateRefs bool
}

// openRebase opens the rebase dialog for t, reading the branches the rebase
// moves and testing it for conflicts in the background.
func (v *View) openRebase(t refTarget) {
	if !v.CanSync() || v.tracking.Branch == "" {
		return
	}
	d := rebaseDialog{
		open: true, target: t, generation: v.rebase.generation + 1, loading: true,
		autostash: v.state.RebaseAutostash, updateRefs: v.state.RebaseUpdateRefs,
	}
	v.rebase = d
	v.invalidate()
	path, git := v.path, v.git
	source, destination := "refs/heads/"+v.tracking.Branch, t.rev()
	go func() {
		hash, err := git.Run(path, "rev-parse", "--verify", destination+"^{commit}")
		var branches []string
		check := RebaseCheckUnknown
		if err == nil {
			branches, _ = git.LocalBranchesInRange(path, source, hash)
			check, _ = git.CheckRebase(path, source, hash)
		}
		v.dispatch(func() {
			current := &v.rebase
			if current.generation != d.generation || !current.open || v.closed {
				return
			}
			current.loading = false
			if err != nil {
				current.err = err.Error()
			}
			current.branches, current.check = branches, check
			v.invalidate()
		})
	}()
}

// rebaseDialogView shows the rebase dialog and runs the rebase it confirms.
func (v *View) rebaseDialogView(c *ui.Context) {
	d := &v.rebase
	args := map[string]any{"Name": d.target.name, "Current": v.checkedOutName()}
	title := v.textf("rebase_title", args)
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			components.Text(c, v.textf("rebase_description", args)).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		formField(c, refLabelWidth, v.text("rebase_source"), false, func() { v.checkedOutChoice(c) })
		formField(c, refLabelWidth, v.text("rebase_destination"), false, func() { v.targetChoice(c, d.target) })
		if len(d.branches) > 0 {
			formOption(c, refLabelWidth, &d.updateRefs, v.text("rebase_update_refs"))
			if d.updateRefs {
				components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(t.Space(24)).
					Padding(0, 0, 0, t.Space(refLabelWidth+3)).Children(func() {
					ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
						for _, branch := range d.branches {
							syncChoice(c, lucide.GitBranch, branch)
						}
					})
				})
			}
		}
		formOption(c, refLabelWidth, &d.autostash, v.text("rebase_autostash"))
		switch {
		case d.loading:
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				components.Spinner(c, components.SpinnerRing).Size(14, 14)
				components.Text(c, v.text("rebase_checking")).FontSize(12).TextColor(t.TextMuted)
			})
		case d.err != "":
			components.Text(c, d.err).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted)
		case d.check == RebaseCheckClean:
			components.Text(c, v.text("rebase_clean")).FontSize(12).Wrap().FillWidth().TextColor(t.Success)
		case d.check == RebaseCheckConflict:
			components.Text(c, v.text("rebase_conflict")).FontSize(12).Wrap().FillWidth().TextColor(t.Warning)
		}
		if d.autostash != v.state.RebaseAutostash || d.updateRefs != v.state.RebaseUpdateRefs {
			v.state.RebaseAutostash, v.state.RebaseUpdateRefs = d.autostash, d.updateRefs
			v.saveState()
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			ready := v.CanSync() && !d.loading
			b := components.Button(c, v.text("rebase_confirm"), components.ButtonPrimary).
				Disabled(!ready).AutoFocus()
			if b.Clicked() && ready {
				target := d.target
				o := IntegrateOptions{Stash: d.autostash, UpdateRefs: d.updateRefs && len(d.branches) > 0}
				d.open = false
				v.Run(opRebase, v.text("ref_rebase_failed"), func(git Git, path string) error {
					return git.Rebase(path, target.rev(), o)
				})
			}
		})
	})
}
