package gitview

import (
	"maps"
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
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
func (v *View) resetDialogView(cx *core.Scope) core.Node {
	d := &v.reset
	run := func() {
		target, mode := d.target, d.mode
		d.open = false
		v.Run(opReset, v.text("ref_reset_failed"), func(git Git, path string) error {
			return git.Reset(path, target.rev(), mode)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "reset-dialog",
		title: func() string {
			if v.tracking.Branch == "" {
				return v.text("ref_reset_head_title")
			}
			return v.text("ref_reset_title")
		},
		description: func() string {
			if v.tracking.Branch == "" {
				return v.text("ref_reset_head_description")
			}
			return v.textf("ref_reset_description", map[string]any{"Name": d.target.name, "Current": v.checkedOutName()})
		},
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			return []core.Node{
				formField(cx, refLabelWidth, v.text("ref_reset_source"), nil, v.checkedOutChoice(cx)),
				formField(cx, refLabelWidth, v.text("ref_reset_destination"), nil, v.targetChoice(cx, d.target)),
				formField(cx, refLabelWidth, v.text("ref_reset_mode"), nil, radios(v, &d.mode, v.text("ref_reset_mode"), []radioChoice[ResetMode]{
					{ResetSoft, v.text("ref_reset_soft")}, {ResetMixed, v.text("ref_reset_mixed")}, {ResetHard, v.text("ref_reset_hard")},
				}, nil)),
				core.Show{When: func() bool { v.watch(); return d.mode == ResetHard }, Then: func(cx *core.Scope) core.Node {
					return dialogNote(v.text("ref_reset_hard_hint"), 13, colors.Warn)
				}},
			}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("ref_reset_confirm"), v.CanSync, run, "confirm")}
		},
		confirm: func() {
			if v.CanSync() {
				run()
			}
		}})
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
func (v *View) cherryPickDialogView(cx *core.Scope) core.Node {
	d := &v.cherryPick
	run := func() {
		commit, parent := d.commit, d.parent
		o := CherryPickOptions{Commit: d.commitChanges, AppendOrigin: d.appendOrigin && d.commitChanges, Parent: parent}
		d.open = false
		v.state.CherryPickAppend = o.AppendOrigin
		v.saveState()
		v.Run(opCherryPick, v.text("ref_cherry_pick_failed"), func(git Git, path string) error {
			return git.CherryPick(path, commit.Hash, o)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "cherry-pick-dialog",
		title:       func() string { return v.text("ref_cherry_pick_title") },
		description: func() string { return v.text("ref_cherry_pick_description") },
		body: func(cx *core.Scope) []core.Node {
			return []core.Node{
				formField(cx, refLabelWidth, v.text("ref_cherry_pick_commit"), nil, v.targetChoice(cx, commitTarget(d.commit))),
				v.parentField(cx, d.commit, &d.parent),
				v.formOption(refLabelWidth, &d.commitChanges, v.text("ref_commit_changes"), v.invalidate),
				core.Show{When: func() bool { v.watch(); return d.commitChanges }, Then: func(cx *core.Scope) core.Node {
					return v.formOption(refLabelWidth, &d.appendOrigin, v.text("ref_append_origin"), nil)
				}},
			}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("ref_cherry_pick_confirm"), v.CanSync, run, "confirm")}
		},
		confirm: func() {
			if v.CanSync() {
				run()
			}
		}})
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
func (v *View) revertDialogView(cx *core.Scope) core.Node {
	d := &v.revert
	run := func() {
		commit, parent := d.commit, d.parent
		o := RevertOptions{Commit: d.commitChange, Parent: parent}
		d.open = false
		v.Run(opRevert, v.text("ref_revert_failed"), func(git Git, path string) error {
			return git.Revert(path, commit.Hash, o)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "revert-dialog",
		title:       func() string { return v.text("ref_revert_title") },
		description: func() string { return v.text("ref_revert_description") },
		body: func(cx *core.Scope) []core.Node {
			return []core.Node{
				formField(cx, refLabelWidth, v.text("ref_revert_commit"), nil, v.targetChoice(cx, commitTarget(d.commit))),
				v.parentField(cx, d.commit, &d.parent),
				v.formOption(refLabelWidth, &d.commitChange, v.text("ref_commit_changes"), nil),
			}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("ref_revert_confirm"), v.CanSync, run, "confirm")}
		},
		confirm: func() {
			if v.CanSync() {
				run()
			}
		}})
}

