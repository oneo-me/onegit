package gitview

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
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
func (v *View) rebaseDialogView(cx *core.Scope) core.Node {
	d := &v.rebase
	args := func() map[string]any { return map[string]any{"Name": d.target.name, "Current": v.checkedOutName()} }
	ready := func() bool { return v.CanSync() && !d.loading }
	remember := func() {
		v.state.RebaseAutostash, v.state.RebaseUpdateRefs = d.autostash, d.updateRefs
		v.saveState()
	}
	run := func() {
		target := d.target
		o := IntegrateOptions{Stash: d.autostash, UpdateRefs: d.updateRefs && len(d.branches) > 0}
		d.open = false
		v.Run(opRebase, v.text("ref_rebase_failed"), func(git Git, path string) error {
			return git.Rebase(path, target.rev(), o)
		})
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "rebase-dialog",
		title:       func() string { return v.textf("rebase_title", args()) },
		description: func() string { return v.textf("rebase_description", args()) },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			branches := region(cx, func() string { v.track(); return strings.Join(d.branches, "\n") }, func(cx *core.Scope) core.Node {
				if len(d.branches) == 0 {
					return nil
				}
				var names []core.Node
				for _, branch := range d.branches {
					names = append(names, branchChoice(cx, branch))
				}
				list := core.Show{When: func() bool { v.watch(); return d.updateRefs }, Then: func(cx *core.Scope) core.Node {
					return ui.ScrollView{Style: core.Style{MaxHeight: core.Pt(96)}, ContentStyle: core.Style{Gap: 4, Padding: core.Edges{Left: refLabelWidth + 12}}, Children: names}
				}}
				return column(8, v.formOption(refLabelWidth, &d.updateRefs, v.text("rebase_update_refs"), remember), list)
			})
			status := region(cx, func() string { v.track(); return fmt.Sprint(d.loading, d.err, d.check) }, func(cx *core.Scope) core.Node {
				switch {
				case d.loading:
					return busyNote(cx, v.text("rebase_checking"))
				case d.err != "":
					return muted(cx, d.err, 12)
				case d.check == RebaseCheckClean:
					return dialogNote(v.text("rebase_clean"), 12, colors.Up)
				case d.check == RebaseCheckConflict:
					return dialogNote(v.text("rebase_conflict"), 12, colors.Warn)
				}
				return nil
			})
			return []core.Node{
				formField(cx, refLabelWidth, v.text("rebase_source"), nil, v.checkedOutChoice(cx)),
				formField(cx, refLabelWidth, v.text("rebase_destination"), nil, v.targetChoice(cx, d.target)),
				branches,
				v.formOption(refLabelWidth, &d.autostash, v.text("rebase_autostash"), remember),
				status,
			}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("rebase_confirm"), ready, run, "confirm")}
		},
		confirm: func() {
			if ready() {
				run()
			}
		}})
}
