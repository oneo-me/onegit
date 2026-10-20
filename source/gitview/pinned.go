package gitview

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// pinnedRef is a ref the user pinned in the refs pane.
type pinnedRef struct {
	group, name string
	node        *refNode
}

// pinnedRoot keys the pinned refs: the repository's main worktree, so all
// of its worktrees share them.
func (v *View) pinnedRoot() string {
	if v.repo.root != "" {
		return v.repo.root
	}
	return v.path
}

// findRefNode finds the ref named name in a ref tree.
func findRefNode(nodes []*refNode, name string) *refNode {
	for _, n := range nodes {
		if n.children != nil {
			if found := findRefNode(n.children, name); found != nil {
				return found
			}
			continue
		}
		if n.path == name {
			return n
		}
	}
	return nil
}

// refTreeFor returns the ref tree of a pinned ref's group.
func (v *View) refTreeFor(group string) []*refNode {
	switch group {
	case "branches":
		return v.repo.refTrees.branches
	case "remotes":
		return v.repo.refTrees.remotes
	case "tags":
		return v.repo.refTrees.tags
	}
	return nil
}

// pinnedRefs lists the pinned refs the repository still has.
func (v *View) pinnedRefs() []pinnedRef {
	var refs []pinnedRef
	for _, key := range v.state.Pinned[v.pinnedRoot()] {
		group, name, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		if n := findRefNode(v.refTreeFor(group), name); n != nil {
			refs = append(refs, pinnedRef{group: group, name: name, node: n})
		}
	}
	return refs
}

// isPinned reports whether a ref is pinned.
func (v *View) isPinned(group, name string) bool {
	for _, ref := range v.pinnedRefs() {
		if ref.group == group && ref.name == name {
			return true
		}
	}
	return false
}

// togglePinned pins a ref, or unpins it, and saves the change.
func (v *View) togglePinned(group, name string) {
	root := v.pinnedRoot()
	if root == "" {
		return
	}
	key := group + "/" + name
	refs := append([]string(nil), v.state.Pinned[root]...)
	found := false
	for i, ref := range refs {
		if ref == key {
			refs = append(refs[:i], refs[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		refs = append(refs, key)
	}
	pinned := map[string][]string{}
	for r, list := range v.state.Pinned {
		pinned[r] = list
	}
	if len(refs) == 0 {
		delete(pinned, root)
	} else {
		pinned[root] = refs
	}
	v.state.Pinned = pinned
	v.saveState()
	v.invalidate()
}

// pinnedRows lists the pinned refs, filtered by query. A pinned ref acts as
// its refs pane row does: clicking selects its commit and double-clicking
// checks it out.
func (v *View) pinnedRows(c *ui.Context, query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	for _, ref := range v.pinnedRefs() {
		if query != "" && !strings.Contains(strings.ToLower(ref.name), query) {
			continue
		}
		n := ref.node
		id := ref.group + "/" + ref.name
		ui.Column(c).Key("pinned-" + id).FillWidth().Shrink(0).Children(func() {
			row := components.TreeItem(c, ref.name, nil, v.section == sectionCommits && v.ref == id, nil).Height(treeRow(c))
			if v.currentRef(ref.group, n) {
				row.Bold()
			}
			row.ContextMenu(func(m *ui.Menu) { v.refMenu(c, m, nodeTarget(ref.group, n), n) })
			if row.Clicked() {
				v.revealCommit(n.hash)
				v.ref = id
			}
			if row.DoubleClicked() {
				v.openCheckout(nodeTarget(ref.group, n))
			}
		})
	}
}