// parentField chooses the parent of a merge commit to act relative to, or
// is nil for another commit.
func (v *View) parentField(cx *core.Scope, commit Commit, parent *int) core.Node {
	if len(commit.Parents) < 2 {
		return nil
	}
	*parent = min(max(*parent, 1), len(commit.Parents))
	var options []ui.Option[int]
	for i, hash := range commit.Parents {
		options = append(options, ui.Option[int]{Value: i + 1, Label: v.commitLabel(hash)})
	}
	return formField(cx, syncLabelWidth, v.text("ref_parent"), nil, selectField(v, parent, v.text("ref_parent"), options, nil, "parent"))
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
func (v *View) pushTagDialogView(cx *core.Scope) core.Node {
	d := &v.pushTag
	ready := func() bool { return v.CanSync() && d.remote != "" }
	run := func() {
		tag, remote := d.tag, d.remote
		d.open = false
		v.syncRepository(opPushTag, v.text("ref_push_tag_failed"), func(git Git, path string) error {
			return git.PushTag(path, remote, tag)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 400, testID: "push-tag-dialog",
		title:       func() string { return v.text("ref_push_tag_title") },
		description: func() string { return v.text("ref_push_tag_description") },
		body: func(cx *core.Scope) []core.Node {
			remotes := v.remotes()
			var options []ui.Option[string]
			for _, remote := range remotes {
				options = append(options, ui.Option[string]{Value: remote, Label: remote, Leading: ui.Icon(remoteIcon(v.repo.github, remote), 14, ui.Inherited)})
			}
			nodes := []core.Node{formField(cx, refLabelWidth, v.text("ref_push_tag_tag"), nil, v.targetChoice(cx, refTarget{group: "tags", name: d.tag}))}
			if len(remotes) == 0 {
				nodes = append(nodes, muted(cx, v.text("no_remotes"), 13))
			}
			disabled := func() bool { return len(remotes) == 0 }
			return append(nodes, formField(cx, refLabelWidth, v.text("sync_remote"), disabled, selectField(v, &d.remote, v.text("sync_remote"), options, nil, "push-tag-remote")))
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("push"), ready, run, "confirm")}
		},
		confirm: func() {
			if ready() {
				run()
			}
		}})
}

// checkedOutChoice names the branch the worktree has checked out, or HEAD.
func (v *View) checkedOutChoice(cx *core.Scope) core.Node {
	icon := lucide.GitBranch
	if v.tracking.Branch == "" {
		icon = lucide.CircleDot
	}
	return choice(cx, icon, v.checkedOutName())
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
			v.dispatch(func() { v.toast(failed, err) })
		}
	}()
}

// saveFile asks where to write data and writes it there, unless a host
// provides SaveFile. It runs off the engine and waits for the user.
func (v *View) saveFile(title, name string, data []byte) (string, error) {
	if v.opts.SaveFile != nil {
		return v.opts.SaveFile(title, name, data)
	}
	type saved struct {
		path string
		err  error
	}
	done := make(chan saved, 1)
	v.dispatch(func() {
		if v.cx == nil {
			done <- saved{}
			return
		}
		v.cx.SaveFile(core.SaveOptions{Name: name, Data: data}, func(ok bool, path string, err error) {
			if !ok {
				path = ""
			}
			done <- saved{path, err}
		})
	})
	s := <-done
	return s.path, s.err
}

// remotes lists the repository's remotes by name.
func (v *View) remotes() []string {
	return slices.Sorted(maps.Keys(v.repo.github))
}
