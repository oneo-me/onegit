package gitview

import (
	"cmp"
	"slices"
	"strings"
)

// refTrees arrange the refs pane's branches, remote branches and tags by the
// folders of their names.
type refTrees struct {
	branches, remotes, tags []*refNode
}

func newRefTrees(refs Refs) refTrees {
	return refTrees{newRefTree(refs.Branches), newRefTree(refs.Remotes), newRefTree(refs.Tags)}
}

// refNode is a ref, or a folder of refs when children is not nil. path is
// the ref's name or the folder's prefix without its trailing slash, and name
// its last part. upstream is the remote branch a local branch tracks.
type refNode struct {
	name, path, hash string
	upstream         RemoteBranch
	children         []*refNode
}

// newRefTree arranges refs by the slash-separated folders of their names,
// each level listing folders before refs. A remote's name becomes the folder
// of its branches.
func newRefTree(refs []Ref) []*refNode {
	root := &refNode{children: []*refNode{}}
	folders := map[string]*refNode{"": root}
	for _, ref := range refs {
		parent := root
		for i := range len(ref.Name) {
			if ref.Name[i] != '/' {
				continue
			}
			path := ref.Name[:i]
			folder := folders[path]
			if folder == nil {
				folder = &refNode{name: path[strings.LastIndex(path, "/")+1:], path: path, children: []*refNode{}}
				folders[path] = folder
				parent.children = append(parent.children, folder)
			}
			parent = folder
		}
		parent.children = append(parent.children, &refNode{name: ref.Name[strings.LastIndex(ref.Name, "/")+1:], path: ref.Name, hash: ref.Hash, upstream: ref.Upstream})
	}
	for _, folder := range folders {
		slices.SortStableFunc(folder.children, func(a, b *refNode) int {
			if fa, fb := a.children != nil, b.children != nil; fa != fb {
				if fa {
					return -1
				}
				return 1
			}
			return cmp.Compare(a.name, b.name)
		})
	}
	return root.children
}

// filterRefs keeps the refs whose name contains query, ignoring case, and
// the folders leading to them. An empty query keeps every ref.
func filterRefs(nodes []*refNode, query string) []*refNode {
	query = strings.ToLower(query)
	if query == "" {
		return nodes
	}
	var kept []*refNode
	for _, n := range nodes {
		if n.children == nil {
			if strings.Contains(strings.ToLower(n.path), query) {
				kept = append(kept, n)
			}
			continue
		}
		if children := filterRefs(n.children, query); len(children) > 0 {
			folder := *n
			folder.children = children
			kept = append(kept, &folder)
		}
	}
	return kept
}
