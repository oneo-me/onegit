package gitview

import (
	"cmp"
	"slices"
)

// changeTree arranges changed files by directory. A file's key is its path; a
// directory's is its path with a trailing slash, so the two never collide. A
// directory with a single subdirectory and no files merges into one row, as
// in VS Code's compact folders. Staged and unstaged files group under a row
// of their area, whose key prefixes theirs (fileKey).
type changeTree struct {
	roots []string
	nodes map[string]*changeNode
}

type changeNode struct {
	// name is the row's label: a file's name or a directory's path below
	// its parent row.
	name string
	// children is nil for a file and lists a directory's rows otherwise.
	children []string
	file     FileChange
	// area is set on the row grouping an area's files.
	area Area
}

// areaKeys are the keys of the rows grouping each area's files, which end
// in a slash like a directory's but start with a byte no path does.
var areaKeys = map[Area]string{AreaStaged: "\x00staged/", AreaUnstaged: "\x00unstaged/"}

// fileKey is the tree key of a file's row.
func fileKey(f FileChange) string { return areaKeys[f.Area] + f.Path }

func newChangeTree(files []FileChange) changeTree {
	t := changeTree{nodes: map[string]*changeNode{}}
	add := func(parent, key string) {
		if parent == "" {
			t.roots = append(t.roots, key)
		} else {
			t.nodes[parent].children = append(t.nodes[parent].children, key)
		}
	}
	for _, f := range files {
		parent := areaKeys[f.Area]
		if parent != "" && t.nodes[parent] == nil {
			t.nodes[parent] = &changeNode{children: []string{}, area: f.Area}
			add("", parent)
		}
		key := fileKey(f)
		for i := len(parent); i < len(key); i++ {
			if key[i] != '/' {
				continue
			}
			dir := key[:i+1]
			if t.nodes[dir] == nil {
				t.nodes[dir] = &changeNode{name: key[len(parent):i], children: []string{}}
				add(parent, dir)
			}
			parent = dir
		}
		t.nodes[key] = &changeNode{name: key[len(parent):], file: f}
		add(parent, key)
	}
	if len(t.roots) > 0 && t.nodes[t.roots[0]].area != AreaAll {
		// The staged files come last, above the commit's message.
		slices.SortFunc(t.roots, func(a, b string) int { return cmp.Compare(t.nodes[b].area, t.nodes[a].area) })
		for _, key := range t.roots {
			n := t.nodes[key]
			n.children = t.compact(n.children)
		}
		return t
	}
	t.roots = t.compact(t.roots)
	return t
}

// compact merges single-directory chains and sorts directories before files.
func (t changeTree) compact(keys []string) []string {
	for i, key := range keys {
		n := t.nodes[key]
		if n.children == nil {
			continue
		}
		for len(n.children) == 1 && t.nodes[n.children[0]].children != nil {
			child := n.children[0]
			c := t.nodes[child]
			delete(t.nodes, key)
			c.name = n.name + "/" + c.name
			key, n = child, c
		}
		keys[i] = key
		n.children = t.compact(n.children)
	}
	slices.SortFunc(keys, func(a, b string) int {
		da, db := t.nodes[a].children != nil, t.nodes[b].children != nil
		if da != db {
			if da {
				return -1
			}
			return 1
		}
		return cmp.Compare(t.nodes[a].name, t.nodes[b].name)
	})
	return keys
}

// children returns a directory's rows, or nil for a file.
func (t changeTree) children(key string) []string {
	if n := t.nodes[key]; n != nil {
		return n.children
	}
	return nil
}

// directories lists every directory row's key.
func (t changeTree) directories() []string {
	var keys []string
	for key, n := range t.nodes {
		if n.children != nil {
			keys = append(keys, key)
		}
	}
	return keys
}

// ancestors lists the directory rows containing path, outermost first.
func (t changeTree) ancestors(path string) []string {
	var keys []string
	for i := range len(path) {
		if path[i] == '/' {
			if n := t.nodes[path[:i+1]]; n != nil && n.children != nil {
				keys = append(keys, path[:i+1])
			}
		}
	}
	return keys
}

// row returns key's index among the rows open shows, or -1 when hidden.
func (t changeTree) row(key string, open func(string) bool) int {
	index := 0
	var walk func([]string) bool
	walk = func(keys []string) bool {
		for _, k := range keys {
			if k == key {
				return true
			}
			index++
			if n := t.nodes[k]; n.children != nil && open(k) && walk(n.children) {
				return true
			}
		}
		return false
	}
	if walk(t.roots) {
		return index
	}
	return -1
}
