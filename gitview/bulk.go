package gitview

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
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
func (v *View) bulkDialogView(cx *core.Scope) core.Node {
	d := &v.bulk
	push := func() bool { return d.kind == bulkPushBranches || d.kind == bulkPushTags }
	ready := func() bool {
		return len(d.checked()) > 0 && v.CanSync() && (!push() || d.remote != "") && !d.checking && (d.force || !d.checkedUnmerged())
	}
	run := func() {
		d.open = false
		v.runBulk()
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, height: 520, testID: "bulk-dialog",
		title:       func() string { return v.bulkTitle(d.kind) },
		description: func() string { return v.text("bulk_description") },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			var nodes []core.Node
			if push() {
				remotes := slices.Sorted(maps.Keys(v.repo.remoteURLs))
				if len(remotes) == 0 {
					nodes = append(nodes, muted(cx, v.text("no_remotes"), 13))
				} else {
					var options []ui.Option[string]
					for _, remote := range remotes {
						options = append(options, ui.Option[string]{Value: remote, Label: remote, Leading: ui.Icon(remoteIcon(v.repo.github, remote), 14, ui.Inherited)})
					}
					nodes = append(nodes, formField(cx, syncLabelWidth, v.text("bulk_remote"), nil, selectField(v, &d.remote, v.text("bulk_remote"), options, nil, "bulk-remote")))
				}
			}
			all := func(on bool) func() {
				return func() {
					for i := range d.refs {
						d.refs[i].On = on
					}
					v.edited()
				}
			}
			nodes = append(nodes, row(8,
				ui.Button{Text: v.text("bulk_all"), Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "bulk-all", OnPress: all(true)},
				ui.Button{Text: v.text("bulk_none"), Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "bulk-none", OnPress: all(false)}))
			if len(d.refs) == 0 {
				nodes = append(nodes, muted(cx, v.text("bulk_empty"), 13))
			} else {
				refs := region(cx, func() string { v.track(); return fmt.Sprint(d.unmerged) }, func(cx *core.Scope) core.Node {
					var boxes []core.Node
					for i := range d.refs {
						ref := &d.refs[i]
						box := []core.Node{v.checkbox(&ref.On, ref.Name, nil)}
						if d.unmerged[ref.Name] {
							box = append(box, ui.Badge{Text: v.text("bulk_unmerged"), Variant: ui.BadgeWarn})
						}
						boxes = append(boxes, row(8, box...))
					}
					return ui.ScrollView{Style: ui.FillRest, ContentStyle: core.Style{Gap: 4}, Children: boxes}
				})
				nodes = append(nodes, refs)
			}
			status := region(cx, func() string { v.watch(); return fmt.Sprint(d.checking, d.err, d.checkedUnmerged()) }, func(cx *core.Scope) core.Node {
				switch {
				case d.checking:
					return busyNote(cx, v.text("delete_branches_checking"))
				case d.err != "":
					return muted(cx, d.err, 12)
				case d.checkedUnmerged():
					return column(8, dialogNote(v.text("bulk_unmerged_message"), 12, colors.Warn), v.toggle(cx, &d.force, v.text("bulk_force"), nil))
				}
				return nil
			})
			return append(nodes, status)
		},
		actions: func(cx *core.Scope) []core.Node {
			label := v.text("bulk_delete")
			if push() {
				label = v.text("push")
			}
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(label, ready, run, "confirm")}
		}})
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
