package gitview

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
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

// menuGroups adds a menu's items in groups, drawing a line between groups
// that show items, so a group with every item left out leaves no line.
type menuGroups struct {
	m           *ui.Menu
	shown, line bool
}

// group starts the next group.
func (g *menuGroups) group() { g.line = g.shown }

func (g *menuGroups) separate() {
	if g.line {
		g.m.Separator()
		g.line = false
	}
	g.shown = true
}

func (g *menuGroups) item(label string) *ui.MenuItem {
	g.separate()
	return g.m.Item(label)
}

func (g *menuGroups) submenu(label string, build func(m *ui.Menu)) *ui.MenuItem {
	g.separate()
	return g.m.Submenu(label, build)
}

// refMenu fills the menu of a ref: checking it out, here or in a new
// worktree, moving a local branch to its upstream or pushing it, pulling a
// remote branch or the checked out branch's upstream, merging it or
// rebasing onto it, pushing a tag, branching or tagging from it, a local
// branch's upstream and name, deleting a ref and copying its name. Items
// that cannot apply to the target are left out; writes are disabled while
// one runs or host work does.
func (v *View) refMenu(c *ui.Context, m *ui.Menu, t refTarget, n *refNode) {
	can := v.CanSync()
	args := map[string]any{"Name": t.name, "Current": v.checkedOutName()}
	local := t.group == "branches"
	g := &menuGroups{m: m}
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
		if g.item(v.textf("ref_open_worktree", map[string]any{"Worktree": filepath.Base(other)})).Chosen() {
			v.opts.OpenWorktree(other)
		}
	} else {
		if v.canCheckout(checkout) && g.item(label).Disabled(!can).Chosen() {
			v.openRefAction(opCheckout, checkout)
		}
		if !v.checkedOut(checkout.name) && g.item(v.text("ref_worktree")).Disabled(!can).Chosen() {
			v.openRefAction(OpWorktree, t)
		}
	}
	g.group()
	if local && n != nil {
		v.remoteItems(g, n)
	}
	if t.group == "remotes" && v.tracking.Branch != "" && g.item(v.textf("ref_pull", args)).Disabled(!v.CanPull()).Chosen() {
		v.openPull(t.name)
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
		if g.item(v.textf("ref_merge", args)).Disabled(!can || same).Chosen() && !same {
			v.openRefAction(opMerge, t)
		}
		if v.tracking.Branch != "" {
			if g.item(v.textf("ref_rebase", args)).Disabled(!can || same).Chosen() && !same {
				v.openRebase(t)
			}
		}
	}
	g.group()
	if g.item(v.text("ref_new_branch")).Disabled(!can).Chosen() {
		v.openRefAction(opBranch, t)
	}
	if g.item(v.text("ref_new_tag")).Disabled(!can).Chosen() {
		v.openRefAction(opTag, t)
	}
	g.group()
	if n != nil {
		if local {
			v.trackingMenu(g, n)
			if g.item(v.text("ref_rename")).Disabled(!can).Chosen() {
				v.openRefAction(opRename, t)
			}
		}
		if d, ok := v.newRefDeletion(t.group, n); ok && g.item(v.text("delete_ref")).Disabled(!can).Chosen() {
			v.deletion = d
		}
	}
	g.group()
	if g.item(v.text("ref_copy_" + t.group)).Chosen() {
		c.WriteClipboard(t.name)
	}
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
func (v *View) commitRefsItems(c *ui.Context, g *menuGroups, commit Commit) {
	tags := false
	for _, r := range v.refsAt(commit.Hash) {
		if r.group == "tags" && !tags {
			tags = true
			g.group()
		}
		t, n := nodeTarget(r.group, r.node), r.node
		g.submenu(n.path, func(m *ui.Menu) { v.refMenu(c, m, t, n) })
	}
}

