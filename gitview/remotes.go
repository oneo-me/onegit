package gitview

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// opRemote is the Git write adding, renaming or deleting a remote.
const opRemote = "remote"

// staleAfterDays is how old a local branch's last commit must be for the
// stale branches dialog to offer it for deletion.
const staleAfterDays = 90

// headTarget is the ref a branch or tag created from its group starts at:
// the checked out branch when shown, or the checked out commit.
func (v *View) headTarget() (refTarget, bool) {
	if v.repo.head == "" {
		return refTarget{}, false
	}
	for _, n := range refLeaves(v.repo.refTrees.branches) {
		if n.path == v.tracking.Branch {
			return nodeTarget("branches", n), true
		}
	}
	name := v.repo.head
	if len(name) > 7 {
		name = name[:7]
	}
	return refTarget{group: "commit", name: name, hash: v.repo.head}, true
}

// branchGroupMenu is the Branches heading's menu: a new branch at the
// checked out commit, and the stale local branches to delete.
func (v *View) branchGroupMenu() []core.MenuItem {
	t, hasHead := v.headTarget()
	g := &menuGroups{}
	g.item(v.text("ref_new_branch"), !v.CanSync() || !hasHead, func() { v.openRefAction(opBranch, t) })
	g.item(v.text("ref_select_stale"), !v.available(), v.openStaleBranches)
	g.item(v.text("bulk_push_branches"), !v.CanSync(), func() { v.openBulk(bulkPushBranches) })
	g.item(v.text("bulk_delete_branches"), !v.CanSync(), func() { v.openBulk(bulkDeleteBranches) })
	return g.items
}

// remoteGroupMenu is the Remote branches heading's menu: adding a remote.
func (v *View) remoteGroupMenu() []core.MenuItem {
	g := &menuGroups{}
	g.item(v.text("remote_add"), !v.CanSync(), func() { v.openRemoteDialog("") })
	return g.items
}

// tagGroupMenu is the Tags heading's menu: a new tag at the checked out
// commit.
func (v *View) tagGroupMenu() []core.MenuItem {
	t, hasHead := v.headTarget()
	g := &menuGroups{}
	g.item(v.text("ref_new_tag"), !v.CanSync() || !hasHead, func() { v.openRefAction(opTag, t) })
	g.item(v.text("bulk_push_tags"), !v.CanSync(), func() { v.openBulk(bulkPushTags) })
	g.item(v.text("bulk_delete_tags"), !v.CanSync(), func() { v.openBulk(bulkDeleteTags) })
	return g.items
}

// remoteMenu is a remote's menu: fetching, editing or deleting it,
// fetching it whenever a worktree opens, opening it on GitHub, adding
// another remote and copying its address.
func (v *View) remoteMenu(cx *core.Scope, remote string) []core.MenuItem {
	can := v.CanSync()
	args := map[string]any{"Remote": remote}
	g := &menuGroups{}
	g.item(v.textf("remote_fetch", args), !can, func() { v.fetchRemotes([]string{remote}) })
	g.item(v.textf("remote_edit", args), !can, func() { v.openRemoteDialog(remote) })
	g.item(v.textf("remote_delete", args), !can, func() {
		v.remoteDeletion = remoteDeletion{open: true, name: remote}
		v.invalidate()
	})
	g.group()
	automatic := v.autoFetch(remote)
	g.check(v.textf("remote_auto_fetch", args), automatic, false, func() { v.setAutoFetch(remote, !automatic) })
	g.group()
	url := v.repo.remoteURLs[remote]
	if page := githubWebURL(url); page != "" {
		g.item(v.text("remote_view_github"), false, func() { cx.OpenURL(page, nil) })
	}
	g.group()
	g.item(v.text("remote_add"), !can, func() { v.openRemoteDialog("") })
	if url != "" {
		g.group()
		g.item(v.text("remote_copy"), false, func() { cx.SetClipboard(url) })
	}
	return g.items
}

