package gitview

import (
	"context"
	_ "embed"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Git writes the view runs. Its toolbar starts all but switching and
// deletion; Syncing reports the running one.
const (
	OpFetch  = "fetch"
	OpPull   = "pull"
	OpPush   = "push"
	OpStash  = "stash"
	OpSwitch = "switch"
	opDelete = "delete"
)

// Tracking is how the worktree's branch compares with its upstream, for a
// toolbar's counts.
func (v *View) Tracking() Tracking { return v.tracking }

// Syncing names the Git write running in the shown worktree, or is empty.
func (v *View) Syncing() string { return v.syncing[v.path] }

// CanSync reports whether the view may start a Git write now: it shows a
// repository, none runs in its worktree and the host is not busy. Writes in
// other worktrees the view showed keep running.
func (v *View) CanSync() bool {
	return v.available() && v.Syncing() == "" && !v.busy()
}

// CanPull reports whether a pull may start, which needs a checked out
// branch.
func (v *View) CanPull() bool { return v.CanSync() && v.tracking.Branch != "" }

func (v *View) busy() bool { return v.opts.Busy != nil && v.opts.Busy() }

// Fetch updates the remote branches and tags from every remote.
func (v *View) Fetch() {
	if v.CanSync() {
		v.syncRepository(OpFetch, v.text("fetch_failed"), func(git Git, path string) error { return git.Fetch(path) })
	}
}

// OpenFetch opens the dialog choosing what a fetch updates.
func (v *View) OpenFetch() {
	if v.CanSync() {
		v.openSync(OpFetch, RemoteBranch{}, "")
	}
}

// OpenPull opens the dialog choosing what a pull brings.
func (v *View) OpenPull() {
	if v.CanPull() {
		v.openSync(OpPull, RemoteBranch{}, "")
	}
}

// openPull opens the pull dialog bringing a remote branch, such as
// origin/main, when the view knows its remote.
func (v *View) openPull(ref string) {
	if !v.CanPull() {
		return
	}
	from := RemoteBranch{}
	if remote := refRemote(v.repo.github, ref); remote != "" {
		from = RemoteBranch{Remote: remote, Branch: ref[len(remote)+1:]}
	}
	v.openSync(OpPull, from, "")
}

// openPullBranch opens the pull dialog bringing the remote branch up.
func (v *View) openPullBranch(up RemoteBranch) {
	if v.CanPull() {
		v.openSync(OpPull, up, "")
	}
}

// OpenPush opens the dialog choosing what a push sends.
func (v *View) OpenPush() {
	v.openPush("")
}

// openPush opens the push dialog sending a local branch, or the checked out
// one when branch is empty.
func (v *View) openPush(branch string) {
	if v.CanSync() {
		v.openSync(OpPush, RemoteBranch{}, branch)
	}
}

// Run runs a host's Git write, named op, in the worktree like the view's
// own: one at a time in each worktree, then rereading the view. A failure shows as a toast
// titled failed.
func (v *View) Run(op, failed string, run func(git Git, path string) error) {
	if v.CanSync() {
		v.syncRepository(op, failed, run)
	}
}

// ActionTooltip names the branches the fetch, pull or push op moves
// between, or is empty.
func (v *View) ActionTooltip(op string) string {
	tr := v.tracking
	switch {
	case op == OpFetch:
		return v.text("fetch_tooltip")
	case tr.Branch == "":
		return ""
	case op == OpPull && tr.Upstream != "":
		return v.textf("pull_tooltip", map[string]any{"Branch": tr.Branch, "Upstream": tr.Upstream, "Count": tr.Behind})
	case op == OpPush && tr.Upstream != "":
		return v.textf("push_tooltip", map[string]any{"Branch": tr.Branch, "Upstream": tr.Upstream, "Count": tr.Ahead})
	case op == OpPush:
		return v.textf("publish_tooltip", map[string]any{"Branch": tr.Branch, "Count": tr.Ahead})
	}
	return ""
}

// loadTracking rereads how the worktree's branch compares with its
// upstream.
func (v *View) loadTracking() {
	defer v.invalidate()
	v.trackingGeneration++
	path := v.path
	if path == "" {
		v.tracking = Tracking{}
		return
	}
	generation, git := v.trackingGeneration, v.git
	go func() {
		tracking, err := git.Tracking(path)
		v.dispatch(func() {
			if generation == v.trackingGeneration && !v.closed {
				if err != nil {
					tracking = Tracking{}
				}
				v.tracking = tracking
				v.invalidate()
			}
		})
	}()
}

// syncRepository runs a Git write in the worktree, then rereads the view; a
// failure shows as a toast titled failed.
func (v *View) syncRepository(op, failed string, run func(git Git, path string) error) {
	v.write(op, failed, run, nil)
}

// write runs a Git write as syncRepository does, then done unless it is nil
// or the write failed.
func (v *View) write(op, failed string, run func(git Git, path string) error, done func()) {
	path := v.path
	if v.syncing[path] != "" || path == "" {
		return
	}
	v.syncing[path] = op
	v.invalidate()
	git := v.git
	go func() {
		err := run(git, path)
		v.dispatch(func() {
			delete(v.syncing, path)
			if v.closed {
				return
			}
			if v.opts.OnWrite != nil {
				v.opts.OnWrite(op, path)
			}
			if err != nil {
				v.toast = &ui.Toast{Title: failed, Description: err.Error(), Type: components.ToastError}
			} else if done != nil {
				done()
			}
			if v.path == path {
				v.Reload()
			}
			v.invalidate()
		})
	}()
}

// syncDialog chooses what a fetch updates, a pull brings or a push sends;
// remotes is what the worktree offers, nil while it is read. A fetch updates
// branch of remote unless it updates every remote, a pull brings branch from
// remote into the checked out branch, and a push sends the local branch to
// to. A push from a repository without remotes on this computer or a
// GitHubHost creates a GitHub repository named name instead, private unless
// public, when gh, the GitHub CLI, is found, and sends the branch there.
type syncDialog struct {
	op         string
	open       bool
	generation uint64
	remotes    *Remotes
	remote     string
	branch     string
	to         RemoteBranch
	force      bool
	gh         bool
	name       string
	public     bool
}

// createsGitHub reports whether the dialog pushes to a GitHub repository it
// creates.
func (v *View) createsGitHub() bool {
	d := &v.dialog
	return d.op == OpPush && d.remotes != nil && len(d.remotes.Names) == 0 && v.git.runsGitHubCLI()
}

// openSync opens the fetch, pull or push dialog and reads the remotes and
// branches it offers; a pull brings from, and a push sends branch, when set
// and still offered, instead of their defaults.
func (v *View) openSync(op string, from RemoteBranch, branch string) {
	path := v.path
	name := v.repo.root
	if name == "" {
		name = path
	}
	v.dialog = syncDialog{op: op, open: true, generation: v.dialog.generation + 1, name: filepath.Base(name)}
	generation, git := v.dialog.generation, v.git
	v.invalidate()
	go func() {
		remotes, err := git.Remotes(path)
		gh := op == OpPush && len(remotes.Names) == 0 && git.GitHubCLI() != ""
		v.dispatch(func() {
			d := &v.dialog
			if generation != d.generation || !d.open || v.closed {
				return
			}
			defer v.invalidate()
			if err != nil {
				d.open = false
				v.toast = &ui.Toast{Title: v.text(op + "_failed"), Description: err.Error(), Type: components.ToastError}
				return
			}
			d.remotes, d.gh = &remotes, gh
			if up, ok := remotes.Upstreams[remotes.Current]; ok {
				d.remote = up.Remote
			} else {
				d.remote = fallbackRemote(remotes)
			}
			d.branch = pullBranch(remotes, d.remote)
			if slices.Contains(remotes.Branches[from.Remote], from.Branch) {
				d.remote, d.branch = from.Remote, from.Branch
			}
			if op == OpPush {
				d.branch = remotes.Current
				if slices.Contains(remotes.Local, branch) {
					d.branch = branch
				} else if d.branch == "" && len(remotes.Local) > 0 {
					d.branch = remotes.Local[0]
				}
				d.to = pushTarget(remotes, d.branch)
			}
		})
	}()
}

// fallbackRemote is the remote a branch without an upstream pulls from or
// pushes to: origin, else the only remote, else the first.
func fallbackRemote(r Remotes) string {
	if remote := r.DefaultRemote(); remote != "" || len(r.Names) == 0 {
		return remote
	}
	return r.Names[0]
}

// pullBranch is the branch of remote a pull brings by default: the checked
// out branch's upstream, else its namesake, else the remote's first.
func pullBranch(r Remotes, remote string) string {
	if up, ok := r.Upstreams[r.Current]; ok && up.Remote == remote {
		return up.Branch
	}
	branches := r.Branches[remote]
	if slices.Contains(branches, r.Current) || len(branches) == 0 {
		return r.Current
	}
	return branches[0]
}

// pushTarget is where a push sends branch by default: its upstream, else
// its namesake on the fallback remote.
func pushTarget(r Remotes, branch string) RemoteBranch {
	if up, ok := r.Upstreams[branch]; ok {
		return up
	}
	return RemoteBranch{Remote: fallbackRemote(r), Branch: branch}
}

// pushTargets lists where a push may send branch: its default first, then
// its namesake on every remote and the remotes' existing branches.
func pushTargets(r Remotes, branch string) []RemoteBranch {
	targets := []RemoteBranch{pushTarget(r, branch)}
	add := func(t RemoteBranch) {
		if t.Remote != "" && t.Branch != "" && !slices.Contains(targets, t) {
			targets = append(targets, t)
		}
	}
	for _, remote := range r.Names {
		add(RemoteBranch{Remote: remote, Branch: branch})
	}
	for _, remote := range r.Names {
		for _, b := range r.Branches[remote] {
			add(RemoteBranch{Remote: remote, Branch: b})
		}
	}
	return targets
}

// syncDialogView is the fetch, pull or push dialog: the operation's
// choices and options.
func (v *View) syncDialogView(c *ui.Context) {
	d := &v.dialog
	title, description, confirm := v.text(d.op), v.text(d.op+"_description"), v.text(d.op)
	if v.createsGitHub() {
		description, confirm = v.text("github_create_description"), v.text("github_create")
	}
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, description).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		r := d.remotes
		ready := false
		switch {
		case r == nil:
			components.Spinner(c, components.SpinnerRing).Size(16, 16)
		case v.createsGitHub() && !d.gh:
			components.Text(c, v.text("github_cli_missing")).Wrap().FillWidth().TextColor(t.TextMuted)
		case v.createsGitHub():
			ready = v.githubFields(c, r)
		case len(r.Names) == 0:
			components.Text(c, v.text("no_remotes")).Wrap().FillWidth().TextColor(t.TextMuted)
		case d.op == OpFetch:
			ready = v.fetchFields(c, r)
		case d.op == OpPull:
			ready = v.pullFields(c, r)
		default:
			ready = v.pushFields(c, r)
		}
		ready = ready && v.Syncing() == "" && !v.busy()
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			if components.Button(c, confirm, components.ButtonPrimary).Disabled(!ready).AutoFocus().Clicked() {
				d.open = false
				v.runSyncDialog()
			}
		})
	})
}

