package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestWorktreeList(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "项目 main")
	testGit(t, "init", "-b", "main", repo)
	items, err := listWorktrees(context.Background(), repo)
	if err != nil || len(items) != 1 || !items[0].Main || items[0].Branch != "main" {
		t.Fatalf("unborn: %+v %v", items, err)
	}
	testGit(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	linked := filepath.Join(root, "a linked\n路径")
	testGit(t, "-C", repo, "worktree", "add", "-b", "feature", linked)
	detached := filepath.Join(root, "detached")
	testGit(t, "-C", repo, "worktree", "add", "--detach", detached)
	testGit(t, "-C", repo, "worktree", "lock", "--reason", "keep", linked)
	if err := os.RemoveAll(detached); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", "/invalid")
	t.Setenv("GIT_WORK_TREE", "/invalid")
	items, err = listWorktrees(context.Background(), linked)
	if err != nil || len(items) != 3 {
		t.Fatalf("linked query: %+v %v", items, err)
	}
	if items[0].Path != repo || !items[0].Main {
		t.Fatalf("main must be first: %+v", items)
	}
	found := map[string]worktree{}
	for _, item := range items {
		found[item.Path] = item
	}
	if !found[linked].Locked || found[linked].Branch != "feature" || found[linked].Main {
		t.Fatalf("linked: %+v", found[linked])
	}
	if !found[detached].Detached || !found[detached].Prunable {
		t.Fatalf("missing detached: %+v", found[detached])
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listWorktrees(ctx, repo); err == nil {
		t.Fatal("cancellation ignored")
	}
	if _, err := listWorktrees(context.Background(), root); err == nil {
		t.Fatal("non-repository accepted")
	}
}

func TestBareWorktreeList(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "bare.git")
	testGit(t, "init", "--bare", repo)
	items, err := listWorktrees(context.Background(), repo)
	if err != nil || len(items) != 1 || !items[0].Bare || !items[0].Main {
		t.Fatalf("bare: %+v %v", items, err)
	}
}

func TestWorktreeSelectionCompletion(t *testing.T) {
	repo := t.TempDir()
	testGit(t, "init", repo)
	s := &settings{}
	callbacks := make(chan func(), 2)
	dispatch := func(fn func()) { callbacks <- fn }
	await := func() func() {
		t.Helper()
		select {
		case fn := <-callbacks:
			return fn
		case <-time.After(10 * time.Second):
			t.Fatal("worktree read timed out")
			return nil
		}
	}
	s.selectProject(&projectNode{Path: repo, Repository: true}, dispatch)
	stale := await()
	s.selectProject(&projectNode{Path: "/folder"}, dispatch)
	stale()
	if s.overview.Loading || len(s.overview.Worktrees) != 0 || s.overview.Problem != "" {
		t.Fatalf("stale result applied: %+v", s.overview)
	}
	s.selectProject(&projectNode{Path: repo, Repository: true}, dispatch)
	await()()
	if s.overview.Loading || len(s.overview.Worktrees) != 1 {
		t.Fatalf("result missing: %+v", s.overview)
	}
	s.selectProject(&projectNode{Path: repo, Repository: true}, dispatch)
	stale = await()
	s.cancelWorktrees()
	stale()
	if len(s.overview.Worktrees) != 0 {
		t.Fatal("result applied after close/reset")
	}
}

func TestProjectWorktreeContent(t *testing.T) {
	s := &settings{systemLanguage: "zh-CN", selectedProject: &projectNode{Path: "/repo", Repository: true}, overview: projectOverview{Worktrees: []worktree{
		{Path: "/repo", Branch: "main", Main: true}, {Path: "/outside/feature", Branch: "feature"},
	}}}
	tt := ui.NewTester(s.welcome, 800, 600)
	tt.Frame()
	for _, label := range []string{"repo", "feature"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %q", label)
		}
	}
}

func TestWorktreeSelection(t *testing.T) {
	s := &settings{overview: projectOverview{Worktrees: []worktree{
		{Path: "/repo", Main: true}, {Path: "/outside/feature"},
	}}}
	tt := ui.NewTester(s.projectWorktrees, 800, 600)
	tt.Frame()
	for _, row := range []struct{ label, path string }{{"feature", "/outside/feature"}, {"repo", "/repo"}} {
		if err := tt.Click(row.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if s.overview.SelectedPath != row.path {
			t.Fatalf("selected %q, want %q", s.overview.SelectedPath, row.path)
		}
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if s.overview.SelectedPath != "/outside/feature" {
		t.Fatal("keyboard selection failed")
	}
	s.selectProject(&projectNode{Path: "/folder"}, func(fn func()) { fn() })
	if s.overview.SelectedPath != "" {
		t.Fatal("worktree selection retained across projects")
	}
}
