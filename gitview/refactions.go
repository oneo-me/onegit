package gitview

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// OpWorktree is the Git write adding a worktree, or removing one with its
// branch, after which a host listing the repository's worktrees reads them
// again.
const OpWorktree = "worktree"

// Git writes of the refs' menus, besides OpSwitch, OpWorktree and deletion.
const (
	opCheckout    = "checkout"
	opMerge       = "merge"
	opRebase      = "rebase"
	opBranch      = "branch"
	opTag         = "tag"
	opFastForward = "fast-forward"
	opTrack       = "track"
	opRename      = "rename"
)

// refTarget is what a menu of the refs pane or the history acts on: a ref of
// the refs pane's group, "branches", "remotes" or "tags", or a commit, whose
// group is "commit", name its short hash and subject its message's.
type refTarget struct {
	group, name, hash, subject string
	upstream                   RemoteBranch
}

func nodeTarget(group string, n *refNode) refTarget {
	return refTarget{group: group, name: n.path, hash: n.hash, upstream: n.upstream}
}

func commitTarget(c Commit) refTarget {
	return refTarget{group: "commit", name: c.Short, hash: c.Hash, subject: c.Subject}
}

// rev names the target to Git unambiguously.
func (t refTarget) rev() string {
	switch t.group {
	case "branches":
		return "refs/heads/" + t.name
	case "remotes":
		return "refs/remotes/" + t.name
	case "tags":
		return "refs/tags/" + t.name
	}
	return t.hash
}

// mergeChoice is how a merge records its result.
type mergeChoice int

const (
	mergeDefault mergeChoice = iota
	mergeNoFF
	mergeSquash
	mergeNoCommit
)

// refAction is the dialog of a refs menu's write, op, on target: a checkout
// or new worktree, a merge, or a new branch or tag. name is the new branch
// or tag, path the new worktree's directory and message an annotated tag's;
// suggested is the directory offered for path while not edited. checkout
// also checks out a new branch, changes is what a checkout does with
// uncommitted changes, and options tell a merge how.
type refAction struct {
	open                           bool
	op                             string
	target                         refTarget
	name, path, suggested, message string
	checkout                       bool
	changes                        LocalChanges
	merge                          mergeChoice
	options                        IntegrateOptions
}

// refLeaves lists the refs of a ref tree, leaving out its folders.
func refLeaves(nodes []*refNode) []*refNode {
	var refs []*refNode
	for _, n := range nodes {
		if n.children != nil {
			refs = append(refs, refLeaves(n.children)...)
		} else {
			refs = append(refs, n)
		}
	}
	return refs
}

// hasRef reports whether a ref tree has a ref named name.
func hasRef(nodes []*refNode, name string) bool {
	return slices.ContainsFunc(refLeaves(nodes), func(n *refNode) bool { return n.path == name })
}

// checkedOut reports whether a worktree has the local branch checked out.
func (v *View) checkedOut(branch string) bool {
	return slices.ContainsFunc(v.repo.checkouts, func(c Checkout) bool { return c.Branch == branch })
}

// checkedOutName names the checked out branch, or HEAD when detached.
func (v *View) checkedOutName() string {
	if v.tracking.Branch != "" {
		return v.tracking.Branch
	}
	return "HEAD"
}