// runSyncDialog fetches, pulls or pushes as the dialog chose.
func (v *View) runSyncDialog() {
	d := v.dialog
	switch {
	case d.op == OpFetch && !v.state.FetchBranch:
		v.Fetch()
		return
	case d.op == OpFetch:
		v.syncRepository(OpFetch, v.text("fetch_failed"), func(git Git, path string) error {
			return git.FetchBranch(path, d.remote, d.branch)
		})
		return
	case d.op == OpPull:
		o := PullOptions{Remote: d.remote, Branch: d.branch, Rebase: v.state.PullRebase, Stash: v.state.PullStash}
		v.syncRepository(OpPull, v.text("pull_failed"), func(git Git, path string) error { return git.Pull(path, o) })
		return
	case v.createsGitHub():
		name, private := strings.TrimSpace(d.name), !d.public
		o := PushOptions{Branch: d.branch, Remote: "origin", To: d.branch, Tags: v.state.PushTags}
		v.syncRepository(OpPush, v.text("github_create_failed"), func(git Git, path string) error {
			if err := git.CreateGitHubRepository(path, name, private); err != nil {
				return err
			}
			return git.PushBranch(path, o)
		})
		return
	}
	o := PushOptions{Branch: d.branch, Remote: d.to.Remote, To: d.to.Branch, Tags: v.state.PushTags, Force: d.force}
	v.syncRepository(OpPush, v.text("push_failed"), func(git Git, path string) error { return git.PushBranch(path, o) })
}

