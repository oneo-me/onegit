package gitview

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// bulkKind is the write a bulk ref dialog runs.
type bulkKind int

const (
	bulkDeleteBranches bulkKind = iota
	bulkPushBranches
	bulkDeleteTags
	bulkPushTags
)

// bulkRef is one row of a bulk ref dialog.
type bulkRef struct {
	Name string
	On   bool
}

// bulkDialog chooses several refs for one write. Deleting branches marks
// the unmerged ones, which force deletes anyway; checking holds reading
// them and generation discards a read the user replaced.
type bulkDialog struct {
	open       bool
	kind       bulkKind
	refs       []bulkRef
	remote     string
	generation uint64
	checking   bool
	unmerged   map[string]bool
	force      bool
	err        string
}

// openBulk fills the dialog with the refs its kind acts on. Deleting local
// branches leaves out the branches worktrees have checked out and reads the
// unmerged ones in the background.
func (v *View) openBulk(kind bulkKind) {
	if !v.CanSync() {
		return
	}
	d := &v.bulk
	*d = bulkDialog{open: true, kind: kind, generation: d.generation + 1, checking: kind == bulkDeleteBranches}
	switch kind {
	case bulkDeleteBranches, bulkPushBranches:
		for _, n := range refLeaves(v.repo.refTrees.branches) {
			if kind == bulkDeleteBranches && v.checkedOut(n.path) {
				continue
			}
			d.refs = append(d.refs, bulkRef{Name: n.path})
		}
	case bulkDeleteTags, bulkPushTags:
		for _, n := range refLeaves(v.repo.refTrees.tags) {
			d.refs = append(d.refs, bulkRef{Name: n.path})
		}
	}
	d.remote = v.defaultRemote()
	v.invalidate()
	if !d.checking {
		return
	}
	generation, path, git := d.generation, v.path, v.git
	go func() {
		unmerged, err := git.UnmergedBranches(path)
		v.dispatch(func() {
			current := &v.bulk
			if current.generation != generation || !current.open || v.closed {
				return
			}
			current.checking, current.unmerged = false, unmerged
			if err != nil {
				current.err = err.Error()
			}
			v.invalidate()
		})
	}()
}

// checkedUnmerged reports whether a chosen branch is unmerged.
func (d *bulkDialog) checkedUnmerged() bool {
	return slices.ContainsFunc(d.refs, func(ref bulkRef) bool { return ref.On && d.unmerged[ref.Name] })
}

// checked returns the names chosen in the dialog.
func (d *bulkDialog) checked() []string {
	var names []string
	for _, ref := range d.refs {
		if ref.On {
			names = append(names, ref.Name)
		}
	}
	return names
}

// bulkTitle names the dialog's write.
func (v *View) bulkTitle(kind bulkKind) string {
	switch kind {
	case bulkDeleteBranches:
		return v.text("bulk_delete_branches")
	case bulkPushBranches:
		return v.text("bulk_push_branches")
	case bulkDeleteTags:
		return v.text("bulk_delete_tags")
	}
	return v.text("bulk_push_tags")
}

// bulkDialogView shows check boxes for the refs and runs the chosen write.
func (v *View) bulkDialogView(c *ui.Context) {
	d := &v.bulk
	if !d.open {
		return
	}
	title := v.bulkTitle(d.kind)
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		w, h := c.Size()
		panel.Width(min(t.Space(110), max(0, w-2*v.dialogInset(c)))).Height(min(t.Space(130), h*.75)).Padding(t.Space(3))
		ui.Column(c).FillWidth().FillHeight().MinHeight(0).Gap(t.Space(2)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text("bulk_description")).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted)
			push := d.kind == bulkPushBranches || d.kind == bulkPushTags
			if push {
				remotes := slices.Sorted(maps.Keys(v.repo.remoteURLs))
				if len(remotes) == 0 {
					components.Text(c, v.text("no_remotes")).Wrap().FillWidth().TextColor(t.TextMuted)
				} else {
					formField(c, 16, v.text("bulk_remote"), false, func() {
						sel := components.Select(c, &d.remote)
						sel.Trigger.Label(v.text("bulk_remote")).Children(func() { components.Text(c, d.remote).SingleLine() })
						sel.Popup(func(panel ui.Element) {
							syncChoices(c, panel, func() {
								for _, remote := range remotes {
									sel.Item(remote).Label(remote).Children(func() { v.remoteChoice(c, remote) })
								}
							})
						})
					})
				}
			}
			ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
				if components.Button(c, v.text("bulk_all"), components.ButtonGhost).Height(24).FontSize(12).Clicked() {
					for i := range d.refs {
						d.refs[i].On = true
					}
				}
				if components.Button(c, v.text("bulk_none"), components.ButtonGhost).Height(24).FontSize(12).Clicked() {
					for i := range d.refs {
						d.refs[i].On = false
					}
				}
			})
			if len(d.refs) == 0 {
				components.Text(c, v.text("bulk_empty")).Wrap().FillWidth().TextColor(t.TextMuted)
			} else {
				components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Shrink(0).Gap(t.Space(1)).Children(func() {
						for i := range d.refs {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								components.Checkbox(c, &d.refs[i].On, d.refs[i].Name)
								if d.unmerged[d.refs[i].Name] {
									components.Badge(c, v.text("bulk_unmerged"), components.BadgeOptions{Tone: components.BadgeWarning})
								}
							})
						}
					})
				})
			}
			switch {
			case d.checking:
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
					components.Spinner(c, components.SpinnerRing).Size(14, 14)
					components.Text(c, v.text("delete_branches_checking")).FontSize(12).TextColor(t.TextMuted)
				})
			case d.err != "":
				components.Text(c, d.err).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted)
			case d.checkedUnmerged():
				components.Text(c, v.text("bulk_unmerged_message")).FontSize(12).Wrap().FillWidth().TextColor(t.Warning)
				components.Switch(c, &d.force, v.text("bulk_force"))
			}
			ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
					d.open = false
				}
				ready := len(d.checked()) > 0 && v.CanSync() && (!push || d.remote != "") && !d.checking && (d.force || !d.checkedUnmerged())
				confirm := v.text("bulk_delete")
				if push {
					confirm = v.text("push")
				}
				if components.Button(c, confirm, components.ButtonPrimary).Disabled(!ready).Clicked() {
					d.open = false
					v.runBulk()
				}
			})
		})
	})
}

// runBulk runs the dialog's write for each chosen ref, combining their
// failures, and forces only the unmerged branches the user chose to lose.
func (v *View) runBulk() {
	d := v.bulk
	names := d.checked()
	if len(names) == 0 {
		return
	}
	op, failed := opDelete, v.text("delete_failed")
	if d.kind == bulkPushBranches || d.kind == bulkPushTags {
		op, failed = OpPush, v.text("push_failed")
	}
	v.syncRepository(op, failed, func(git Git, path string) error {
		var errs []error
		for _, name := range names {
			var err error
			switch d.kind {
			case bulkDeleteBranches:
				err = git.DeleteBranch(path, name, d.force && d.unmerged[name])
			case bulkDeleteTags:
				err = git.DeleteTag(path, name)
			case bulkPushBranches:
				err = git.PushBranch(path, PushOptions{Branch: name, Remote: d.remote, To: name})
			case bulkPushTags:
				err = git.PushTag(path, d.remote, name)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
		return errors.Join(errs...)
	})
}