// refMenu is the menu of a ref: checking it out, here or in a new
// worktree, moving a local branch to its upstream or pushing it, pulling a
// remote branch or the checked out branch's upstream, merging it or
// rebasing onto it, pushing a tag, branching or tagging from it, a local
// branch's upstream and name, deleting a ref and copying its name. Items
// that cannot apply to the target are left out; writes are disabled while
// one runs.
func (v *View) refMenu(cx *core.Scope, t refTarget, n *refNode) []core.MenuItem {
	can := v.CanSync()
	args := map[string]any{"Name": t.name, "Current": v.checkedOutName()}
	local := t.group == "branches"
	g := &menuGroups{}
	checkout, label := v.checkoutTarget(t), v.text("ref_checkout")
	if t.group == "tags" {
		label = v.text("ref_checkout_tags")
	}
	if checkout.name != t.name {
		label = v.textf("ref_checkout_branch", map[string]any{"Name": checkout.name})
	}
	// A branch another worktree has checked out opens that worktree
	// instead of checking it out again.
	if other := v.otherCheckout(checkout); other != "" && v.opts.OpenWorktree != nil {
		g.item(v.textf("ref_open_worktree", map[string]any{"Worktree": filepath.Base(other)}), false, func() { v.opts.OpenWorktree(other) })
	} else {
		if v.canCheckout(checkout) {
			g.item(label, !can, func() { v.openRefAction(opCheckout, checkout) })
		}
		if !v.checkedOut(checkout.name) {
			g.item(v.text("ref_worktree"), !can, func() { v.openRefAction(OpWorktree, t) })
		}
	}
	g.group()
	if local && n != nil {
		v.remoteItems(g, n)
	}
	if t.group == "remotes" && v.tracking.Branch != "" {
		g.item(v.textf("ref_pull", args), !v.CanPull(), func() { v.openPull(t.name) })
	}
	if t.group == "tags" {
		v.tagPushItems(g, t.name)
	}
	g.group()
	// Integrating the checked out branch with itself does nothing, and a
	// ref at the checked out commit has nothing to replay.
	own := local && t.name == v.tracking.Branch
	// A local branch may still be merged or rebased at the same commit,
	// as Fork offers; a tag or remote branch at HEAD has nothing to do.
	same := t.hash == v.repo.head && !local
	if !own {
		g.item(v.textf("ref_merge", args), !can || same, func() { v.openRefAction(opMerge, t) })
		if v.tracking.Branch != "" {
			g.item(v.textf("ref_rebase", args), !can || same, func() { v.openRebase(t) })
			g.item(v.textf("ref_rebase_interactive", args), !can || same || v.git.Host != nil, func() { v.openInteractiveRebase(t.hash) })
		}
	}
	g.group()
	g.item(v.text("ref_new_branch"), !can, func() { v.openRefAction(opBranch, t) })
	g.item(v.text("ref_new_tag"), !can, func() { v.openRefAction(opTag, t) })
	g.group()
	if n != nil {
		if local {
			v.trackingMenu(g, n)
			g.item(v.text("ref_rename"), !can, func() { v.openRefAction(opRename, t) })
		}
		pin := v.text("ref_pin")
		if v.isPinned(t.group, t.name) {
			pin = v.text("ref_unpin")
		}
		g.item(pin, false, func() { v.togglePinned(t.group, t.name) })
		if d, ok := v.newRefDeletion(t.group, n); ok {
			g.item(v.text("delete_ref"), !can, func() { v.openRefDeletion(d) })
		}
	}
	g.group()
	g.item(v.text("ref_copy_"+t.group), false, func() { cx.SetClipboard(t.name) })
	return g.items
}

// commitRef is a branch, remote branch or tag pointing at a commit, with
// the node it has in the refs pane.
type commitRef struct {
	group string
	node  *refNode
}

// refsAt lists the refs pointing at a commit, the way Fork lists them at
// the top of a commit's menu: its branches, a local branch before the
// remote branch it tracks, then its tags.
func (v *View) refsAt(hash string) []commitRef {
	var branches, tags []commitRef
	for _, group := range []struct {
		key   string
		nodes []*refNode
	}{
		{"branches", v.repo.refTrees.branches},
		{"remotes", v.repo.refTrees.remotes},
		{"tags", v.repo.refTrees.tags},
	} {
		for _, n := range refLeaves(group.nodes) {
			if n.hash != hash {
				continue
			}
			r := commitRef{group: group.key, node: n}
			if group.key == "tags" {
				tags = append(tags, r)
			} else {
				branches = append(branches, r)
			}
		}
	}
	slices.SortStableFunc(branches, v.compareBranchesAt)
	slices.SortFunc(tags, func(a, b commitRef) int { return cmp.Compare(a.node.path, b.node.path) })
	return append(branches, tags...)
}

