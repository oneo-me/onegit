package gitview

import (
	"context"
	_ "embed"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Git writes the view runs. A host's toolbar starts the first three; Syncing
// reports the running one.
const (
	OpFetch  = "fetch"
	OpPull   = "pull"
	OpPush   = "push"
	opDelete = "delete"
)

// Tracking is how the worktree's branch compares with its upstream, for a
// toolbar's counts.
func (v *View) Tracking() Tracking { return v.tracking }

// LocalBranches lists the shown local branches by name, for a toolbar's
// branch menu; Tracking names the checked out one.
func (v *View) LocalBranches() []string {
	var names []string
	var walk func([]*refNode)
	walk = func(nodes []*refNode) {
		for _, n := range nodes {
			if n.children != nil {
				walk(n.children)
			} else {
				names = append(names, n.path)
			}
		}
	}
	walk(v.repo.refTrees.branches)
	slices.Sort(names)
	return names
}

// Syncing names the Git write running, or is empty.
func (v *View) Syncing() string { return v.syncing }

// CanSync reports whether the view may start a Git write now: it shows a
// repository, none runs and the host is not busy.
func (v *View) CanSync() bool {
	return v.available() && v.syncing == "" && !v.busy()
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

// OpenPull opens the dialog choosing what a pull brings.
func (v *View) OpenPull() {
	if v.CanPull() {
		v.openSync(OpPull)
	}
}

// OpenPush opens the dialog choosing what a push sends.
func (v *View) OpenPush() {
	if v.CanSync() {
		v.openSync(OpPush)
	}
}

// Run runs a host's Git write, named op, in the worktree like the view's
// own: one at a time, then rereading the view. A failure shows as a toast
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
	path := v.path
	if v.syncing != "" || path == "" {
		return
	}
	v.syncing = op
	v.invalidate()
	git := v.git
	go func() {
		err := run(git, path)
		v.dispatch(func() {
			v.syncing = ""
			if v.closed {
				return
			}
			if err != nil {
				v.toast = &ui.Toast{Title: failed, Description: err.Error(), Type: components.ToastError}
			}
			if v.path == path {
				v.Reload()
			}
			v.invalidate()
		})
	}()
}

// syncDialog chooses what a pull brings or a push sends; remotes is what
// the worktree offers, nil while it is read. A pull brings branch from
// remote into the checked out branch; a push sends the local branch to to.
type syncDialog struct {
	op         string
	open       bool
	generation uint64
	remotes    *Remotes
	remote     string
	branch     string
	to         RemoteBranch
	force      bool
}

// openSync opens the pull or push dialog and reads the remotes and
// branches it offers.
func (v *View) openSync(op string) {
	path := v.path
	v.dialog = syncDialog{op: op, open: true, generation: v.dialog.generation + 1}
	generation, git := v.dialog.generation, v.git
	v.invalidate()
	go func() {
		remotes, err := git.Remotes(path)
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
			d.remotes = &remotes
			if up, ok := remotes.Upstreams[remotes.Current]; ok {
				d.remote = up.Remote
			} else {
				d.remote = fallbackRemote(remotes)
			}
			d.branch = pullBranch(remotes, d.remote)
			if op == OpPush {
				d.branch = remotes.Current
				if d.branch == "" && len(remotes.Local) > 0 {
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

// syncDialogView is the pull or push dialog: the operation's choices and
// options.
func (v *View) syncDialogView(c *ui.Context) {
	d := &v.dialog
	title := v.text(d.op)
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			components.Text(c, v.text(d.op+"_description")).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		r := d.remotes
		ready := false
		switch {
		case r == nil:
			components.Spinner(c, components.SpinnerRing).Size(16, 16)
		case len(r.Names) == 0:
			components.Text(c, v.text("no_remotes")).Wrap().FillWidth().TextColor(t.TextMuted)
		case d.op == OpPull:
			ready = v.pullFields(c, r)
		default:
			ready = v.pushFields(c, r)
		}
		ready = ready && v.syncing == "" && !v.busy()
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				d.open = false
			}
			if components.Button(c, title, components.ButtonPrimary).Disabled(!ready).AutoFocus().Clicked() {
				d.open = false
				v.runSyncDialog()
			}
		})
	})
}

// runSyncDialog pulls or pushes as the dialog chose.
func (v *View) runSyncDialog() {
	d := v.dialog
	if d.op == OpPull {
		o := PullOptions{Remote: d.remote, Branch: d.branch, Rebase: v.state.PullRebase, Stash: v.state.PullStash}
		v.syncRepository(OpPull, v.text("pull_failed"), func(git Git, path string) error { return git.Pull(path, o) })
		return
	}
	o := PushOptions{Branch: d.branch, Remote: d.to.Remote, To: d.to.Branch, Tags: v.state.PushTags, Force: d.force}
	v.syncRepository(OpPush, v.text("push_failed"), func(git Git, path string) error { return git.PushBranch(path, o) })
}