// fetchFields choose the remote branch a fetch updates, or every remote;
// they report whether the fetch can run.
func (v *View) fetchFields(c *ui.Context, r *Remotes) bool {
	d := &v.dialog
	all := !v.state.FetchBranch
	v.remoteField(c, r, all)
	v.remoteBranchField(c, r, all)
	ui.Row(c).FillWidth().Padding(0, 0, 0, c.Theme().Space(syncLabelWidth+3)).Children(func() {
		components.Switch(c, &all, v.text("fetch_all"))
	})
	if all == v.state.FetchBranch {
		v.state.FetchBranch = !all
		v.saveState()
	}
	return all || d.remote != "" && slices.Contains(r.Branches[d.remote], d.branch)
}

// pullFields choose the remote and its branch a pull brings into the
// checked out branch, and how; they report whether the pull can run.
func (v *View) pullFields(c *ui.Context, r *Remotes) bool {
	d := &v.dialog
	v.remoteField(c, r, false)
	v.remoteBranchField(c, r, false)
	syncField(c, v.text("sync_into"), false, func() {
		ui.Row(c).Padding(0, c.Theme().Space(2.5)).Children(func() { branchChoice(c, r.Current) })
	})
	v.syncOption(c, &v.state.PullRebase, v.text("pull_rebase"), true)
	v.syncOption(c, &v.state.PullStash, v.text("pull_stash"), true)
	return r.Current != "" && d.remote != "" && slices.Contains(r.Branches[d.remote], d.branch)
}