// compareBranchesAt orders branches at a commit by name, comparing a local
// branch through the remote branch it tracks so the two stay together, and
// the local branch first.
func (v *View) compareBranchesAt(a, b commitRef) int {
	if c := cmp.Compare(v.branchSortName(a), v.branchSortName(b)); c != 0 {
		return c
	}
	if a.group != b.group {
		if a.group == "branches" {
			return -1
		}
		return 1
	}
	return cmp.Compare(a.node.path, b.node.path)
}

// branchSortName names a branch at a commit for sorting: the branch part of
// a remote branch, the branch it tracks for a local branch that tracks one,
// else its name.
func (v *View) branchSortName(r commitRef) string {
	if r.group == "remotes" {
		if remote := refRemote(v.repo.github, r.node.path); remote != "" {
			return r.node.path[len(remote)+1:]
		}
		return r.node.path
	}
	if up := r.node.upstream; up.Remote != "" {
		return up.Branch
	}
	return r.node.path
}

// commitRefsItems adds a submenu for each ref pointing at commit, filled
// with the menu the refs pane gives that ref: its branches, then its tags.
func (v *View) commitRefsItems(cx *core.Scope, g *menuGroups, commit Commit) {
	tags := false
	for _, r := range v.refsAt(commit.Hash) {
		if r.group == "tags" && !tags {
			tags = true
			g.group()
		}
		t, n := nodeTarget(r.group, r.node), r.node
		g.submenu(n.path, false, v.refMenu(cx, t, n))
	}
}

// commitMenu is a history row's menu: the menus of the branches, remote
// branches and tags pointing at it, checking the commit out, merging it
// into or rebasing the checked out branch to it, branching or tagging from
// it, resetting the branch or HEAD to it, cherry-picking or reverting it,
// saving its patch and copying its hash or summary. A detached HEAD has no
// branch to merge or rebase, but resets.
func (v *View) commitMenu(cx *core.Scope, commit Commit) []core.MenuItem {
	t := commitTarget(commit)
	can := v.CanSync()
	checkedOut := v.tracking.Branch != ""
	args := map[string]any{"Name": commit.Short, "Current": v.checkedOutName()}
	head := commit.Hash == v.repo.head
	g := &menuGroups{}
	// A commit a ref points at leads with that ref's menu, as Fork does, so
	// its branch, remote branch or tag acts from the history row too.
	v.commitRefsItems(cx, g, commit)
	g.group()
	g.item(v.text("ref_new_branch"), !can, func() { v.openRefAction(opBranch, t) })
	g.item(v.text("ref_new_tag"), !can, func() { v.openRefAction(opTag, t) })
	g.group()
	if checkedOut {
		g.item(v.textf("ref_merge_here", args), !can || head, func() { v.openRefAction(opMerge, t) })
		g.item(v.textf("ref_rebase_here", args), !can || head, func() { v.openRebase(t) })
		g.item(v.textf("ref_rebase_interactive_here", args), !can || head || v.git.Host != nil, func() { v.openInteractiveRebase(commit.Hash) })
	}
	g.group()
	g.item(v.textf("ref_reset_here", args), !can || head, func() { v.openReset(t) })
	g.group()
	g.item(v.text("ref_checkout_commit"), !can || head, func() { v.openRefAction(opCheckout, t) })
	g.item(v.text("ref_cherry_pick"), !can, func() { v.openCherryPick(commit) })
	g.item(v.text("ref_revert"), !can, func() { v.openRevert(commit) })
	g.item(v.text("ref_save_patch"), !can, func() { v.saveCommitPatch(commit) })
	v.compareItems(g, commit)
	g.group()
	g.item(v.text("ref_copy_commit"), false, func() { cx.SetClipboard(commit.Hash) })
	g.item(v.text("ref_copy_info"), false, func() {
		info := commit.Short + " - " + commit.Subject
		if commit.Body != "" {
			info += "\n\n" + commit.Body
		}
		cx.SetClipboard(info)
	})
	return g.items
}

