package gitview

import (
	_ "embed"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
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

// GitHubPage is the GitHub page of the remote the worktree's branch tracks,
// else of the default remote, or empty when that remote is not on GitHub.
func (v *View) GitHubPage() string {
	remote := refRemote(v.repo.github, v.tracking.Upstream)
	if remote == "" {
		remote = (Remotes{Names: slices.Sorted(maps.Keys(v.repo.remoteURLs))}).DefaultRemote()
	}
	return githubWebURL(v.repo.remoteURLs[remote])
}

// Syncing names the Git write running in the shown worktree, or is empty.
func (v *View) Syncing() string { return v.syncing[v.path] }

// CanSync reports whether the view may start a Git write now: it shows a
// repository, none runs in its worktree and the host is not busy. Writes in
// other worktrees the view showed keep running.
func (v *View) CanSync() bool {
	return v.available() && v.Syncing() == ""
}

// CanStage reports whether the view may write the worktree's index: its
// changes are read and no write runs there. Staging and discarding part of a
// file do not need the repository's refs or history.
func (v *View) CanStage() bool {
	return v.path != "" && v.current != nil && v.current.loaded && v.current.err == "" &&
		v.Syncing() == ""
}

// CanPull reports whether a pull may start, which needs a checked out
// branch.
func (v *View) CanPull() bool { return v.CanSync() && v.tracking.Branch != "" }

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
				v.toast(failed, err)
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
				v.toast(v.text(op+"_failed"), err)
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

//go:embed icons/github.svg
var githubSVG []byte

// githubIcon is GitHub's mark, shown for remotes hosted there.
var githubIcon = sync.OnceValue(func() *core.SVG { return core.MustParseSVG(githubSVG) })

// remoteIcon is GitHub's mark for a remote hosted there, else a generic one.
func remoteIcon(github map[string]bool, remote string) func() *core.SVG {
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

// refDeletion is the dialog confirming the deletion of ref in the refs
// pane's group: a local branch, remote branch or tag. remote is the copy
// on a remote that alsoRemote deletes too, the remote branch a local
// branch tracks or a tag on the default remote, or the remote branch
// itself; it has no remote when there is none. worktree is the directory
// of another worktree that has a local branch checked out, which
// alsoWorktree removes first and the deletion needs. checking holds reading
// whether a local branch is unmerged, which force deletes anyway;
// generation discards a read the user replaced.
type refDeletion struct {
	open                     bool
	group, ref, worktree     string
	remote                   RemoteBranch
	alsoRemote, alsoWorktree bool
	generation               uint64
	checking, unmerged       bool
	force                    bool
	err                      string
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

// openRefDeletion opens the deletion dialog d, reading in the background
// whether a local branch is unmerged.
func (v *View) openRefDeletion(d refDeletion) {
	d.generation = v.deletion.generation + 1
	d.checking = d.group == "branches"
	v.deletion = d
	v.invalidate()
	if !d.checking {
		return
	}
	path, git := v.path, v.git
	go func() {
		unmerged, err := git.UnmergedBranches(path)
		v.dispatch(func() {
			current := &v.deletion
			if current.generation != d.generation || !current.open || v.closed {
				return
			}
			current.checking, current.unmerged = false, unmerged[d.ref]
			if err != nil {
				current.err = err.Error()
			}
			v.invalidate()
		})
	}()
}

// deleteRef deletes the dialog's ref, forcing an unmerged branch only when
// the user chose to. A local branch goes first, after its worktree, so the
// remote keeps its copy when Git refuses to delete it; a tag goes from the
// remote first, as the refs pane no longer offers it once deleted locally. Removing a worktree reports OpWorktree.
func (v *View) deleteRef() {
	d := v.deletion
	alsoRemote := d.alsoRemote && d.remote.Remote != ""
	force := d.unmerged && d.force
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
				err = git.DeleteWorktreeBranch(path, d.worktree, d.ref, force)
			} else {
				err = git.DeleteBranch(path, d.ref, force)
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

// syncDialogView is the fetch, pull or push dialog: the operation's
// choices and options.
func (v *View) syncDialogView(cx *core.Scope) core.Node {
	d := &v.dialog
	ready := func() bool {
		r := d.remotes
		ok := false
		switch {
		case r == nil, v.createsGitHub() && !d.gh:
		case v.createsGitHub():
			ok = d.branch != "" && strings.TrimSpace(d.name) != ""
		case len(r.Names) == 0:
		case d.op == OpFetch:
			ok = !v.state.FetchBranch || d.remote != "" && slices.Contains(r.Branches[d.remote], d.branch)
		case d.op == OpPull:
			ok = r.Current != "" && d.remote != "" && slices.Contains(r.Branches[d.remote], d.branch)
		default:
			ok = d.branch != "" && d.to.Remote != "" && d.to.Branch != ""
		}
		return ok && v.Syncing() == ""
	}
	run := func() {
		d.open = false
		v.runSyncDialog()
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 440, testID: "sync-dialog",
		title: func() string { return v.text(d.op) },
		description: func() string {
			if v.createsGitHub() {
				return v.text("github_create_description")
			}
			return v.text(d.op + "_description")
		},
		body: func(cx *core.Scope) []core.Node {
			key := func() string {
				v.track()
				return fmt.Sprint(d.remotes != nil, v.createsGitHub(), d.gh, d.op)
			}
			return []core.Node{region(cx, key, func(cx *core.Scope) core.Node {
				r := d.remotes
				switch {
				case r == nil:
					return spinner(16)
				case v.createsGitHub() && !d.gh:
					return muted(cx, v.text("github_cli_missing"), 13)
				case v.createsGitHub():
					return column(12, v.githubFields(cx, r)...)
				case len(r.Names) == 0:
					return muted(cx, v.text("no_remotes"), 13)
				case d.op == OpFetch:
					return column(12, v.fetchFields(cx, r)...)
				case d.op == OpPull:
					return column(12, v.pullFields(cx, r)...)
				}
				return column(12, v.pushFields(cx, r)...)
			})}
		},
		actions: func(cx *core.Scope) []core.Node {
			confirm := core.Dynamic(func(cx *core.Scope) core.Node {
				v.track()
				label := v.text(d.op)
				if v.createsGitHub() {
					label = v.text("github_create")
				}
				return v.confirmButton(label, ready, run, "confirm")
			})
			return []core.Node{v.cancelButton(&d.open), confirm}
		},
		confirm: func() {
			if ready() {
				run()
			}
		}})
}

// fetchFields choose the remote branch a fetch updates, or every remote.
func (v *View) fetchFields(cx *core.Scope, r *Remotes) []core.Node {
	all := func() bool { v.watch(); return !v.state.FetchBranch }
	fetchAll := !v.state.FetchBranch
	return []core.Node{
		v.remoteField(cx, r, all),
		v.remoteBranchField(cx, r, all),
		core.View{Style: core.Style{Padding: core.Edges{Left: syncLabelWidth + 12}}, Children: []core.Node{
			v.toggle(cx, &fetchAll, v.text("fetch_all"), func() {
				v.state.FetchBranch = !fetchAll
				v.saveState()
				v.invalidate()
			}),
		}},
	}
}

// pullFields choose the remote and its branch a pull brings into the
// checked out branch, and how.
func (v *View) pullFields(cx *core.Scope, r *Remotes) []core.Node {
	return []core.Node{
		v.remoteField(cx, r, nil),
		v.remoteBranchField(cx, r, nil),
		formField(cx, syncLabelWidth, v.text("sync_into"), nil, branchChoice(cx, r.Current)),
		v.formOption(syncLabelWidth, &v.state.PullRebase, v.text("pull_rebase"), v.saveState),
		v.formOption(syncLabelWidth, &v.state.PullStash, v.text("pull_stash"), v.saveState),
	}
}

// remoteField chooses the dialog's remote, moving its branch to the
// remote's default; a disabled one only shows it.
func (v *View) remoteField(cx *core.Scope, r *Remotes, disabled func() bool) core.Node {
	d := &v.dialog
	var options []ui.Option[string]
	for _, remote := range r.Names {
		options = append(options, ui.Option[string]{Value: remote, Label: remote, Leading: ui.Icon(remoteIcon(v.repo.github, remote), 14, ui.Inherited)})
	}
	return formField(cx, syncLabelWidth, v.text("sync_remote"), disabled, selectField(v, &d.remote, v.text("sync_remote"), options, func() {
		d.branch = pullBranch(*r, d.remote)
	}, "sync-remote"))
}

// remoteBranchField chooses a branch of the dialog's remote; a disabled one
// only shows it.
func (v *View) remoteBranchField(cx *core.Scope, r *Remotes, disabled func() bool) core.Node {
	d := &v.dialog
	key := func() string { v.track(); return d.remote }
	return formField(cx, syncLabelWidth, v.text("sync_branch"), disabled, region(cx, key, func(cx *core.Scope) core.Node {
		var options []ui.Option[string]
		for _, b := range r.Branches[d.remote] {
			options = append(options, ui.Option[string]{Value: b, Label: b})
		}
		return selectField(v, &d.branch, v.text("sync_branch"), options, nil, "sync-branch")
	}))
}

// pushFields choose the local branch a push sends and the remote branch it
// goes to, and how.
func (v *View) pushFields(cx *core.Scope, r *Remotes) []core.Node {
	d := &v.dialog
	key := func() string { v.track(); return d.branch }
	target := region(cx, key, func(cx *core.Scope) core.Node {
		targets := pushTargets(*r, d.branch)
		var options []ui.Option[RemoteBranch]
		for _, to := range targets {
			name := to.Remote + "/" + to.Branch
			label := name
			switch {
			case to == targets[0]:
				label = v.textf("push_default", map[string]any{"Target": name})
			case !slices.Contains(r.Branches[to.Remote], to.Branch):
				label = v.textf("push_new_branch", map[string]any{"Target": name})
			}
			options = append(options, ui.Option[RemoteBranch]{Value: to, Label: label})
		}
		return selectField(v, &d.to, v.text("sync_to"), options, nil, "sync-to")
	})
	return []core.Node{
		v.pushBranchField(cx, r),
		formField(cx, syncLabelWidth, v.text("sync_to"), nil, target),
		v.formOption(syncLabelWidth, &v.state.PushTags, v.text("push_tags"), v.saveState),
		v.formOption(syncLabelWidth, &d.force, v.text("push_force"), nil),
	}
}

// pushBranchField chooses the local branch a push sends, moving its target
// to the branch's default.
func (v *View) pushBranchField(cx *core.Scope, r *Remotes) core.Node {
	d := &v.dialog
	var options []ui.Option[string]
	for _, b := range r.Local {
		options = append(options, ui.Option[string]{Value: b, Label: b})
	}
	return formField(cx, syncLabelWidth, v.text("sync_branch"), nil, selectField(v, &d.branch, v.text("sync_branch"), options, func() {
		d.to = pushTarget(*r, d.branch)
	}, "push-branch"))
}

// githubFields choose the local branch a push sends to the GitHub
// repository it creates, and the repository's name and visibility.
func (v *View) githubFields(cx *core.Scope, r *Remotes) []core.Node {
	d := &v.dialog
	return []core.Node{
		v.pushBranchField(cx, r),
		formField(cx, syncLabelWidth, v.text("github_name"), nil, v.textField(&d.name, v.text("github_name"), v.text("github_name_placeholder"), false, nil, "github-name")),
		formField(cx, syncLabelWidth, v.text("github_visibility"), nil, radios(v, &d.public, v.text("github_visibility"), []radioChoice[bool]{
			{false, v.text("github_private")}, {true, v.text("github_public")},
		}, nil)),
		v.formOption(syncLabelWidth, &v.state.PushTags, v.text("push_tags"), v.saveState),
	}
}

// refDeletionDialog asks before deleting a ref, offering to delete a local
// branch's upstream or a tag on the default remote too. A branch checked
// out in another worktree is deleted only with that worktree, and an
// unmerged branch only after the user chooses to lose its commits.
func (v *View) refDeletionDialog(cx *core.Scope) core.Node {
	d := &v.deletion
	args := func() map[string]any {
		return map[string]any{"Name": d.ref, "Remote": d.remote.Remote, "Target": d.remote.Remote + "/" + d.remote.Branch, "Worktree": filepath.Base(d.worktree)}
	}
	ready := func() bool {
		return v.CanSync() && !(d.worktree != "" && !d.alsoWorktree) && !d.checking && !(d.unmerged && !d.force)
	}
	run := func() {
		d.open = false
		v.deleteRef()
	}
	return v.dialogNode(cx, dialogSpec{open: &d.open, alert: true, testID: "delete-dialog",
		title: func() string { return v.textf("delete_"+d.group+"_title", args()) },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			state := region(cx, func() string { v.track(); return fmt.Sprint(d.checking, d.err, d.unmerged) }, func(cx *core.Scope) core.Node {
				switch {
				case d.checking:
					return busyNote(cx, v.text("delete_branches_checking"))
				case d.err != "":
					return muted(cx, d.err, 13)
				case d.unmerged:
					return column(8, dialogNote(v.text("delete_branches_unmerged"), 13, colors.Warn), v.toggle(cx, &d.force, v.text("delete_branches_force"), nil))
				}
				return muted(cx, v.textf("delete_"+d.group+"_message", args()), 13)
			})
			nodes := []core.Node{state}
			if d.group != "remotes" && d.remote.Remote != "" {
				nodes = append(nodes, v.toggle(cx, &d.alsoRemote, v.textf("delete_"+d.group+"_remote", args()), nil))
			}
			if d.worktree != "" {
				nodes = append(nodes, v.toggle(cx, &d.alsoWorktree, v.textf("delete_worktree", args()), nil))
			}
			return nodes
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("delete"), ready, run, "confirm")}
		}})
}
