package gitview

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
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
func (v *View) stashDialogView(c *ui.Context) {
	d := &v.stashDialog
	title := v.text("stash_title")
	description := v.text("stash_description")
	if d.partial {
		args := map[string]any{"Count": len(d.files)}
		title = v.textf("stash_files_title", args)
		description = v.text("stash_files_description")
	}
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, description).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		ready := v.CanStash()
		if d.partial {
			ready = v.CanSync() && len(d.files) > 0
		}
		syncField(c, v.text("stash_message"), false, func() {
			box := components.TextBox(c, &d.message, components.TextBoxOptions{LeadingIcon: lucide.Archive()}).
				FillWidth().Label(v.text("stash_message")).Placeholder(v.text("stash_placeholder")).AutoFocus()
			box.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods != 0 {
					return false
				}
				if ready {
					d.message = k.Text()
					v.saveStash()
				}
				return true
			})
		})
		if !d.partial {
			untracked := !v.state.StashTrackedOnly
			ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(0, 0, 0, t.Space(syncLabelWidth+3)).Children(func() {
				components.Checkbox(c, &untracked, v.text("stash_untracked"))
				components.Text(c, v.text("stash_untracked_hint")).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted)
			})
			if untracked == v.state.StashTrackedOnly {
				v.state.StashTrackedOnly = !untracked
				v.saveState()
			}
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("stash_save"), components.ButtonPrimary).Disabled(!ready).Clicked() {
				v.saveStash()
			}
		})
	})
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
func (v *View) stashRenameDialogView(c *ui.Context) {
	d := &v.stashRename
	title := v.text("stash_rename_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text("stash_rename_description")).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		syncField(c, v.text("stash_message"), false, func() {
			box := components.TextBox(c, &d.message, components.TextBoxOptions{LeadingIcon: lucide.Archive()}).
				FillWidth().Label(v.text("stash_message")).Placeholder(v.text("stash_rename_placeholder")).AutoFocus()
			box.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods != 0 {
					return false
				}
				d.message = k.Text()
				v.renameStash()
				return true
			})
		})
		message := strings.TrimSpace(d.message)
		ready := message != "" && message != d.stash.Message && v.CanSync()
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			b := components.Button(c, v.text("stash_rename_confirm"), components.ButtonPrimary).Disabled(!ready)
			if b.Clicked() && ready {
				d.message = message
				v.renameStash()
			}
		})
	})
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
func (v *View) stashDeletionDialogView(c *ui.Context) {
	d := &v.stashDeletion
	title := v.text("stash_delete_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Role(ui.RoleAlertDialog)
		components.Text(c, title).Bold().Wrap().FillWidth()
		components.Text(c, v.text("stash_delete_message")).Wrap().FillWidth().TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).AutoFocus().Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("delete"), components.ButtonPrimary).Disabled(!v.CanSync()).Clicked() {
				d.open = false
				hash := d.stash.Hash
				v.syncRepository(opDropStash, v.text("stash_delete_failed"), func(git Git, path string) error {
					return git.DropStash(path, hash)
				})
			}
		})
	})
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
func (v *View) applyStashDialog(c *ui.Context) {
	d := &v.applyStash
	title := v.text("stash_apply_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text("stash_apply_description")).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		syncField(c, v.text("stash_apply_stash"), false, func() {
			ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				components.Icon(c, lucide.Archive).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
				components.Text(c, d.stash.Name).Shrink(0).TextColor(t.TextMuted).SingleLine()
				components.Text(c, d.stash.Message).MinWidth(0).SingleLine().Ellipsis("…")
			})
		})
		drop := !v.state.StashKeepApplied
		ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(0, 0, 0, t.Space(syncLabelWidth+3)).Children(func() {
			components.Checkbox(c, &drop, v.text("stash_apply_drop"))
			components.Text(c, v.text("stash_apply_drop_hint")).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		if drop == v.state.StashKeepApplied {
			v.state.StashKeepApplied = !drop
			v.saveState()
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			apply := components.Button(c, v.text("stash_apply"), components.ButtonPrimary).Disabled(!v.CanSync()).AutoFocus()
			if apply.Clicked() {
				d.open = false
				hash, drop := d.stash.Hash, !v.state.StashKeepApplied
				v.syncRepository(opApplyStash, v.text("stash_apply_failed"), func(git Git, path string) error {
					return git.ApplyStash(path, hash, drop)
				})
			}
		})
	})
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
func (v *View) stashRows(c *ui.Context, stashes []Stash) {
	for _, s := range stashes {
		ui.Column(c).Key("stash-" + s.Hash).FillWidth().Shrink(0).Children(func() {
			row := components.TreeItem(c, s.Message, nil, v.section == sectionStash && v.stash == s.Hash, nil).Height(treeRow(c))
			components.Tooltip(c, row, s.Name+"  "+s.Date.Local().Format("2006-01-02 15:04"))
			row.ContextMenu(func(m *ui.Menu) {
				if m.Item(v.text("stash_apply_menu")).Disabled(!v.CanSync()).Chosen() {
					v.openApplyStash(s)
				}
				if m.Item(v.text("stash_rename")).Disabled(!v.CanSync()).Chosen() {
					v.openRenameStash(s)
				}
				if m.Item(v.text("ref_save_patch")).Disabled(!v.CanSync()).Chosen() {
					v.saveStashPatch(s)
				}
				m.Separator()
				if m.Item(v.text("delete_ref")).Disabled(!v.CanSync()).Chosen() {
					v.openDeleteStash(s)
				}
			})
			if row.Clicked() {
				v.showStash(s.Hash)
			}
			if row.DoubleClicked() {
				v.openApplyStash(s)
			}
		})
	}
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
			v.dispatch(func() {
				v.toast = &ui.Toast{Title: failed, Description: err.Error(), Type: components.ToastError}
				v.invalidate()
			})
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
