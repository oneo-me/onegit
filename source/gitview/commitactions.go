package gitview

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Git writes of the history rows' menus and the tag menu.
const (
	opReset      = "reset"
	opCherryPick = "cherry-pick"
	opRevert     = "revert"
	opPushTag    = "push-tag"
)

// resetDialog asks how to move the checked out branch to a revision.
type resetDialog struct {
	open   bool
	target refTarget
	mode   ResetMode
}

// openReset asks how to reset the checked out branch to t.
func (v *View) openReset(t refTarget) {
	if v.CanSync() {
		v.reset = resetDialog{open: true, target: t, mode: ResetMixed}
		v.invalidate()
	}
}

// resetDialogView asks for the reset type and runs the reset.
func (v *View) resetDialogView(c *ui.Context) {
	d := &v.reset
	args := map[string]any{"Name": d.target.name, "Current": v.checkedOutName()}
	title := v.text("ref_reset_title")
	description := v.textf("ref_reset_description", args)
	if v.tracking.Branch == "" {
		title = v.text("ref_reset_head_title")
		description = v.text("ref_reset_head_description")
	}
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			components.Text(c, description).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		formField(c, refLabelWidth, v.text("ref_reset_source"), false, func() { v.checkedOutChoice(c) })
		formField(c, refLabelWidth, v.text("ref_reset_destination"), false, func() { v.targetChoice(c, d.target) })
		formField(c, refLabelWidth, v.text("ref_reset_mode"), false, func() {
			ui.Column(c).Gap(t.Space(2)).Children(func() {
				components.Radio(c, &d.mode, ResetSoft, v.text("ref_reset_soft"))
				components.Radio(c, &d.mode, ResetMixed, v.text("ref_reset_mixed"))
				components.Radio(c, &d.mode, ResetHard, v.text("ref_reset_hard"))
			})
		})
		if d.mode == ResetHard {
			components.Text(c, v.text("ref_reset_hard_hint")).Wrap().FillWidth().TextColor(t.Warning)
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			b := components.Button(c, v.text("ref_reset_confirm"), components.ButtonPrimary).Disabled(!v.CanSync()).AutoFocus()
			if b.Clicked() && v.CanSync() {
				target, mode := d.target, d.mode
				d.open = false
				v.Run(opReset, v.text("ref_reset_failed"), func(git Git, path string) error {
					return git.Reset(path, target.rev(), mode)
				})
			}
		})
	})
}

// cherryPickDialog asks how to apply a commit to the checked out branch.
type cherryPickDialog struct {
	open                        bool
	commit                      Commit
	parent                      int
	commitChanges, appendOrigin bool
}

// openCherryPick asks how to apply commit.
func (v *View) openCherryPick(commit Commit) {
	if !v.CanSync() {
		return
	}
	d := cherryPickDialog{open: true, commit: commit, commitChanges: true, appendOrigin: v.state.CherryPickAppend}
	if len(commit.Parents) > 1 {
		d.parent = 1
	}
	v.cherryPick = d
	v.invalidate()
}

// cherryPickDialogView asks for the parent of a merge commit and whether to
// record a commit, then cherry-picks it.
func (v *View) cherryPickDialogView(c *ui.Context) {
	d := &v.cherryPick
	title := v.text("ref_cherry_pick_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text("ref_cherry_pick_description")).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		formField(c, refLabelWidth, v.text("ref_cherry_pick_commit"), false, func() { v.targetChoice(c, commitTarget(d.commit)) })
		v.parentField(c, d.commit, &d.parent)
		formOption(c, refLabelWidth, &d.commitChanges, v.text("ref_commit_changes"))
		if d.commitChanges {
			formOption(c, refLabelWidth, &d.appendOrigin, v.text("ref_append_origin"))
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			b := components.Button(c, v.text("ref_cherry_pick_confirm"), components.ButtonPrimary).Disabled(!v.CanSync()).AutoFocus()
			if b.Clicked() && v.CanSync() {
				commit, parent := d.commit, d.parent
				o := CherryPickOptions{Commit: d.commitChanges, AppendOrigin: d.appendOrigin && d.commitChanges, Parent: parent}
				d.open = false
				v.state.CherryPickAppend = o.AppendOrigin
				v.saveState()
				v.Run(opCherryPick, v.text("ref_cherry_pick_failed"), func(git Git, path string) error {
					return git.CherryPick(path, commit.Hash, o)
				})
			}
		})
	})
}

// revertDialog asks how to undo a commit in the checked out branch.
type revertDialog struct {
	open         bool
	commit       Commit
	parent       int
	commitChange bool
}

// openRevert asks how to revert commit.
func (v *View) openRevert(commit Commit) {
	if !v.CanSync() {
		return
	}
	d := revertDialog{open: true, commit: commit, commitChange: true}
	if len(commit.Parents) > 1 {
		d.parent = 1
	}
	v.revert = d
	v.invalidate()
}

// revertDialogView asks for the parent of a merge commit and whether to
// record a commit, then reverts it.
func (v *View) revertDialogView(c *ui.Context) {
	d := &v.revert
	title := v.text("ref_revert_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text("ref_revert_description")).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		formField(c, refLabelWidth, v.text("ref_revert_commit"), false, func() { v.targetChoice(c, commitTarget(d.commit)) })
		v.parentField(c, d.commit, &d.parent)
		formOption(c, refLabelWidth, &d.commitChange, v.text("ref_commit_changes"))
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			b := components.Button(c, v.text("ref_revert_confirm"), components.ButtonPrimary).Disabled(!v.CanSync()).AutoFocus()
			if b.Clicked() && v.CanSync() {
				commit, parent := d.commit, d.parent
				o := RevertOptions{Commit: d.commitChange, Parent: parent}
				d.open = false
				v.Run(opRevert, v.text("ref_revert_failed"), func(git Git, path string) error {
					return git.Revert(path, commit.Hash, o)
				})
			}
		})
	})
}

