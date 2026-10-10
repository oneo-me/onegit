package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func testGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestScanProjects(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "group", "项目 with spaces")
	testGit(t, "init", repo)
	testGit(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	linked := filepath.Join(root, "linked")
	testGit(t, "-C", repo, "worktree", "add", "--detach", linked)
	moduleSource := t.TempDir()
	testGit(t, "init", moduleSource)
	testGit(t, "-C", moduleSource, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	testGit(t, "-C", repo, "-c", "protocol.file.allow=always", "submodule", "add", moduleSource, "module")
	submodule := filepath.Join(repo, "module")
	nested := filepath.Join(repo, "nested")
	testGit(t, "init", nested)
	bare := filepath.Join(root, "bare.git")
	testGit(t, "init", "--bare", bare)
	empty := filepath.Join(root, "empty")
	testGit(t, "init", empty)
	for _, dir := range []string{filepath.Join(root, "fake", ".git"), filepath.Join(repo, "ordinary"), filepath.Join(root, "unused")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	// Environment overrides must not cause every directory to become a repo.
	t.Setenv("GIT_DIR", filepath.Join(repo, ".git"))
	t.Setenv("GIT_WORK_TREE", repo)
	tree, err := scanProjects(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]*projectNode{}
	var collect func(*projectNode)
	collect = func(n *projectNode) {
		if n == nil {
			return
		}
		found[n.Path] = n
		for _, child := range n.Children {
			collect(child)
		}
	}
	collect(tree)
	for _, path := range []string{repo, linked, bare, empty} {
		if found[path] == nil || !found[path].Repository {
			t.Fatalf("missing repository %s: %+v", path, found)
		}
	}
	if len(found) != 6 || !found[bare].Bare {
		t.Fatalf("unexpected tree: %+v", found)
	}
	for _, path := range []string{nested, submodule} {
		if found[path] != nil {
			t.Fatalf("scanned inside repository: %s", path)
		}
	}
	// The selected root itself can be a repository, including a submodule.
	for _, path := range []string{repo, submodule} {
		got, err := scanProjects(context.Background(), path)
		if err != nil || got == nil || !got.Repository || len(got.Children) != 0 {
			t.Fatalf("repository root should be a leaf: %+v, %v", got, err)
		}
	}
	// Selecting an ordinary subdirectory must not discover its ancestor repo.
	if got, err := scanProjects(context.Background(), filepath.Join(repo, "ordinary")); err != nil || got != nil {
		t.Fatalf("ordinary directory: %+v, %v", got, err)
	}
}

func TestScanFailuresAndEmpty(t *testing.T) {
	root := t.TempDir()
	if tree, err := scanProjects(context.Background(), root); err != nil || tree != nil {
		t.Fatalf("empty: %+v, %v", tree, err)
	}
	if _, err := scanProjects(context.Background(), filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing root accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanProjects(ctx, root); err == nil {
		t.Fatal("cancellation ignored")
	}
	t.Setenv("PATH", "")
	if _, err := scanProjects(context.Background(), root); err == nil {
		t.Fatal("missing git ignored")
	}
}

func TestProjectTreeInteraction(t *testing.T) {
	leaf := &projectNode{Path: "/projects/group/repo", Repository: true, Open: true}
	group := &projectNode{Path: "/projects/group", Open: true, Children: []*projectNode{leaf}}
	s := &settings{systemLanguage: "zh-CN", values: preferences{ProjectParentDir: "/projects"}, projectTree: &projectNode{Path: "/projects", Open: true, Children: []*projectNode{group, {Path: "/projects/repo", Repository: true}}}}
	tt := ui.NewTester(s.worktreeSidebar, 300, 720)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame() // Repeated leaf names must have independent identities.
	if err := tt.Click("group"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if group.Open {
		t.Fatal("keyboard collapse failed")
	}
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	if !group.Open {
		t.Fatal("keyboard expansion failed")
	}
	if err := tt.Click("repo"); err != nil {
		t.Fatal(err)
	}
	if s.selectedProject == nil || !s.selectedProject.Repository {
		t.Fatal("project selection failed")
	}
	content := ui.NewTester(s.welcome, 800, 600)
	content.Frame()
	if content.HasText(s.selectedProject.Path) {
		t.Fatal("project path should not appear in the compact worktree list")
	}
}

func TestScanCompletionIgnoresOldDirectory(t *testing.T) {
	s := &settings{values: preferences{ProjectParentDir: t.TempDir()}}
	testGit(t, "init", s.values.ProjectParentDir)
	callbacks := make(chan func(), 2)
	s.startProjectScan(func(fn func()) { callbacks <- fn })
	var oldCompletion func()
	select {
	case oldCompletion = <-callbacks:
	case <-time.After(5 * time.Second):
		t.Fatal("first scan did not complete")
	}
	s.values.ProjectParentDir = t.TempDir()
	s.startProjectScan(func(fn func()) { callbacks <- fn })
	select {
	case fn := <-callbacks:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not complete")
	}
	oldCompletion()
	if s.scanning || s.projectTree != nil || s.scanProblem != "" {
		t.Fatalf("unexpected completion: %+v", s)
	}
}