// pullFields choose the remote and its branch a pull brings into the
// checked out branch, and how; they report whether the pull can run.
func (v *View) pullFields(c *ui.Context, r *Remotes) bool {
	d := &v.dialog
	syncField(c, v.text("sync_remote"), func() {
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
	syncField(c, v.text("sync_branch"), func() {
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
	syncField(c, v.text("sync_into"), func() {
		ui.Row(c).Padding(0, c.Theme().Space(2.5)).Children(func() { branchChoice(c, r.Current) })
	})
	v.syncOption(c, &v.state.PullRebase, v.text("pull_rebase"), true)
	v.syncOption(c, &v.state.PullStash, v.text("pull_stash"), true)
	return r.Current != "" && d.remote != "" && slices.Contains(r.Branches[d.remote], d.branch)
}

// pushFields choose the local branch a push sends and the remote branch it
// goes to, and how; they report whether the push can run.
func (v *View) pushFields(c *ui.Context, r *Remotes) bool {
	d := &v.dialog
	syncField(c, v.text("sync_branch"), func() {
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
	syncField(c, v.text("sync_to"), func() {
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

// syncLabelWidth is the width of syncField's labels in theme spaces.
const syncLabelWidth = 16

// syncField is a dialog row with its label aligned to the others' end.
func syncField(c *ui.Context, label string, value func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
		components.Text(c, label).Width(t.Space(syncLabelWidth)).Shrink(0).TextAlign(ui.End).SingleLine()
		ui.Row(c).Grow(1).MinWidth(0).Children(value)
	})
}

// syncOption is a dialog check box aligned with the fields' values; a
// remembered option is saved when it changes.
func (v *View) syncOption(c *ui.Context, checked *bool, label string, remember bool) {
	t := c.Theme()
	before := *checked
	ui.Row(c).FillWidth().Padding(0, 0, 0, t.Space(syncLabelWidth+3)).Children(func() {
		components.Checkbox(c, checked, label)
	})
	if remember && *checked != before {
		v.saveState()
	}
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
// itself; it has no remote when there is none.
type refDeletion struct {
	open       bool
	group, ref string
	remote     RemoteBranch
	alsoRemote bool
}

// refDeleteItem adds deleting n of group to its context menu, disabled for
// a ref the view cannot delete or while a Git write or host work runs.
func (v *View) refDeleteItem(m *ui.Menu, group string, n *refNode) {
	d, ok := v.newRefDeletion(group, n)
	if m.Item(v.text("delete_ref")).Disabled(!ok || !v.CanSync()).Chosen() {
		v.deletion = d
	}
}

// newRefDeletion prepares the dialog deleting n of group, reporting false
// for a branch checked out in a worktree, which Git keeps, or a remote
// branch of no known remote.
func (v *View) newRefDeletion(group string, n *refNode) (refDeletion, bool) {
	d := refDeletion{open: true, group: group, ref: n.path}
	switch group {
	case "branches":
		if slices.ContainsFunc(v.repo.checkouts, func(c Checkout) bool { return c.Branch == n.path }) {
			return d, false
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
// branch's upstream or a tag on the default remote too.
func (v *View) refDeletionDialog(c *ui.Context) {
	d := &v.deletion
	args := map[string]any{"Name": d.ref, "Remote": d.remote.Remote, "Target": d.remote.Remote + "/" + d.remote.Branch}
	title := v.textf("delete_"+d.group+"_title", args)
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Role(ui.RoleAlertDialog)
		components.Text(c, title).Bold().Wrap().FillWidth()
		components.Text(c, v.textf("delete_"+d.group+"_message", args)).Wrap().FillWidth().TextColor(t.TextMuted)
		if d.group != "remotes" && d.remote.Remote != "" {
			components.Switch(c, &d.alsoRemote, v.textf("delete_"+d.group+"_remote", args))
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).AutoFocus().Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("delete"), components.ButtonPrimary).Disabled(!v.CanSync()).Clicked() {
				d.open = false
				v.deleteRef()
			}
		})
	})
}

// deleteRef deletes the dialog's ref. A local branch goes first, so the
// remote keeps its copy when Git refuses to delete it; a tag goes from the
// remote first, as the refs pane no longer offers it once deleted locally.
func (v *View) deleteRef() {
	d := v.deletion
	alsoRemote := d.alsoRemote && d.remote.Remote != ""
	if v.ref == d.group+"/"+d.ref {
		v.ref = ""
	}
	v.syncRepository(opDelete, v.text("delete_failed"), func(git Git, path string) error {
		switch d.group {
		case "branches":
			if err := git.DeleteBranch(path, d.ref); err != nil || !alsoRemote {
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
	var ignore func(string, bool) bool
	if watch.repository {
		gitDir, err := v.git.GitDir(dir)
		if err != nil {
			return
		}
		dir = gitDir
		ignore = func(path string, _ bool) bool {
			switch path {
			case "objects", "lfs", "logs", "hooks", "info":
				return true
			}
			return false
		}
	} else {
		ignore = func(path string, _ bool) bool {
			name := path[strings.LastIndex(path, "/")+1:]
			return name == ".git" || name == "node_modules"
		}
	}
	watcher, err := mygo.WatchFiles(ctx, dir, mygo.FileWatchOptions{
		Recursive: true, HintsOnly: true, Ignore: ignore,
		Debounce: 300 * time.Millisecond, MaxDelay: 2 * time.Second,
	})
	if err != nil {
		return
	}
	defer watcher.Close()
	for {
		if _, err := watcher.Next(ctx); err != nil {
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
