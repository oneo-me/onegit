package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	// Windows forbids newlines in file names; keep the arbitrary-name case on
	// platforms that can create it.
	linkedName := "a linked 路径"
	if runtime.GOOS != "windows" {
		linkedName = "a linked\n路径"
	}
	linked := filepath.Join(root, linkedName)
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
	callbacks := make(chan func(), 64)
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
	s.repo.dispatch = func(func()) {}
	tt := ui.NewTester(s.worktreeTabs, 800, 600)
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

func TestRememberProjectAndWorktreeSelections(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repoA, repoB := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, repo := range []string{repoA, repoB} {
		testGit(t, "init", repo)
	}
	testGit(t, "-C", repoA, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	linked := filepath.Join(root, "linked")
	testGit(t, "-C", repoA, "worktree", "add", "-b", "feature", linked)
	s := &settings{statePath: filepath.Join(t.TempDir(), "state.yaml"), values: preferences{ProjectParentDir: root}}
	finishScan(t, s)
	if s.selectedProject == nil || s.selectedProject.Path != repoA || s.overview.SelectedPath != repoA {
		t.Fatal("first repository and worktree were not selected by default")
	}
	callbacks := make(chan func(), 64)
	dispatch := func(fn func()) { callbacks <- fn }
	finish := func(s *settings) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for s.scanning || s.overview.Loading || s.repo.status.loading || s.repo.branches.loading || s.repo.commits.loading {
			select {
			case fn := <-callbacks:
				fn()
			case <-deadline:
				t.Fatal("selection restoration timed out")
			}
		}
	}
	choose := func(repo, label, want string) {
		t.Helper()
		s.selectProject(findProject(s.projectTree, repo), dispatch)
		finish(s)
		view := ui.NewTester(s.worktreeTabs, 800, 600)
		view.Frame()
		// A repository with a single worktree shows no tab strip; the only entry
		// is already selected.
		if _, ok := view.Find(label); ok {
			if err := view.Click(label); err != nil {
				t.Fatal(err)
			}
			view.Frame()
		}
		if s.overview.SelectedPath != want {
			t.Fatalf("selected %q, want %q", s.overview.SelectedPath, want)
		}
		finish(s)
	}
	choose(repoA, "linked", linked)
	choose(repoB, "b", repoB)
	s.selectProject(findProject(s.projectTree, repoA), dispatch)
	finish(s)
	if s.overview.SelectedPath != linked {
		t.Fatal("switching repositories forgot the worktree")
	}
	// A folder click must not overwrite the last repository preference.
	s.selectProject(s.projectTree, dispatch)
	state, err := loadSidebarState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	s = &settings{state: state, statePath: s.statePath, values: s.values}
	s.startProjectScan(dispatch)
	if s.selectedProject == nil || s.selectedProject.Path != repoA {
		t.Fatal("cached repository was not restored immediately")
	}
	finish(s)
	view := ui.NewTester(s.worktreeTabs, 800, 600)
	view.Frame()
	if s.overview.SelectedPath != linked || s.overview.selectedIndex != 1 {
		t.Fatal("restart did not restore worktree highlight")
	}
	s.selectProject(findProject(s.projectTree, repoB), dispatch)
	finish(s)
	if s.overview.SelectedPath != repoB {
		t.Fatal("restart lost the other repository's worktree")
	}
	// Each parent directory retains its own last repository.
	s.values.ProjectParentDir = t.TempDir()
	s.startProjectScan(dispatch)
	finish(s)
	if s.selectedProject != nil {
		t.Fatal("repository selection leaked across parent directories")
	}
	s.values.ProjectParentDir = root
	s.startProjectScan(dispatch)
	finish(s)
	if s.selectedProject == nil || s.selectedProject.Path != repoB || s.overview.SelectedPath != repoB {
		t.Fatal("switching back lost selection")
	}
	// A removed worktree falls back to the first current entry.
	testGit(t, "-C", repoA, "worktree", "remove", linked)
	s.selectProject(findProject(s.projectTree, repoA), dispatch)
	finish(s)
	view = ui.NewTester(s.worktreeTabs, 800, 600)
	view.Frame()
	if s.overview.SelectedPath != repoA || s.overview.selectedIndex != 0 {
		t.Fatal("removed worktree did not fall back to the first entry")
	}
}

func TestEmptyNavigationContent(t *testing.T) {
	for _, s := range []*settings{
		{},
		{selectedProject: &projectNode{Path: "/folder"}},
	} {
		view := ui.NewTester(s.welcome, 800, 600)
		view.Frame()
		if len(view.Texts()) != 0 {
			t.Fatalf("empty content contains prompts: %v", view.Texts())
		}
	}
	for _, parent := range []string{"", "/empty"} {
		s := &settings{values: preferences{ProjectParentDir: parent}, systemLanguage: "zh-CN"}
		view := ui.NewTester(s.worktreeSidebar, 300, 720)
		view.Frame()
		for _, prompt := range []string{"请在设置中选择项目父目录。", "未找到 Git 项目。"} {
			if view.HasText(prompt) {
				t.Fatalf("empty sidebar contains %q", prompt)
			}
		}
	}
}

func TestDefaultRepositoryUsesSidebarOrder(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, repo := range []string{a, b} {
		testGit(t, "init", repo)
	}
	s := &settings{values: preferences{ProjectParentDir: root}}
	finishScan(t, s)
	first, pinned := findProject(s.projectTree, a), findProject(s.projectTree, b)
	pinned.Pinned = true
	s.selectedProject = nil
	s.state.SelectedProjects[root] = "/removed"
	callbacks := make(chan func(), 64)
	s.restoreProjectSelection(func(fn func()) { callbacks <- fn })
	awaitNavigation(t, s, callbacks)
	if s.selectedProject != pinned {
		t.Fatal("default selection ignored pinned sidebar order")
	}
	pinned.Missing = true
	s.restoreProjectSelection(func(fn func()) { callbacks <- fn })
	awaitNavigation(t, s, callbacks)
	if s.selectedProject != first {
		t.Fatal("missing repository did not fall back to an available repository")
	}
	first.Missing = true
	s.restoreProjectSelection(func(fn func()) { callbacks <- fn })
	if s.selectedProject != nil || s.overview.SelectedPath != "" {
		t.Fatal("all missing repositories should leave blank content")
	}
}