// remoteField chooses the dialog's remote, moving its branch to the
// remote's default; a disabled one only shows it.
func (v *View) remoteField(c *ui.Context, r *Remotes, disabled bool) {
	d := &v.dialog
	syncField(c, v.text("sync_remote"), disabled, func() {
		before := d.remote
		sel := components.Select(c, &d.remote)
		sel.Trigger.Label(v.text("sync_remote")).FillWidth().Children(func() { v.remoteChoice(c, d.remote) })
		sel.Popup(func(panel ui.Element) {
			syncChoices(c, panel, func() {
				for _, remote := range r.Names {
					sel.Item(remote).Label(remote).Children(func() { v.remoteChoice(c, remote) })
				}
			})
		})
		if d.remote != before {
			d.branch = pullBranch(*r, d.remote)
		}
	})
}

// remoteBranchField chooses a branch of the dialog's remote; a disabled one
// only shows it.
func (v *View) remoteBranchField(c *ui.Context, r *Remotes, disabled bool) {
	d := &v.dialog
	syncField(c, v.text("sync_branch"), disabled, func() {
		sel := components.Select(c, &d.branch)
		sel.Trigger.Label(v.text("sync_branch")).FillWidth().Children(func() { branchChoice(c, d.branch) })
		sel.Popup(func(panel ui.Element) {
			syncChoices(c, panel, func() {
				for _, b := range r.Branches[d.remote] {
					sel.Item(b).Label(b).Children(func() { branchChoice(c, b) })
				}
			})
		})
	})
}

// pushFields choose the local branch a push sends and the remote branch it
// goes to, and how; they report whether the push can run.
func (v *View) pushFields(c *ui.Context, r *Remotes) bool {
	d := &v.dialog
	v.pushBranchField(c, r)
	targets := pushTargets(*r, d.branch)
	label := func(to RemoteBranch) string {
		name := to.Remote + "/" + to.Branch
		switch {
		case to == targets[0]:
			return v.textf("push_default", map[string]any{"Target": name})
		case !slices.Contains(r.Branches[to.Remote], to.Branch):
			return v.textf("push_new_branch", map[string]any{"Target": name})
		}
		return name
	}
	syncField(c, v.text("sync_to"), false, func() {
		sel := components.Select(c, &d.to)
		sel.Trigger.Label(v.text("sync_to")).FillWidth().Children(func() { branchChoice(c, label(d.to)) })
		sel.Popup(func(panel ui.Element) {
			syncChoices(c, panel, func() {
				for _, to := range targets {
					sel.Item(to).Label(label(to)).Children(func() { branchChoice(c, label(to)) })
				}
			})
		})
	})
	v.syncOption(c, &v.state.PushTags, v.text("push_tags"), true)
	v.syncOption(c, &d.force, v.text("push_force"), false)
	return d.branch != "" && d.to.Remote != "" && d.to.Branch != ""
}

