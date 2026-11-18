package gitview

import (
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// stashDialog takes the message of a new stash, or of a stash of the chosen
// files when partial is set.
type stashDialog struct {
	open    bool
	message string
	files   []FileChange
	partial bool
}

// OpenStash opens the dialog saving the worktree's changes to a new stash.
func (v *View) OpenStash() {
	if v.CanStash() {
		v.stashDialog = stashDialog{open: true}
		v.invalidate()
	}
}

// openStashFiles opens the dialog saving the changes to files to a new
// stash.
func (v *View) openStashFiles(files []FileChange) {
	if v.CanSync() && len(files) > 0 {
		v.stashDialog = stashDialog{open: true, files: files, partial: true}
		v.invalidate()
	}
}

// CanStash reports whether a stash may start: CanSync holds and the
// worktree's changes, when read, are not empty.
func (v *View) CanStash() bool {
	return v.CanSync() && (!v.current.loaded || v.hasCurrentChanges())
}

// stashDialogView is the dialog saving a stash with an optional message,
// untracked files too unless the user leaves them out, or the chosen files.
func (v *View) stashDialogView(cx *core.Scope) core.Node {
	d := &v.stashDialog
	ready := func() bool {
		if d.partial {
			return v.CanSync() && len(d.files) > 0
		}
		return v.CanStash()
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "stash-dialog",
		title: func() string {
			if d.partial {
				return v.textf("stash_files_title", map[string]any{"Count": len(d.files)})
			}
			return v.text("stash_title")
		},
		description: func() string {
			if d.partial {
				return v.text("stash_files_description")
			}
			return v.text("stash_description")
		},
		body: func(cx *core.Scope) []core.Node {
			nodes := []core.Node{formField(cx, syncLabelWidth, v.text("stash_message"), nil,
				v.textField(&d.message, v.text("stash_message"), v.text("stash_placeholder"), true, func() {
					if ready() {
						v.saveStash()
					}
				}, "stash-message"))}
			if !d.partial {
				untracked := !v.state.StashTrackedOnly
				nodes = append(nodes, core.View{Style: core.Style{Gap: 4, Padding: core.Edges{Left: syncLabelWidth + 12}}, Children: []core.Node{
					v.checkbox(&untracked, v.text("stash_untracked"), func() {
						v.state.StashTrackedOnly = !untracked
						v.saveState()
					}),
					muted(cx, v.text("stash_untracked_hint"), 12),
				}})
			}
			return nodes
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("stash_save"), ready, v.saveStash, "confirm")}
		}})
}

// saveStash closes the dialog and stashes the worktree's changes as it
// chose.
func (v *View) saveStash() {
	d := &v.stashDialog
	d.open = false
	message, untracked := strings.TrimSpace(d.message), !v.state.StashTrackedOnly
	if d.partial {
		files := d.files
		v.syncRepository(OpStash, v.text("stash_failed"), func(git Git, path string) error {
			return git.SaveStashFiles(path, message, files)
		})
		return
	}
	v.syncRepository(OpStash, v.text("stash_failed"), func(git Git, path string) error {
		return git.SaveStash(path, message, untracked)
	})
}

// opApplyStash is the Git write applying a stash.
const opApplyStash = "apply-stash"

// Git writes changing the stash list.
const (
	opRenameStash = "rename-stash"
	opDropStash   = "drop-stash"
)

// stashRenameDialog takes a stash's new message.
type stashRenameDialog struct {
	open    bool
	stash   Stash
	message string
}

// stashDeletionDialog confirms dropping a stash.
type stashDeletionDialog struct {
	open  bool
	stash Stash
}

// openRenameStash asks for a new message for s.
func (v *View) openRenameStash(s Stash) {
	if v.CanSync() {
		v.stashRename = stashRenameDialog{open: true, stash: s, message: s.Message}
		v.invalidate()
	}
}

// openDeleteStash asks before dropping s.
func (v *View) openDeleteStash(s Stash) {
	if v.CanSync() {
		v.stashDeletion = stashDeletionDialog{open: true, stash: s}
		v.invalidate()
	}
}

// stashRenameDialogView renames a stash with the message typed.
func (v *View) stashRenameDialogView(cx *core.Scope) core.Node {
	d := &v.stashRename
	ready := func() bool {
		message := strings.TrimSpace(d.message)
		return message != "" && message != d.stash.Message && v.CanSync()
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "stash-rename-dialog",
		title:       func() string { return v.text("stash_rename_title") },
		description: func() string { return v.text("stash_rename_description") },
		body: func(cx *core.Scope) []core.Node {
			return []core.Node{formField(cx, syncLabelWidth, v.text("stash_message"), nil,
				v.textField(&d.message, v.text("stash_message"), v.text("stash_rename_placeholder"), true, v.renameStash, "stash-rename-message"))}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("stash_rename_confirm"), ready, v.renameStash, "confirm")}
		}})
}

// renameStash closes the dialog and stores the stash under its new message.
func (v *View) renameStash() {
	d := &v.stashRename
	message := strings.TrimSpace(d.message)
	if message == "" || message == d.stash.Message {
		return
	}
	d.open = false
	hash := d.stash.Hash
	var renamed string
	v.write(opRenameStash, v.text("stash_rename_failed"), func(git Git, path string) error {
		var err error
		renamed, err = git.RenameStash(path, hash, message)
		return err
	}, func() {
		if v.stash == hash {
			v.stash = renamed
		}
	})
}

