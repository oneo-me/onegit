package gitview

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// stashDialog takes the message of a new stash.
type stashDialog struct {
	open    bool
	message string
}

// OpenStash opens the dialog saving the worktree's changes to a new stash.
func (v *View) OpenStash() {
	if v.CanStash() {
		v.stashDialog = stashDialog{open: true}
		v.invalidate()
	}
}

// CanStash reports whether a stash may start: CanSync holds and the
// worktree's changes, when read, are not empty.
func (v *View) CanStash() bool {
	return v.CanSync() && (!v.current.loaded || v.hasCurrentChanges())
}

// stashDialogView is the dialog saving a stash with an optional message,
// untracked files too unless the user leaves them out.
func (v *View) stashDialogView(c *ui.Context) {
	d := &v.stashDialog
	title := v.text("stash_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text("stash_description")).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		ready := v.CanStash()
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
		untracked := !v.state.StashTrackedOnly
		ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(0, 0, 0, t.Space(syncLabelWidth+3)).Children(func() {
			components.Checkbox(c, &untracked, v.text("stash_untracked"))
			components.Text(c, v.text("stash_untracked_hint")).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		if untracked == v.state.StashTrackedOnly {
			v.state.StashTrackedOnly = !untracked
			v.saveState()
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
	v.syncRepository(OpStash, v.text("stash_failed"), func(git Git, path string) error {
		return git.SaveStash(path, message, untracked)
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

// stashRows lists stashes by message; choosing one shows its files.
func (v *View) stashRows(c *ui.Context, stashes []Stash) {
	for _, s := range stashes {
		ui.Column(c).Key("stash-" + s.Hash).FillWidth().Shrink(0).Children(func() {
			row := components.TreeItem(c, s.Message, nil, v.section == sectionStash && v.stash == s.Hash, nil).Height(treeRow(c))
			components.Tooltip(c, row, s.Name+"  "+s.Date.Local().Format("2006-01-02 15:04"))
			if row.Clicked() {
				v.showStash(s.Hash)
			}
		})
	}
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