// tagPushItems add pushing a tag to the repository's remotes, a single one
// directly and several in a submenu.
func (v *View) tagPushItems(g *menuGroups, tag string) {
	remotes := slices.Sorted(maps.Keys(v.repo.github))
	can := v.CanSync()
	switch len(remotes) {
	case 0:
		return
	case 1:
		name := remotes[0]
		g.item(v.textf("ref_push_tag", map[string]any{"Remote": name}), !can, func() { v.openPushTag(tag, name) })
	default:
		var items []core.MenuItem
		for _, name := range remotes {
			items = append(items, core.MenuItem{Label: v.textf("ref_push_tag", map[string]any{"Remote": name}), Disabled: !can,
				OnSelect: func() { v.openPushTag(tag, name) }})
		}
		g.submenu(v.text("ref_push_tag_multiple"), false, items)
	}
}

// remoteItems add a local branch's fast-forward to its upstream, possible
// when it lacks commits the upstream has and has none the upstream lacks,
// its pull while it is checked out, and its push to its upstream's remote
// or the fallback remote.
func (v *View) remoteItems(g *menuGroups, n *refNode) {
	can := v.CanSync()
	if up := n.upstream; up.Remote != "" {
		name := up.Remote + "/" + up.Branch
		g.item(v.textf("ref_fast_forward", map[string]any{"Upstream": name}), !can || n.behind == 0 || n.ahead > 0, func() {
			worktree := ""
			if i := slices.IndexFunc(v.repo.checkouts, func(c Checkout) bool { return c.Branch == n.path }); i >= 0 {
				worktree = v.repo.checkouts[i].Path
			}
			branch, rev := n.path, "refs/remotes/"+name
			v.Run(opFastForward, v.text("ref_fast_forward_failed"), func(git Git, path string) error {
				return git.FastForward(path, worktree, branch, rev)
			})
		})
		if n.path == v.tracking.Branch {
			g.item(v.textf("ref_pull_upstream", map[string]any{"Upstream": name}), !v.CanPull(), func() { v.openPullBranch(up) })
		}
	}
	remote := n.upstream.Remote
	if remote == "" {
		names := slices.Sorted(maps.Keys(v.repo.github))
		if remote = (Remotes{Names: names}).DefaultRemote(); remote == "" && len(names) > 0 {
			remote = names[0]
		}
	}
	if remote != "" {
		g.item(v.textf("ref_push", map[string]any{"Remote": remote}), !can, func() { v.openPush(n.path) })
	}
}

// trackingMenu chooses the remote branch a local branch tracks, or none.
func (v *View) trackingMenu(g *menuGroups, n *refNode) {
	can := v.CanSync()
	current := ""
	if n.upstream.Remote != "" {
		current = n.upstream.Remote + "/" + n.upstream.Branch
	}
	branch := n.path
	track := func(upstream string) {
		v.Run(opTrack, v.text("ref_tracking_failed"), func(git Git, path string) error {
			return git.SetUpstream(path, branch, upstream)
		})
	}
	var items []core.MenuItem
	for _, r := range refLeaves(v.repo.refTrees.remotes) {
		items = append(items, core.MenuItem{Label: r.path, Checkable: true, Checked: r.path == current, Disabled: !can, OnSelect: func() {
			if r.path != current {
				track(r.path)
			}
		}})
	}
	items = append(items, core.MenuItem{Separator: true}, core.MenuItem{Label: v.text("ref_tracking_none"), Checkable: true, Checked: current == "",
		Disabled: !can, OnSelect: func() {
			if current != "" {
				track("")
			}
		}})
	g.submenu(v.text("ref_tracking"), false, items)
}