// pushBranchField chooses the local branch a push sends, moving its target
// to the branch's default.
func (v *View) pushBranchField(c *ui.Context, r *Remotes) {
	d := &v.dialog
	syncField(c, v.text("sync_branch"), false, func() {
		before := d.branch
		sel := components.Select(c, &d.branch)
		sel.Trigger.Label(v.text("sync_branch")).FillWidth().Children(func() { branchChoice(c, d.branch) })
		sel.Popup(func(panel ui.Element) {
			syncChoices(c, panel, func() {
				for _, b := range r.Local {
					sel.Item(b).Label(b).Children(func() { branchChoice(c, b) })
				}
			})
		})
		if d.branch != before {
			d.to = pushTarget(*r, d.branch)
		}
	})
}

// githubFields choose the local branch a push sends to the GitHub
// repository it creates, and the repository's name and visibility; they
// report whether the push can run.
func (v *View) githubFields(c *ui.Context, r *Remotes) bool {
	d := &v.dialog
	v.pushBranchField(c, r)
	syncField(c, v.text("github_name"), false, func() {
		components.TextBox(c, &d.name, components.TextBoxOptions{}).FillWidth().Label(v.text("github_name")).Placeholder(v.text("github_name_placeholder"))
	})
	syncField(c, v.text("github_visibility"), false, func() {
		ui.Row(c).Gap(c.Theme().Space(4)).Children(func() {
			components.Radio(c, &d.public, false, v.text("github_private"))
			components.Radio(c, &d.public, true, v.text("github_public"))
		})
	})
	v.syncOption(c, &v.state.PushTags, v.text("push_tags"), true)
	return d.branch != "" && strings.TrimSpace(d.name) != ""
}

// syncLabelWidth is the width of syncField's labels in theme spaces.
const syncLabelWidth = 16

// syncField is a dialog row with its label aligned to the others' end; a
// disabled one takes no input.
func syncField(c *ui.Context, label string, disabled bool, value func()) {
	formField(c, syncLabelWidth, label, disabled, value)
}

// formField is syncField with labels width theme spaces wide.
func formField(c *ui.Context, width float32, label string, disabled bool, value func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Disabled(disabled).Children(func() {
		text := components.Text(c, label).Width(t.Space(width)).Shrink(0).TextAlign(ui.End).SingleLine()
		if disabled {
			text.TextColor(t.TextMuted)
		}
		ui.Row(c).Grow(1).MinWidth(0).Children(value)
	})
}

// syncOption is a dialog check box aligned with the fields' values; a
// remembered option is saved when it changes.
func (v *View) syncOption(c *ui.Context, checked *bool, label string, remember bool) {
	before := *checked
	formOption(c, syncLabelWidth, checked, label)
	if remember && *checked != before {
		v.saveState()
	}
}

// formOption is a check box aligned with the values of formFields whose
// labels are width theme spaces wide.
func formOption(c *ui.Context, width float32, checked *bool, label string) {
	ui.Row(c).FillWidth().Padding(0, 0, 0, c.Theme().Space(width+3)).Children(func() {
		components.Checkbox(c, checked, label)
	})
}

// syncChoices scrolls a select's options when they outgrow the window.
func syncChoices(c *ui.Context, panel ui.Element, items func()) {
	w, h := c.Size()
	panel.MinWidth(c.Theme().Space(60)).MaxWidth(max(0, w-16))
	components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().MaxHeight(min(c.Theme().Space(80), h/2)).Children(items)
}

// remoteChoice shows a remote with the mark of its host.
func (v *View) remoteChoice(c *ui.Context, remote string) {
	syncChoice(c, remoteIcon(v.repo.github, remote), remote)
}

//go:embed icons/github.svg
var githubSVG []byte