// fetchRemotes updates each remote's branches, one after another, as one
// Git write of the view.
func (v *View) fetchRemotes(remotes []string) {
	if len(remotes) == 0 || !v.CanSync() {
		return
	}
	v.syncRepository(OpFetch, v.text("fetch_failed"), func(git Git, path string) error {
		var errs []error
		for _, remote := range remotes {
			if err := git.FetchRemote(path, remote); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", remote, err))
			}
		}
		return errors.Join(errs...)
	})
}

// autoFetch reports whether remote is fetched whenever a worktree opens.
func (v *View) autoFetch(remote string) bool {
	if v.repo.root == "" {
		return false
	}
	return slices.Contains(v.state.AutoFetch[v.repo.root], remote)
}

// autoFetchRemotes lists the remotes to fetch when a worktree opens, leaving
// out those the repository no longer has.
func (v *View) autoFetchRemotes() []string {
	if v.repo.root == "" {
		return nil
	}
	var remotes []string
	for _, remote := range v.state.AutoFetch[v.repo.root] {
		if _, ok := v.repo.remoteURLs[remote]; ok {
			remotes = append(remotes, remote)
		}
	}
	slices.Sort(remotes)
	return remotes
}

// setAutoFetch remembers whether remote is fetched when a worktree opens,
// fetching it now when it becomes automatic.
func (v *View) setAutoFetch(remote string, on bool) {
	if v.repo.root == "" {
		return
	}
	if v.state.AutoFetch == nil {
		v.state.AutoFetch = map[string][]string{}
	}
	list := slices.DeleteFunc(slices.Clone(v.state.AutoFetch[v.repo.root]), func(name string) bool { return name == remote })
	if on {
		list = append(list, remote)
	}
	slices.Sort(list)
	if len(list) == 0 {
		delete(v.state.AutoFetch, v.repo.root)
	} else {
		v.state.AutoFetch[v.repo.root] = list
	}
	v.saveState()
	if on {
		v.fetchRemotes([]string{remote})
	}
}

// fetchAutomatic fetches the remotes chosen for automatic fetching after a
// worktree opens.
func (v *View) fetchAutomatic() {
	if !v.closed {
		v.fetchRemotes(v.autoFetchRemotes())
	}
}

// renameAutoFetch moves a remote's automatic fetch setting in root's
// repository to its new name.
func (v *View) renameAutoFetch(root, name, newName string) {
	if root == "" || !slices.Contains(v.state.AutoFetch[root], name) {
		return
	}
	list := slices.DeleteFunc(slices.Clone(v.state.AutoFetch[root]), func(r string) bool { return r == name || r == newName })
	v.state.AutoFetch[root] = append(list, newName)
	slices.Sort(v.state.AutoFetch[root])
	v.saveState()
}

// forgetAutoFetch drops a deleted remote's automatic fetch setting from
// root's repository.
func (v *View) forgetAutoFetch(root, name string) {
	if root == "" || !slices.Contains(v.state.AutoFetch[root], name) {
		return
	}
	list := slices.DeleteFunc(slices.Clone(v.state.AutoFetch[root]), func(r string) bool { return r == name })
	if len(list) == 0 {
		delete(v.state.AutoFetch, root)
	} else {
		v.state.AutoFetch[root] = list
	}
	v.saveState()
}

// remoteDialog adds a remote, or edits remote's name and address.
type remoteDialog struct {
	open                bool
	edit                bool
	original, name, url string
}

// openRemoteDialog opens the dialog adding a remote, or editing remote when
// it is not empty.
func (v *View) openRemoteDialog(remote string) {
	d := remoteDialog{open: true, edit: remote != "", original: remote, name: remote, url: v.repo.remoteURLs[remote]}
	if remote == "" && len(v.repo.remoteURLs) == 0 {
		d.name = "origin"
	}
	v.remoteDialog = d
	v.invalidate()
}

