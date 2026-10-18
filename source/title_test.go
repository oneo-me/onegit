package main

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The repository's actions live in the title bar beside its name, not in a
// strip of their own.
func TestRepositoryTitleActions(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "demo")
	testGit(t, "init", repo)
	s := &settings{systemLanguage: "zh-CN", selectedProject: &projectNode{Path: repo, Repository: true}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).FillWidth().Height(52).Padding(0, 8).Children(func() { s.repositoryTitle(c) })
	}, 800, 52)
	tt.Frame()
	if !tt.HasText("demo") {
		t.Fatal("project name missing from the title bar")
	}
	for _, label := range []string{"获取", "拉取", "推送", "贮藏", "分支操作"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%s missing from the title bar", label)
		}
		if r.Y < 0 || r.Y+r.H > 52 {
			t.Fatalf("%s outside the title bar: %+v", label, r)
		}
	}

	// Many worktrees scroll within the title bar and leave the actions in place.
	for i := range 30 {
		s.overview.Worktrees = append(s.overview.Worktrees, worktree{Path: filepath.Join(root, "worktree-"+strconv.Itoa(i))})
	}
	tt.Frame()
	if r, ok := tt.Find("分支操作"); !ok || r.X+r.W > 800 {
		t.Fatalf("worktree tabs pushed the actions out: %+v", r)
	}

	page := ui.NewTester(s.welcome, 800, 600)
	page.Frame()
	if page.HasText("获取") {
		t.Fatal("the repository page still has a toolbar strip")
	}
}