// checkoutTarget is what checking out t switches to: the local branch
// tracking a remote branch when there is one, else t.
func (v *View) checkoutTarget(t refTarget) refTarget {
	if t.group != "remotes" {
		return t
	}
	for _, n := range refLeaves(v.repo.refTrees.branches) {
		if n.upstream.Remote != "" && n.upstream.Remote+"/"+n.upstream.Branch == t.name {
			return nodeTarget("branches", n)
		}
	}
	return t
}

// canCheckout reports whether checking out t changes the worktree: a local
// branch no worktree has checked out, or a tag or commit other than the
// detached HEAD's.
func (v *View) canCheckout(t refTarget) bool {
	switch t.group {
	case "branches":
		return !v.checkedOut(t.name)
	case "remotes":
		return true
	}
	return v.tracking.Branch != "" || t.hash != v.repo.head
}

// openCheckout checks out t, as a double click does: a local branch
// another worktree has checked out opens that worktree, one switches at once
// when the worktree was read without changes, and anything else asks first.
// It does nothing when that would change nothing or a write may not start.
func (v *View) openCheckout(t refTarget) {
	t = v.checkoutTarget(t)
	if path := v.otherCheckout(t); path != "" && v.opts.OpenWorktree != nil {
		v.opts.OpenWorktree(path)
		return
	}
	if !v.CanSync() || !v.canCheckout(t) {
		return
	}
	if t.group == "branches" && v.current.loaded && !v.hasCurrentChanges() {
		v.switchBranch(t.name)
		return
	}
	v.openRefAction(opCheckout, t)
}

// otherCheckout is the path of the worktree other than the shown one that
// has the local branch t checked out, or empty.
func (v *View) otherCheckout(t refTarget) string {
	if t.group != "branches" {
		return ""
	}
	for _, c := range v.repo.checkouts {
		if c.Branch == t.name && filepath.Clean(filepath.FromSlash(c.Path)) != filepath.Clean(v.path) {
			return c.Path
		}
	}
	return ""
}

// switchBranch checks out a local branch in the worktree.
func (v *View) switchBranch(branch string) {
	v.Run(OpSwitch, v.text("switch_failed"), func(git Git, path string) error {
		return git.Checkout(path, CheckoutOptions{Branch: branch})
	})
}

// openRefAction opens the dialog of op on t, suggesting a new branch named
// after a remote branch and a worktree beside the main one named after its
// branch. A rebase uses its own dialog, which reads what it moves first.
func (v *View) openRefAction(op string, t refTarget) {
	if op == opRebase {
		v.openRebase(t)
		return
	}
	a := refAction{open: true, op: op, target: t, checkout: true}
	if op == opMerge {
		a.merge = mergeChoice(v.state.MergeChoice)
		if a.merge < mergeDefault || a.merge > mergeNoCommit {
			a.merge = mergeDefault
		}
	}
	if t.group == "remotes" && (op == opCheckout || op == OpWorktree) {
		if remote := refRemote(v.repo.github, t.name); remote != "" {
			a.name = t.name[len(remote)+1:]
		} else {
			_, a.name, _ = strings.Cut(t.name, "/")
		}
	}
	if op == opRename {
		a.name = t.name
	}
	if op == OpWorktree {
		a.suggested = v.suggestedPath(t, a.name)
		a.path = a.suggested
	}
	v.action = a
	v.invalidate()
}