// remoteProblem reports whether the remote dialog can be confirmed, with the
// problem to show for a name another remote has.
func (v *View) remoteProblem() (ready bool, problem string) {
	d := &v.remoteDialog
	name, url := strings.TrimSpace(d.name), strings.TrimSpace(d.url)
	if name == "" || url == "" {
		return false, ""
	}
	if _, ok := v.repo.remoteURLs[name]; ok && (!d.edit || name != d.original) {
		return false, v.textf("remote_exists", map[string]any{"Name": name})
	}
	return true, ""
}

// remoteDialogView is the dialog adding or editing a remote.
func (v *View) remoteDialogView(cx *core.Scope) core.Node {
	d := &v.remoteDialog
	ready := func() bool {
		ok, _ := v.remoteProblem()
		return ok && v.CanSync()
	}
	run := func() {
		d.open = false
		v.runRemoteDialog()
	}
	submit := func() {
		if ready() {
			run()
		}
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 400, testID: "remote-dialog",
		title: func() string {
			if d.edit {
				return v.textf("remote_edit_title", map[string]any{"Remote": d.original})
			}
			return v.text("remote_add_title")
		},
		description: func() string {
			if d.edit {
				return v.text("remote_edit_description")
			}
			return v.text("remote_add_description")
		},
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			return []core.Node{
				formField(cx, remoteLabelWidth, v.text("remote_name"), nil, v.textField(&d.name, v.text("remote_name"), "", !d.edit, submit, "remote-name")),
				formField(cx, remoteLabelWidth, v.text("remote_url"), nil, v.textField(&d.url, v.text("remote_url"), "https://example.com/owner/repository.git", d.edit, submit, "remote-url")),
				core.Text{ContentFunc: func() string {
					v.watch()
					_, problem := v.remoteProblem()
					return problem
				}, Style: core.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Danger)},
			}
		},
		actions: func(cx *core.Scope) []core.Node {
			label := v.text("remote_add_confirm")
			if d.edit {
				label = v.text("remote_edit_confirm")
			}
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(label, ready, run, "confirm")}
		},
		confirm: submit})
}

// runRemoteDialog adds or edits the remote as the dialog filled it in.
func (v *View) runRemoteDialog() {
	d := v.remoteDialog
	name, url := strings.TrimSpace(d.name), strings.TrimSpace(d.url)
	root := v.repo.root
	failed := v.text("remote_edit_failed")
	if !d.edit {
		failed = v.text("remote_add_failed")
	}
	v.write(opRemote, failed, func(git Git, path string) error {
		if !d.edit {
			return git.AddRemote(path, name, url)
		}
		if name != d.original {
			if err := git.RenameRemote(path, d.original, name); err != nil {
				return err
			}
		}
		return git.SetRemoteURL(path, name, url)
	}, func() {
		if d.edit && name != d.original {
			v.renameAutoFetch(root, d.original, name)
		}
	})
}

// remoteDeletion confirms deleting a remote.
type remoteDeletion struct {
	open bool
	name string
}

// remoteDeletionDialog asks before deleting a remote with its remote
// branches and configuration.
func (v *View) remoteDeletionDialog(cx *core.Scope) core.Node {
	d := &v.remoteDeletion
	run := func() {
		d.open = false
		v.runRemoteDeletion()
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, alert: true, testID: "remote-delete-dialog",
		title:       func() string { return v.textf("remote_delete_title", map[string]any{"Remote": d.name}) },
		description: func() string { return v.textf("remote_delete_message", map[string]any{"Remote": d.name}) },
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("delete"), v.CanSync, run, "confirm")}
		}})
}

// runRemoteDeletion deletes the remote and forgets its automatic fetch.
func (v *View) runRemoteDeletion() {
	name, root := v.remoteDeletion.name, v.repo.root
	v.write(opRemote, v.text("remote_delete_failed"), func(git Git, path string) error {
		return git.DeleteRemote(path, name)
	}, func() { v.forgetAutoFetch(root, name) })
}

// staleBranch is a local branch whose last commit is old enough to delete.
type staleBranch struct {
	name     string
	date     time.Time
	selected bool
}