// githubIcon is GitHub's mark, shown for remotes hosted there.
var githubIcon = sync.OnceValue(func() *ui.SVG { return ui.MustParseSVG(githubSVG) })

// remoteIcon is GitHub's mark for a remote hosted there, else a generic one.
func remoteIcon(github map[string]bool, remote string) func() *ui.SVG {
	if github[remote] {
		return githubIcon
	}
	return lucide.Cloud
}

// refRemote is the remote a ref such as origin/main belongs to, preferring
// the longest name as remote names may contain slashes, or empty for a ref
// of no known remote.
func refRemote(github map[string]bool, ref string) string {
	remote := ""
	for name := range github {
		if len(name) > len(remote) && strings.HasPrefix(ref, name+"/") {
			remote = name
		}
	}
	return remote
}

func branchChoice(c *ui.Context, name string) { syncChoice(c, lucide.GitBranch, name) }

func syncChoice(c *ui.Context, icon func() *ui.SVG, label string) {
	ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(c.Theme().Space(2)).Children(func() {
		components.Icon(c, icon).Size(14, 14).Shrink(0)
		components.Text(c, label).MinWidth(0).SingleLine().Ellipsis("…")
	})
}

// refDeletion is the dialog confirming the deletion of ref in the refs
// pane's group: a local branch, remote branch or tag. remote is the copy
// on a remote that alsoRemote deletes too, the remote branch a local
// branch tracks or a tag on the default remote, or the remote branch
// itself; it has no remote when there is none. worktree is the directory
// of another worktree that has a local branch checked out, which
// alsoWorktree removes first and the deletion needs.
type refDeletion struct {
	open                     bool
	group, ref, worktree     string
	remote                   RemoteBranch
	alsoRemote, alsoWorktree bool
}

// newRefDeletion prepares the dialog deleting n of group, reporting false
// for a branch checked out in the shown or main worktree, which stay, or a
// remote branch of no known remote.
func (v *View) newRefDeletion(group string, n *refNode) (refDeletion, bool) {
	d := refDeletion{open: true, group: group, ref: n.path}
	switch group {
	case "branches":
		if i := slices.IndexFunc(v.repo.checkouts, func(c Checkout) bool { return c.Branch == n.path }); i >= 0 {
			dir := v.repo.checkouts[i].Path
			if i == 0 || n.path == v.tracking.Branch || filepath.Clean(dir) == filepath.Clean(v.path) {
				return d, false
			}
			d.worktree = dir
		}
		if _, ok := v.repo.github[n.upstream.Remote]; ok {
			d.remote = n.upstream
		}
	case "remotes":
		remote := refRemote(v.repo.github, n.path)
		if remote == "" {
			return d, false
		}
		d.remote = RemoteBranch{Remote: remote, Branch: n.path[len(remote)+1:]}
	case "tags":
		if remote := (Remotes{Names: slices.Sorted(maps.Keys(v.repo.github))}).DefaultRemote(); remote != "" {
			d.remote = RemoteBranch{Remote: remote, Branch: n.path}
		}
	}
	return d, true
}

// refDeletionDialog asks before deleting a ref, offering to delete a local
// branch's upstream or a tag on the default remote too. A branch checked
// out in another worktree is deleted only with that worktree.
func (v *View) refDeletionDialog(c *ui.Context) {
	d := &v.deletion
	args := map[string]any{"Name": d.ref, "Remote": d.remote.Remote, "Target": d.remote.Remote + "/" + d.remote.Branch, "Worktree": filepath.Base(d.worktree)}
	title := v.textf("delete_"+d.group+"_title", args)
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Role(ui.RoleAlertDialog)
		components.Text(c, title).Bold().Wrap().FillWidth()
		components.Text(c, v.textf("delete_"+d.group+"_message", args)).Wrap().FillWidth().TextColor(t.TextMuted)
		if d.group != "remotes" && d.remote.Remote != "" {
			components.Switch(c, &d.alsoRemote, v.textf("delete_"+d.group+"_remote", args))
		}
		if d.worktree != "" {
			components.Switch(c, &d.alsoWorktree, v.textf("delete_worktree", args))
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).AutoFocus().Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("delete"), components.ButtonPrimary).Disabled(!v.CanSync() || d.worktree != "" && !d.alsoWorktree).Clicked() {
				d.open = false
				v.deleteRef()
			}
		})
	})
}