// parentField chooses the parent of a merge commit to act relative to.
func (v *View) parentField(c *ui.Context, commit Commit, parent *int) {
	if len(commit.Parents) < 2 {
		return
	}
	*parent = min(max(*parent, 1), len(commit.Parents))
	syncField(c, v.text("ref_parent"), false, func() {
		sel := components.Select(c, parent)
		sel.Trigger.Label(v.text("ref_parent")).FillWidth().Children(func() {
			v.commitChoice(c, commit.Parents[*parent-1])
		})
		sel.Popup(func(panel ui.Element) {
			syncChoices(c, panel, func() {
				for i, hash := range commit.Parents {
					sel.Item(i + 1).Label(v.commitLabel(hash)).Children(func() { v.commitChoice(c, hash) })
				}
			})
		})
	})
}

// commitChoice shows a commit by its short hash and subject.
func (v *View) commitChoice(c *ui.Context, hash string) {
	syncChoice(c, lucide.GitCommitVertical, v.commitLabel(hash))
}

// commitLabel names a commit by its short hash and subject when the history
// read holds it.
func (v *View) commitLabel(hash string) string {
	if i := commitIndex(v.repo.commits, hash); i >= 0 {
		commit := v.repo.commits[i]
		return commit.Short + " " + commit.Subject
	}
	return hash[:min(len(hash), 10)]
}

// pushTagDialog sends a tag to a remote.
type pushTagDialog struct {
	open        bool
	tag, remote string
}

// openPushTag asks for the remote to send tag to.
func (v *View) openPushTag(tag, remote string) {
	if !v.CanSync() {
		return
	}
	v.pushTag = pushTagDialog{open: true, tag: tag, remote: remote}
	v.invalidate()
}

// pushTagDialogView chooses the remote and pushes the tag.
func (v *View) pushTagDialogView(c *ui.Context) {
	d := &v.pushTag
	title := v.text("ref_push_tag_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(100))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text("ref_push_tag_description")).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		formField(c, refLabelWidth, v.text("ref_push_tag_tag"), false, func() {
			v.targetChoice(c, refTarget{group: "tags", name: d.tag})
		})
		remotes := v.remotes()
		if len(remotes) == 0 {
			components.Text(c, v.text("no_remotes")).Wrap().FillWidth().TextColor(t.TextMuted)
		}
		syncField(c, v.text("sync_remote"), len(remotes) == 0, func() {
			sel := components.Select(c, &d.remote)
			sel.Trigger.Label(v.text("sync_remote")).FillWidth().Children(func() { v.remoteChoice(c, d.remote) })
			sel.Popup(func(panel ui.Element) {
				syncChoices(c, panel, func() {
					for _, remote := range remotes {
						sel.Item(remote).Label(remote).Children(func() { v.remoteChoice(c, remote) })
					}
				})
			})
		})
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			b := components.Button(c, v.text("push"), components.ButtonPrimary).
				Disabled(!v.CanSync() || d.remote == "").AutoFocus()
			if b.Clicked() && d.remote != "" {
				tag, remote := d.tag, d.remote
				d.open = false
				v.syncRepository(opPushTag, v.text("ref_push_tag_failed"), func(git Git, path string) error {
					return git.PushTag(path, remote, tag)
				})
			}
		})
	})
}

// checkedOutChoice names the branch the worktree has checked out, or HEAD.
func (v *View) checkedOutChoice(c *ui.Context) {
	icon := lucide.GitBranch
	if v.tracking.Branch == "" {
		icon = lucide.CircleDot
	}
	syncChoice(c, icon, v.checkedOutName())
}

// saveCommitPatch writes a commit's patch to a file the user chooses.
func (v *View) saveCommitPatch(commit Commit) {
	path, git := v.path, v.git
	title, failed := v.text("ref_save_patch_title"), v.text("ref_save_patch_failed")
	go func() {
		data, err := git.CommitPatch(path, commit.Hash)
		if err == nil {
			_, err = v.saveFile(title, commit.Short+".patch", data)
		}
		if err != nil {
			v.dispatch(func() {
				v.toast = &ui.Toast{Title: failed, Description: err.Error(), Type: components.ToastError}
				v.invalidate()
			})
		}
	}()
}

// saveFile asks where to write data and writes it there, unless a host
// provides SaveFile.
func (v *View) saveFile(title, name string, data []byte) (string, error) {
	if v.opts.SaveFile != nil {
		return v.opts.SaveFile(title, name, data)
	}
	options := mygo.SaveDialogOptions{Title: title, DefaultPath: name}
	if ext := strings.TrimPrefix(filepath.Ext(name), "."); ext != "" {
		options.Filters = []mygo.FileFilter{{Name: title, Extensions: []string{ext}}}
	}
	path, err := mygo.Dialog.Save(options)
	if err != nil || path == "" {
		return path, err
	}
	return path, os.WriteFile(path, data, 0o644)
}

// remotes lists the repository's remotes by name.
func (v *View) remotes() []string {
	return slices.Sorted(maps.Keys(v.repo.github))
}
