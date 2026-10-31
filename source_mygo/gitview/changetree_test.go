package gitview

import (
	"slices"
	"testing"
)

func TestChangeTreeGroupsFilesByDirectory(t *testing.T) {
	var files []FileChange
	for _, path := range []string{"README.md", "a/b/c/x.go", "a/b/c/y.go", "a/z.go", "docs/guide.md", "go.mod"} {
		files = append(files, FileChange{Path: path, OldPath: path, Status: 'M'})
	}
	tree := newChangeTree(files)

	var rows []string
	var walk func([]string, string)
	walk = func(keys []string, indent string) {
		for _, key := range keys {
			rows = append(rows, indent+tree.nodes[key].name)
			walk(tree.children(key), indent+"  ")
		}
	}
	walk(tree.roots, "")
	want := []string{"a", "  b/c", "    x.go", "    y.go", "  z.go", "docs", "  guide.md", "README.md", "go.mod"}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows = %q, want %q", rows, want)
	}
	if got := tree.ancestors("a/b/c/y.go"); !slices.Equal(got, []string{"a/", "a/b/c/"}) {
		t.Fatalf("ancestors = %q", got)
	}
	if got := tree.nodes["a/b/c/x.go"].file.Path; got != "a/b/c/x.go" {
		t.Fatalf("file = %q", got)
	}

	open := map[string]bool{"a/": true, "a/b/c/": true, "docs/": false}
	has := func(key string) bool { return open[key] }
	for key, row := range map[string]int{"a/": 0, "a/b/c/y.go": 3, "a/z.go": 4, "docs/": 5, "docs/guide.md": -1, "README.md": 6} {
		if got := tree.row(tree.roots, key, has); got != row {
			t.Errorf("row(%q) = %d, want %d", key, got, row)
		}
	}
}

func TestChangeTreeGroupsAreas(t *testing.T) {
	tree := newChangeTree([]FileChange{
		{Path: "a/b/x.go", Status: 'M', Area: AreaUnstaged},
		{Path: "a/b/x.go", Status: 'M', Area: AreaStaged},
		{Path: "y.go", Status: 'A', Area: AreaUnstaged},
	})
	staged, unstaged := areaKeys[AreaStaged], areaKeys[AreaUnstaged]
	if !slices.Equal(tree.roots, []string{unstaged, staged}) {
		t.Fatalf("roots = %q", tree.roots)
	}
	if got := tree.children(unstaged); !slices.Equal(got, []string{unstaged + "a/b/", unstaged + "y.go"}) || tree.nodes[got[0]].name != "a/b" {
		t.Fatalf("unstaged = %q", got)
	}
	if got := tree.ancestors(unstaged + "a/b/x.go"); !slices.Equal(got, []string{unstaged, unstaged + "a/b/"}) {
		t.Fatalf("ancestors = %q", got)
	}
}