// deleteRef deletes the dialog's ref. A local branch goes first, after
// its worktree, so the remote keeps its copy when Git refuses to delete it;
// a tag goes from the remote first, as the refs pane no longer offers it
// once deleted locally. Removing a worktree reports OpWorktree.
func (v *View) deleteRef() {
	d := v.deletion
	alsoRemote := d.alsoRemote && d.remote.Remote != ""
	if v.ref == d.group+"/"+d.ref {
		v.ref = ""
	}
	op := opDelete
	if d.worktree != "" {
		op = OpWorktree
	}
	v.syncRepository(op, v.text("delete_failed"), func(git Git, path string) error {
		switch d.group {
		case "branches":
			var err error
			if d.worktree != "" {
				err = git.DeleteWorktreeBranch(path, d.worktree, d.ref)
			} else {
				err = git.DeleteBranch(path, d.ref)
			}
			if err != nil || !alsoRemote {
				return err
			}
			return git.DeleteRemoteBranch(path, d.remote.Remote, d.remote.Branch)
		case "remotes":
			return git.DeleteRemoteBranch(path, d.remote.Remote, d.remote.Branch)
		}
		if alsoRemote {
			if err := git.DeleteRemoteTag(path, d.remote.Remote, d.ref); err != nil {
				return err
			}
		}
		return git.DeleteTag(path, d.ref)
	})
}

// gitWatch follows a directory for the view: the repository's Git
// directory, or a shown worktree at path.
type gitWatch struct {
	repository, ended bool
	path              string
	cancel            context.CancelFunc
}

// followWatches watches, with Options.Watch, the repository's Git
// directory, for its refs, HEADs and indexes, the shown worktree and the
// selected entry's directory, and stops watching what it no longer shows.
// A watch that ended starts again only with retry.
func (v *View) followWatches(retry bool) {
	// Keys keep a repository's watch apart from its main worktree's, which
	// share a path.
	want := map[string]string{}
	if v.opts.Watch && !v.closed && v.available() {
		want["repository\x00"+v.path] = v.path
		// The worktree's files decide whether the refs pane offers its
		// changes.
		want["worktree\x00"+v.path] = v.path
		if dir := v.selectedWatch(); dir != "" {
			want["worktree\x00"+dir] = dir
		}
	}
	for key, watch := range v.watches {
		if _, ok := want[key]; !ok {
			watch.cancel()
			delete(v.watches, key)
		}
	}
	for key, dir := range want {
		if watch := v.watches[key]; watch != nil && !(retry && watch.ended) {
			continue
		}
		if v.watches == nil {
			v.watches = map[string]*gitWatch{}
		}
		ctx, cancel := context.WithCancel(context.Background())
		watch := &gitWatch{repository: strings.HasPrefix(key, "repository\x00"), path: dir, cancel: cancel}
		v.watches[key] = watch
		go v.watch(ctx, watch, dir)
	}
}

func (v *View) stopWatches() {
	for key, watch := range v.watches {
		watch.cancel()
		delete(v.watches, key)
	}
}

// watch rereads the view after each change under dir until the watch is
// canceled or dir goes away. Hints are enough: a change only says what to
// read again. A repository's object database is left out, since new
// objects matter only once a ref points at them.
func (v *View) watch(ctx context.Context, watch *gitWatch, dir string) {
	defer v.dispatch(func() { watch.ended = true })
	if watch.repository {
		gitDir, err := v.git.GitDir(dir)
		if err != nil {
			return
		}
		dir = gitDir
	}
	watcher, err := v.git.watch(ctx, dir, watch.repository)
	if err != nil {
		return
	}
	defer watcher.Close()
	for {
		if err := watcher.Next(ctx); err != nil {
			return
		}
		v.dispatch(func() {
			if ctx.Err() != nil || v.closed {
				return
			}
			if watch.repository {
				v.loadRepository()
				v.loadTracking()
				v.loadChanges()
				return
			}
			if watch.path == v.path {
				v.loadChanges()
			}
			if v.selectedWatch() == watch.path {
				v.selectCommit(v.history.selected)
			}
		})
	}
}