// suggestedPath is a directory for a new worktree checking out t, or a new
// branch name at it, beside the main worktree and named after both.
func (v *View) suggestedPath(t refTarget, name string) string {
	main := v.path
	if len(v.repo.checkouts) > 0 {
		main = v.repo.checkouts[0].Path
	}
	if t.group == "branches" || name == "" {
		name = t.name
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\: `, r) {
			return '-'
		}
		return r
	}, name)
	return filepath.Join(filepath.Dir(main), filepath.Base(main)+"-"+name)
}

// actionReady reports whether the dialog's write can run as filled in,
// with the problem to show when a name is taken.
func (v *View) actionReady() (ready bool, problem string) {
	a := v.action
	name := strings.TrimSpace(a.name)
	switch {
	case (a.op == opBranch || a.op == opTag || a.op == opRename) && name == "", a.op == OpWorktree && strings.TrimSpace(a.path) == "",
		a.op == opRename && name == a.target.name:
		return false, ""
	case a.op == opTag && hasRef(v.repo.refTrees.tags, name):
		return false, v.textf("ref_tag_exists", map[string]any{"Name": name})
	case a.op != opTag && name != "" && hasRef(v.repo.refTrees.branches, name):
		return false, v.textf("ref_branch_exists", map[string]any{"Name": name})
	}
	return true, ""
}

// refActionDialog asks for what a refs menu's write needs before running
// it.
func (v *View) refActionDialog(cx *core.Scope) core.Node {
	a := &v.action
	args := func() map[string]any { return map[string]any{"Name": a.target.name, "Current": v.checkedOutName()} }
	// A checkout's text depends on what it checks out.
	key := func() string {
		k := "ref_" + a.op
		if a.op == opCheckout {
			k += "_" + a.target.group
		}
		return k
	}
	ready := func() bool {
		ok, _ := v.actionReady()
		return ok && v.CanSync()
	}
	run := func() {
		a.open = false
		v.runRefAction()
	}
	return v.dialogNode(cx, dialogSpec{open: &a.open, width: 440, testID: "ref-action-dialog",
		title:       func() string { return v.textf(key()+"_title", args()) },
		description: func() string { return v.textf(key()+"_description", args()) },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			field := func(label string, value *string, placeholder string, focus bool, changed func()) core.Node {
				input := v.textField(value, label, placeholder, focus, func() {
					if ready() {
						run()
					}
				}, "field-"+label)
				if changed != nil {
					in := input.(ui.Input)
					in.OnChange = func(s string) {
						*value = s
						changed()
						v.edited()
					}
					input = in
				}
				return formField(cx, refLabelWidth, label, nil, input)
			}
			var nodes []core.Node
			switch a.op {
			case opCheckout:
				nodes = append(nodes, formField(cx, refLabelWidth, v.text("ref_checkout_target_"+a.target.group), nil, v.targetChoice(cx, a.target)))
				if a.target.group == "remotes" {
					nodes = append(nodes, field(v.text("ref_branch_name"), &a.name, v.text("ref_detached"), true, nil))
				}
				nodes = append(nodes, formField(cx, refLabelWidth, v.text("ref_local_changes"), nil, radios(v, &a.changes, v.text("ref_local_changes"), []radioChoice[LocalChanges]{
					{KeepChanges, v.text("ref_keep_changes")}, {StashChanges, v.text("ref_stash_changes")}, {DiscardChanges, v.text("ref_discard_changes")},
				}, nil)))
			case OpWorktree:
				if a.target.group == "branches" {
					nodes = append(nodes, formField(cx, refLabelWidth, v.text("sync_branch"), nil, branchChoice(cx, a.target.name)))
				} else {
					// The suggested directory follows the name until edited.
					nodes = append(nodes, field(v.text("ref_branch_name"), &a.name, v.text("ref_detached"), true, func() {
						if a.path == a.suggested {
							a.suggested = v.suggestedPath(a.target, a.name)
							a.path = a.suggested
						}
					}))
				}
				nodes = append(nodes, field(v.text("ref_path"), &a.path, "", a.target.group == "branches", nil))
			case opRename:
				nodes = append(nodes, field(v.text("ref_branch_name"), &a.name, "", true, nil))
			case opBranch:
				nodes = append(nodes, field(v.text("ref_branch_name"), &a.name, "", true, nil),
					v.formOption(refLabelWidth, &a.checkout, v.text("ref_checkout_created"), nil))
			case opTag:
				nodes = append(nodes, field(v.text("ref_tag_name"), &a.name, "", true, nil),
					field(v.text("ref_tag_message"), &a.message, v.text("ref_tag_lightweight"), false, nil))
			case opMerge:
				nodes = append(nodes, formField(cx, refLabelWidth, v.text("ref_merge_type"), nil, radios(v, &a.merge, v.text("ref_merge_type"), []radioChoice[mergeChoice]{
					{mergeDefault, v.text("ref_merge_default")}, {mergeNoFF, v.text("ref_no_ff")},
					{mergeSquash, v.text("ref_merge_squash")}, {mergeNoCommit, v.text("ref_merge_no_commit")},
				}, func() {
					v.state.MergeChoice = int(a.merge)
					v.saveState()
				})), v.formOption(refLabelWidth, &a.options.Stash, v.text("pull_stash"), nil))
			}
			problem := core.Text{ContentFunc: func() string {
				v.watch()
				_, p := v.actionReady()
				return p
			}, Style: core.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Danger)}
			return append(nodes, problem)
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&a.open), v.confirmButton(v.text("ref_"+a.op+"_confirm"), ready, run, "confirm")}
		},
		confirm: func() {
			if ready() {
				run()
			}
		}})
}

// runRefAction runs the dialog's write.
func (v *View) runRefAction() {
	a := v.action
	t, name := a.target, strings.TrimSpace(a.name)
	checkout := CheckoutOptions{Branch: name, Start: t.rev(), Track: t.group == "remotes" && name != ""}
	switch a.op {
	case opCheckout:
		op := opCheckout
		switch t.group {
		case "branches":
			op, checkout = OpSwitch, CheckoutOptions{Branch: t.name}
		case "tags", "commit":
			checkout = CheckoutOptions{Start: t.rev()}
		}
		checkout.Changes = a.changes
		v.Run(op, v.text("ref_checkout_failed"), func(git Git, path string) error { return git.Checkout(path, checkout) })
	case OpWorktree:
		if t.group == "branches" {
			checkout = CheckoutOptions{Branch: t.name}
		}
		dir := strings.TrimSpace(a.path)
		v.Run(OpWorktree, v.text("ref_worktree_failed"), func(git Git, path string) error { return git.AddWorktree(path, dir, checkout) })
	case opMerge:
		o := a.options
		o.NoFastForward = a.merge == mergeNoFF
		o.Squash = a.merge == mergeSquash
		o.NoCommit = a.merge == mergeNoCommit
		v.Run(opMerge, v.text("ref_merge_failed"), func(git Git, path string) error { return git.Merge(path, t.rev(), o) })
	case opBranch:
		v.Run(opBranch, v.text("ref_branch_failed"), func(git Git, path string) error {
			if a.checkout {
				return git.Checkout(path, CheckoutOptions{Branch: name, Start: t.rev()})
			}
			return git.CreateBranch(path, name, t.rev())
		})
	case opRename:
		v.Run(opRename, v.text("ref_rename_failed"), func(git Git, path string) error { return git.RenameBranch(path, t.name, name) })
	case opTag:
		v.Run(opTag, v.text("ref_tag_failed"), func(git Git, path string) error { return git.CreateTag(path, name, t.rev(), a.message) })
	}
}

// targetChoice shows what a dialog acts on: a commit's short hash and
// subject, or a ref's name with its group's mark.
func (v *View) targetChoice(cx *core.Scope, t refTarget) core.Node {
	colors := ui.ColorsOf(cx)
	icon := lucide.GitBranch
	switch t.group {
	case "commit":
		hash := line(t.name, 13, ui.Regular, colors.Muted)
		hash.Style.FlexShrink = 0
		return row(8, ui.Icon(lucide.GitCommitVertical, 14, colors.Muted), hash, line(t.subject, 13, ui.Regular, colors.Text))
	case "remotes":
		icon = remoteIcon(v.repo.github, refRemote(v.repo.github, t.name))
	case "tags":
		icon = lucide.Tag
	}
	return choice(cx, icon, t.name)
}