// stashDeletionDialogView drops a stash after confirmation.
func (v *View) stashDeletionDialogView(cx *core.Scope) core.Node {
	d := &v.stashDeletion
	run := func() {
		d.open = false
		hash := d.stash.Hash
		v.syncRepository(opDropStash, v.text("stash_delete_failed"), func(git Git, path string) error {
			return git.DropStash(path, hash)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, alert: true, testID: "stash-delete-dialog",
		title:       func() string { return v.text("stash_delete_title") },
		description: func() string { return v.text("stash_delete_message") },
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("delete"), v.CanSync, run, "confirm")}
		}})
}

// applyStash is the dialog applying stash to the worktree.
type applyStash struct {
	open  bool
	stash Stash
}

// openApplyStash asks before applying s, unless a write may not start.
func (v *View) openApplyStash(s Stash) {
	if v.CanSync() {
		v.applyStash = applyStash{open: true, stash: s}
		v.invalidate()
	}
}

// applyStashDialog applies a stash to the worktree, then deletes it unless
// the user keeps it.
func (v *View) applyStashDialog(cx *core.Scope) core.Node {
	d := &v.applyStash
	run := func() {
		d.open = false
		hash, drop := d.stash.Hash, !v.state.StashKeepApplied
		v.syncRepository(opApplyStash, v.text("stash_apply_failed"), func(git Git, path string) error {
			return git.ApplyStash(path, hash, drop)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "apply-stash-dialog",
		title:       func() string { return v.text("stash_apply_title") },
		description: func() string { return v.text("stash_apply_description") },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			name := line(d.stash.Name, 13, ui.Regular, colors.Muted)
			name.Style.FlexShrink = 0
			drop := !v.state.StashKeepApplied
			return []core.Node{
				formField(cx, syncLabelWidth, v.text("stash_apply_stash"), nil,
					ui.Icon(lucide.Archive, 14, colors.Muted), name, line(d.stash.Message, 13, ui.Regular, colors.Text)),
				core.View{Style: core.Style{Gap: 4, Padding: core.Edges{Left: syncLabelWidth + 12}}, Children: []core.Node{
					v.checkbox(&drop, v.text("stash_apply_drop"), func() {
						v.state.StashKeepApplied = !drop
						v.saveState()
					}),
					muted(cx, v.text("stash_apply_drop_hint"), 12),
				}},
			}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("stash_apply"), v.CanSync, run, "confirm")}
		},
		confirm: func() {
			if v.CanSync() {
				run()
			}
		}})
}

// filterStashes keeps the stashes whose message contains query, ignoring
// case. An empty query keeps every stash.
func filterStashes(stashes []Stash, query string) []Stash {
	query = strings.ToLower(query)
	if query == "" {
		return stashes
	}
	var kept []Stash
	for _, s := range stashes {
		if strings.Contains(strings.ToLower(s.Message), query) {
			kept = append(kept, s)
		}
	}
	return kept
}

// stashRows lists stashes by message; choosing one shows its files, its menu
// applies, renames or deletes it, and a double click offers to apply it.
func (v *View) stashRows(cx *core.Scope, stashes []Stash) []core.Node {
	var rows []core.Node
	for _, s := range stashes {
		rows = append(rows, v.treeItem(cx, s.Message, s.Name+"  "+s.Date.Local().Format("2006-01-02 15:04"), 0,
			func() bool { return v.section == sectionStash && v.stash == s.Hash }, nil,
			func() { v.showStash(s.Hash) },
			func() { v.openApplyStash(s) },
			func() []core.MenuItem {
				g := &menuGroups{}
				can := v.CanSync()
				g.item(v.text("stash_apply_menu"), !can, func() { v.openApplyStash(s) })
				g.item(v.text("stash_rename"), !can, func() { v.openRenameStash(s) })
				g.item(v.text("ref_save_patch"), !can, func() { v.saveStashPatch(s) })
				g.group()
				g.item(v.text("delete_ref"), !can, func() { v.openDeleteStash(s) })
				return g.items
			}, "stash-"+s.Hash))
	}
	return rows
}

// saveStashPatch writes the changes a stash saved to a file the user
// chooses.
func (v *View) saveStashPatch(s Stash) {
	path, git := v.path, v.git
	title, failed := v.text("ref_save_patch_title"), v.text("ref_save_patch_failed")
	go func() {
		data, err := git.StashPatch(path, s.Hash)
		if err == nil {
			_, err = v.saveFile(title, "stash-"+s.Hash[:min(10, len(s.Hash))]+".patch", data)
		}
		if err != nil {
			v.dispatch(func() { v.toast(failed, err) })
		}
	}()
}

// showStash shows the files the stash hash saved.
func (v *View) showStash(hash string) {
	v.section, v.ref, v.seekChanges, v.stash = sectionStash, "", false, hash
	path := v.path
	v.loadFiles(v.stashFiles, hash, func(git Git) (Changes, error) { return git.StashChanges(path, hash) })
	v.targetChanged()
}

// clearStash forgets the chosen stash and its files.
func (v *View) clearStash() {
	v.stash = ""
	v.clearFiles(v.stashFiles, "")
	v.stashFiles.key = ""
}

// keepStash leaves a stash that is gone, such as after it was applied
// elsewhere, for the history.
func (v *View) keepStash() {
	if v.stash == "" {
		return
	}
	for _, s := range v.repo.stashes {
		if s.Hash == v.stash {
			return
		}
	}
	v.clearStash()
	if v.section == sectionStash {
		v.section = sectionCommits
		v.targetChanged()
	}
}