// staleBranches chooses the old local branches to delete.
type staleBranches struct {
	open, loading bool
	generation    uint64
	branches      []staleBranch
}

// openStaleBranches reads the local branches and offers those whose last
// commit is older than staleAfterDays.
func (v *View) openStaleBranches() {
	if !v.available() {
		return
	}
	generation := v.stale.generation + 1
	v.stale = staleBranches{open: true, loading: true, generation: generation}
	path, git, keep := v.path, v.git, v.opts.RefFilter
	checkedOut := map[string]bool{}
	for _, c := range v.repo.checkouts {
		if c.Branch != "" {
			checkedOut[c.Branch] = true
		}
	}
	cutoff := time.Now().AddDate(0, 0, -staleAfterDays)
	v.invalidate()
	go func() {
		dates, err := git.BranchDates(path)
		v.dispatch(func() {
			d := &v.stale
			if generation != d.generation || v.path != path || !d.open || v.closed {
				return
			}
			d.loading = false
			if err != nil {
				d.open = false
				v.toast(v.text("ref_stale_read_failed"), err)
				v.invalidate()
				return
			}
			for _, b := range dates {
				if checkedOut[b.Name] || !b.Date.Before(cutoff) || (keep != nil && !keep(RefBranch, b.Name)) {
					continue
				}
				d.branches = append(d.branches, staleBranch{name: b.Name, date: b.Date, selected: true})
			}
			slices.SortFunc(d.branches, func(a, b staleBranch) int { return a.date.Compare(b.date) })
			v.invalidate()
		})
	}()
}

// staleBranchesDialogView lists the stale local branches with a delete
// action for the chosen ones.
func (v *View) staleBranchesDialogView(cx *core.Scope) core.Node {
	d := &v.stale
	selected := func() int {
		n := 0
		for _, b := range d.branches {
			if b.selected {
				n++
			}
		}
		return n
	}
	run := func() {
		d.open = false
		v.runStaleDeletion()
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 400, testID: "stale-dialog",
		title:       func() string { return v.text("ref_stale_title") },
		description: func() string { return v.textf("ref_stale_description", map[string]any{"Days": staleAfterDays}) },
		body: func(cx *core.Scope) []core.Node {
			return []core.Node{region(cx, func() string { v.track(); return fmt.Sprint(d.loading, len(d.branches)) }, func(cx *core.Scope) core.Node {
				switch {
				case d.loading:
					return spinner(16)
				case len(d.branches) == 0:
					return muted(cx, v.text("ref_stale_none"), 13)
				}
				var boxes []core.Node
				for i := range d.branches {
					b := &d.branches[i]
					boxes = append(boxes, v.checkbox(&b.selected, b.name+" · "+b.date.Format("2006-01-02"), nil))
				}
				return ui.ScrollView{Style: core.Style{MaxHeight: core.Pt(240)}, ContentStyle: core.Style{Gap: 6}, Children: boxes}
			})}
		},
		actions: func(cx *core.Scope) []core.Node {
			label := core.Dynamic(func(cx *core.Scope) core.Node {
				v.watch()
				var n core.Node
				cx.Untrack(func() {
					n = v.confirmButton(v.textf("ref_stale_delete", map[string]any{"Count": selected()}), func() bool { return selected() > 0 && v.CanSync() }, run, "confirm")
				})
				return n
			})
			return []core.Node{v.cancelButton(&d.open), label}
		}})
}

// runStaleDeletion deletes the chosen stale branches, keeping branches Git
// would refuse to delete for lost commits.
func (v *View) runStaleDeletion() {
	var names []string
	for _, b := range v.stale.branches {
		if b.selected {
			names = append(names, b.name)
		}
	}
	if len(names) == 0 {
		return
	}
	v.Run(opDelete, v.text("ref_stale_failed"), func(git Git, path string) error {
		var errs []error
		for _, name := range names {
			if err := git.DeleteBranch(path, name, false); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
		return errors.Join(errs...)
	})
}
