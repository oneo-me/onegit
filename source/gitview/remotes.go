package gitview

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
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

// branchGroupMenu fills the Branches heading's menu: a new branch at the
// checked out commit, and the stale local branches to delete.
func (v *View) branchGroupMenu(m *ui.Menu) {
	t, hasHead := v.headTarget()
	if m.Item(v.text("ref_new_branch")).Disabled(!v.CanSync() || !hasHead).Chosen() && hasHead {
		v.openRefAction(opBranch, t)
	}
	if m.Item(v.text("ref_select_stale")).Disabled(!v.available()).Chosen() {
		v.openStaleBranches()
	}
	if m.Item(v.text("bulk_push_branches")).Disabled(!v.CanSync()).Chosen() {
		v.openBulk(bulkPushBranches)
	}
	if m.Item(v.text("bulk_delete_branches")).Disabled(!v.CanSync()).Chosen() {
		v.openBulk(bulkDeleteBranches)
	}
}

// remoteGroupMenu fills the Remote branches heading's menu: adding a remote.
func (v *View) remoteGroupMenu(m *ui.Menu) {
	if m.Item(v.text("remote_add")).Disabled(!v.CanSync()).Chosen() {
		v.openRemoteDialog("")
	}
}

// tagGroupMenu fills the Tags heading's menu: a new tag at the checked out
// commit.
func (v *View) tagGroupMenu(m *ui.Menu) {
	t, hasHead := v.headTarget()
	if m.Item(v.text("ref_new_tag")).Disabled(!v.CanSync() || !hasHead).Chosen() && hasHead {
		v.openRefAction(opTag, t)
	}
	if m.Item(v.text("bulk_push_tags")).Disabled(!v.CanSync()).Chosen() {
		v.openBulk(bulkPushTags)
	}
	if m.Item(v.text("bulk_delete_tags")).Disabled(!v.CanSync()).Chosen() {
		v.openBulk(bulkDeleteTags)
	}
}

// remoteMenu fills a remote's menu: fetching, editing or deleting it,
// fetching it whenever a worktree opens, opening it on GitHub, adding
// another remote and copying its address.
func (v *View) remoteMenu(c *ui.Context, m *ui.Menu, remote string) {
	can := v.CanSync()
	args := map[string]any{"Remote": remote}
	g := &menuGroups{m: m}
	if g.item(v.textf("remote_fetch", args)).Disabled(!can).Chosen() {
		v.fetchRemotes([]string{remote})
	}
	if g.item(v.textf("remote_edit", args)).Disabled(!can).Chosen() {
		v.openRemoteDialog(remote)
	}
	if g.item(v.textf("remote_delete", args)).Disabled(!can).Chosen() {
		v.remoteDeletion = remoteDeletion{open: true, name: remote}
	}
	g.group()
	automatic := v.autoFetch(remote)
	if g.item(v.textf("remote_auto_fetch", args)).Checked(automatic).Chosen() {
		v.setAutoFetch(remote, !automatic)
	}
	g.group()
	url := v.repo.remoteURLs[remote]
	if page := githubWebURL(url); page != "" {
		if g.item(v.text("remote_view_github")).Chosen() {
			c.OpenURL(page)
		}
	}
	g.group()
	if g.item(v.text("remote_add")).Disabled(!can).Chosen() {
		v.openRemoteDialog("")
	}
	if url != "" {
		g.group()
		if g.item(v.text("remote_copy")).Chosen() {
			c.WriteClipboard(url)
		}
	}
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

// remoteLabelWidth is the width of the remote dialogs' labels in theme
// spaces.
const remoteLabelWidth = 20

// remoteDialogView is the dialog adding or editing a remote.
func (v *View) remoteDialogView(c *ui.Context) {
	d := &v.remoteDialog
	title, description, confirm := v.text("remote_add_title"), v.text("remote_add_description"), v.text("remote_add_confirm")
	if d.edit {
		args := map[string]any{"Remote": d.original}
		title, description, confirm = v.textf("remote_edit_title", args), v.text("remote_edit_description"), v.text("remote_edit_confirm")
	}
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(100))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			components.Text(c, description).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		field := func(label string, value *string, placeholder string, focus bool) {
			formField(c, remoteLabelWidth, label, false, func() {
				box := components.TextBox(c, value, components.TextBoxOptions{}).FillWidth().Label(label).Placeholder(placeholder)
				if focus {
					box.AutoFocus()
				}
			})
		}
		field(v.text("remote_name"), &d.name, "", !d.edit)
		field(v.text("remote_url"), &d.url, "https://example.com/owner/repository.git", d.edit)
		ready, problem := v.remoteProblem()
		if problem != "" {
			components.Text(c, problem).Wrap().FillWidth().TextColor(t.Danger)
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			if components.Button(c, confirm, components.ButtonPrimary).Disabled(!ready || !v.CanSync()).Clicked() {
				d.open = false
				v.runRemoteDialog()
			}
		})
	})
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
func (v *View) remoteDeletionDialog(c *ui.Context) {
	d := &v.remoteDeletion
	title := v.textf("remote_delete_title", map[string]any{"Remote": d.name})
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(100))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			components.Text(c, v.textf("remote_delete_message", map[string]any{"Remote": d.name})).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("delete"), components.ButtonPrimary).Disabled(!v.CanSync()).Clicked() {
				d.open = false
				v.runRemoteDeletion()
			}
		})
	})
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
				v.toast = &ui.Toast{Title: v.text("ref_stale_read_failed"), Description: err.Error(), Type: components.ToastError}
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
func (v *View) staleBranchesDialogView(c *ui.Context) {
	d := &v.stale
	title := v.text("ref_stale_title")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(100))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			components.Text(c, v.textf("ref_stale_description", map[string]any{"Days": staleAfterDays})).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		switch {
		case d.loading:
			components.Spinner(c, components.SpinnerRing).Size(16, 16)
		case len(d.branches) == 0:
			components.Text(c, v.text("ref_stale_none")).Wrap().FillWidth().TextColor(t.TextMuted)
		default:
			components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(t.Space(60)).Children(func() {
				for i := range d.branches {
					b := &d.branches[i]
					components.Checkbox(c, &b.selected, b.name+" · "+b.date.Format("2006-01-02"))
				}
			})
		}
		selected := 0
		for _, b := range d.branches {
			if b.selected {
				selected++
			}
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			label := v.textf("ref_stale_delete", map[string]any{"Count": selected})
			if components.Button(c, label, components.ButtonPrimary).Disabled(selected == 0 || !v.CanSync()).Clicked() {
				d.open = false
				v.runStaleDeletion()
			}
		})
	})
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
