package main

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestWorktreeTabsHiddenForSingleWorktree(t *testing.T) {
	s := &settings{systemLanguage: "zh-CN", selectedProject: &projectNode{Path: "/repo", Repository: true}}
	s.repo.dispatch = func(func()) {}
	s.overview = projectOverview{Worktrees: []worktree{{Path: "/repo", Branch: "main", Main: true}}, SelectedPath: "/repo", selectedIndex: 0}

	view := ui.NewTester(s.worktreeTabs, 640, 200)
	view.Frame()
	if len(view.Texts()) != 0 {
		t.Fatalf("single worktree still shows tabs: %v", view.Texts())
	}

	s.overview.Worktrees = append(s.overview.Worktrees, worktree{Path: "/outside/feature", Branch: "feature"})
	view = ui.NewTester(s.worktreeTabs, 640, 200)
	view.Frame()
	if !view.HasText("repo") || !view.HasText("feature") {
		t.Fatalf("missing worktree tabs: %v", view.Texts())
	}
	if err := view.Click("feature"); err != nil {
		t.Fatal(err)
	}
	if s.overview.SelectedPath != "/outside/feature" || s.overview.selectedIndex != 1 {
		t.Fatalf("tab click did not switch worktree: %+v", s.overview)
	}
}

func TestLoadStateIgnoresStaleResult(t *testing.T) {
	var state loadState[string]
	pending := make(chan func(), 2)
	dispatch := func(fn func()) { pending <- fn }

	state.start(dispatch, "/one", func(context.Context) (string, error) { return "one", nil }, nil)
	first := <-pending
	state.start(dispatch, "/two", func(context.Context) (string, error) { return "two", nil }, nil)
	second := <-pending

	second()
	if state.value != "two" || state.path != "/two" {
		t.Fatalf("latest result missing: %+v", state)
	}
	first()
	if state.value != "two" {
		t.Fatalf("stale result overwrote the latest: %+v", state)
	}
	state.reset()
	if state.loaded || state.path != "" || state.value != "" {
		t.Fatalf("reset kept old data: %+v", state)
	}
}