// commitMenu fills a history row's menu: the menus of the branches, remote
// branches and tags pointing at it, checking the commit out, merging it into
// or rebasing the checked out branch to it, branching or tagging from it,
// resetting the branch or HEAD to it, cherry-picking or reverting it, saving
// its patch and copying its hash or summary. A detached HEAD has no branch
// to merge or rebase, but resets.
func (v *View) commitMenu(c *ui.Context, m *ui.Menu, commit Commit) {
	t := commitTarget(commit)
	can := v.CanSync()
	checkedOut := v.tracking.Branch != ""
	args := map[string]any{"Name": commit.Short, "Current": v.checkedOutName()}
	g := &menuGroups{m: m}
	// A commit a ref points at leads with that ref's menu, as Fork does, so
	// its branch, remote branch or tag acts from the history row too.
	v.commitRefsItems(c, g, commit)
	g.group()
	if g.item(v.text("ref_new_branch")).Disabled(!can).Chosen() {
		v.openRefAction(opBranch, t)
	}
	if g.item(v.text("ref_new_tag")).Disabled(!can).Chosen() {
		v.openRefAction(opTag, t)
	}
	g.group()
	if checkedOut {
		if g.item(v.textf("ref_merge_here", args)).Disabled(!can || commit.Hash == v.repo.head).Chosen() && commit.Hash != v.repo.head {
			v.openRefAction(opMerge, t)
		}
		if g.item(v.textf("ref_rebase_here", args)).Disabled(!can || commit.Hash == v.repo.head).Chosen() && commit.Hash != v.repo.head {
			v.openRebase(t)
		}
	}
	g.group()
	if g.item(v.textf("ref_reset_here", args)).Disabled(!can || commit.Hash == v.repo.head).Chosen() && commit.Hash != v.repo.head {
		v.openReset(t)
	}
	g.group()
	if g.item(v.text("ref_checkout_commit")).Disabled(!can || commit.Hash == v.repo.head).Chosen() && commit.Hash != v.repo.head {
		v.openRefAction(opCheckout, t)
	}
	if g.item(v.text("ref_cherry_pick")).Disabled(!can).Chosen() {
		v.openCherryPick(commit)
	}
	if g.item(v.text("ref_revert")).Disabled(!can).Chosen() {
		v.openRevert(commit)
	}
	if g.item(v.text("ref_save_patch")).Disabled(!can).Chosen() {
		v.saveCommitPatch(commit)
	}
	g.group()
	if g.item(v.text("ref_copy_commit")).Chosen() {
		c.WriteClipboard(commit.Hash)
	}
	if g.item(v.text("ref_copy_info")).Chosen() {
		info := commit.Short + " - " + commit.Subject
		if commit.Body != "" {
			info += "\n\n" + commit.Body
		}
		c.WriteClipboard(info)
	}
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
		if g.item(v.textf("ref_push_tag", map[string]any{"Remote": name})).Disabled(!can).Chosen() {
			v.openPushTag(tag, name)
		}
	default:
		g.submenu(v.text("ref_push_tag_multiple"), func(m *ui.Menu) {
			for _, name := range remotes {
				if m.Item(v.textf("ref_push_tag", map[string]any{"Remote": name})).Disabled(!can).Chosen() {
					v.openPushTag(tag, name)
				}
			}
		})
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
		ff := g.item(v.textf("ref_fast_forward", map[string]any{"Upstream": name}))
		if ff.Disabled(!can || n.behind == 0 || n.ahead > 0).Chosen() {
			worktree := ""
			if i := slices.IndexFunc(v.repo.checkouts, func(c Checkout) bool { return c.Branch == n.path }); i >= 0 {
				worktree = v.repo.checkouts[i].Path
			}
			branch, rev := n.path, "refs/remotes/"+name
			v.Run(opFastForward, v.text("ref_fast_forward_failed"), func(git Git, path string) error {
				return git.FastForward(path, worktree, branch, rev)
			})
		}
		if n.path == v.tracking.Branch {
			if g.item(v.textf("ref_pull_upstream", map[string]any{"Upstream": name})).Disabled(!v.CanPull()).Chosen() {
				v.openPullBranch(up)
			}
		}
	}
	remote := n.upstream.Remote
	if remote == "" {
		names := slices.Sorted(maps.Keys(v.repo.github))
		if remote = (Remotes{Names: names}).DefaultRemote(); remote == "" && len(names) > 0 {
			remote = names[0]
		}
	}
	if remote != "" && g.item(v.textf("ref_push", map[string]any{"Remote": remote})).Disabled(!can).Chosen() {
		v.openPush(n.path)
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
	g.submenu(v.text("ref_tracking"), func(m *ui.Menu) {
		for _, r := range refLeaves(v.repo.refTrees.remotes) {
			if m.Item(r.path).Checked(r.path == current).Disabled(!can).Chosen() && r.path != current {
				track(r.path)
			}
		}
		m.Separator()
		if m.Item(v.text("ref_tracking_none")).Checked(current == "").Disabled(!can).Chosen() && current != "" {
			track("")
		}
	})
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

// refLabelWidth is the width of the refs dialogs' labels in theme spaces.
const refLabelWidth = 34

// refActionDialog asks for what a refs menu's write needs before running
// it.
func (v *View) refActionDialog(c *ui.Context) {
	a := &v.action
	args := map[string]any{"Name": a.target.name, "Current": v.checkedOutName()}
	// A checkout's text depends on what it checks out.
	key := "ref_" + a.op
	if a.op == opCheckout {
		key += "_" + a.target.group
	}
	title := v.textf(key+"_title", args)
	components.Dialog(c, &a.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(t.Space(110))
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			components.Text(c, v.textf(key+"_description", args)).Wrap().FillWidth().TextColor(t.TextMuted)
		})
		field := func(label string, value *string, placeholder string, focus bool) {
			formField(c, refLabelWidth, label, false, func() {
				box := components.TextBox(c, value, components.TextBoxOptions{}).FillWidth().Label(label).Placeholder(placeholder)
				if focus {
					box.AutoFocus()
				}
			})
		}
		switch a.op {
		case opCheckout:
			formField(c, refLabelWidth, v.text("ref_checkout_target_"+a.target.group), false, func() { v.targetChoice(c, a.target) })
			if a.target.group == "remotes" {
				field(v.text("ref_branch_name"), &a.name, v.text("ref_detached"), true)
			}
			formField(c, refLabelWidth, v.text("ref_local_changes"), false, func() {
				ui.Column(c).Gap(t.Space(2)).Children(func() {
					components.Radio(c, &a.changes, KeepChanges, v.text("ref_keep_changes"))
					components.Radio(c, &a.changes, StashChanges, v.text("ref_stash_changes"))
					components.Radio(c, &a.changes, DiscardChanges, v.text("ref_discard_changes"))
				})
			})
		case OpWorktree:
			before := a.name
			if a.target.group == "branches" {
				formField(c, refLabelWidth, v.text("sync_branch"), false, func() { branchChoice(c, a.target.name) })
			} else {
				field(v.text("ref_branch_name"), &a.name, v.text("ref_detached"), true)
			}
			// The suggested directory follows the name until edited.
			if a.name != before && a.path == a.suggested {
				a.suggested = v.suggestedPath(a.target, a.name)
				a.path = a.suggested
			}
			field(v.text("ref_path"), &a.path, "", a.target.group == "branches")
		case opRename:
			field(v.text("ref_branch_name"), &a.name, "", true)
		case opBranch:
			field(v.text("ref_branch_name"), &a.name, "", true)
			formOption(c, refLabelWidth, &a.checkout, v.text("ref_checkout_created"))
		case opTag:
			field(v.text("ref_tag_name"), &a.name, "", true)
			field(v.text("ref_tag_message"), &a.message, v.text("ref_tag_lightweight"), false)
		case opMerge:
			formOption(c, refLabelWidth, &a.options.NoFastForward, v.text("ref_no_ff"))
			formOption(c, refLabelWidth, &a.options.Stash, v.text("pull_stash"))
		}
		ready, problem := v.actionReady()
		if problem != "" {
			components.Text(c, problem).Wrap().FillWidth().TextColor(t.Danger)
		}
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).Clicked() {
				a.open = false
			}
			confirm := components.Button(c, v.text("ref_"+a.op+"_confirm"), components.ButtonPrimary).Disabled(!ready || !v.CanSync())
			if a.op == opMerge || a.op == opCheckout && a.target.group != "remotes" {
				confirm.AutoFocus()
			}
			if confirm.Clicked() {
				a.open = false
				v.runRefAction()
			}
		})
	})
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
		v.Run(opMerge, v.text("ref_merge_failed"), func(git Git, path string) error { return git.Merge(path, t.rev(), a.options) })
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
func (v *View) targetChoice(c *ui.Context, t refTarget) {
	th := c.Theme()
	icon := lucide.GitBranch
	switch t.group {
	case "commit":
		ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(th.Space(2)).Children(func() {
			components.Icon(c, lucide.GitCommitVertical).Size(14, 14).Shrink(0).TextColor(th.TextMuted)
			components.Text(c, t.name).Shrink(0).TextColor(th.TextMuted).SingleLine()
			components.Text(c, t.subject).MinWidth(0).SingleLine().Ellipsis("…")
		})
		return
	case "remotes":
		icon = remoteIcon(v.repo.github, refRemote(v.repo.github, t.name))
	case "tags":
		icon = lucide.Tag
	}
	syncChoice(c, icon, t.name)
}
