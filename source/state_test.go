package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func finishScan(t *testing.T, s *settings) {
	t.Helper()
	callbacks := make(chan func(), 1)
	s.startProjectScan(func(fn func()) { callbacks <- fn })
	select {
	case fn := <-callbacks:
		fn()
	case <-time.After(10 * time.Second):
		t.Fatal("scan timed out")
	}
}

func findProject(root *projectNode, path string) *projectNode {
	var found *projectNode
	walkProjects(root, func(n *projectNode) {
		if n.Path == path {
			found = n
		}
	})
	return found
}

func TestSidebarRestartAndMerge(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "group", "old")
	testGit(t, "init", repo)
	path := filepath.Join(t.TempDir(), "state.yaml")
	s := &settings{statePath: path, values: preferences{ProjectParentDir: root}, systemLanguage: "zh-CN"}
	finishScan(t, s)
	group := findProject(s.projectTree, filepath.Dir(repo))
	if group == nil || group.Open {
		t.Fatal("new group must be collapsed")
	}
	tt := ui.NewTester(s.worktreeSidebar, 300, 720)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := tt.Click("group"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	restored, err := loadSidebarState(path)
	if err != nil || !findProject(restored.Roots[root], group.Path).Open {
		t.Fatal("keyboard expansion not persisted", err)
	}
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	restored, err = loadSidebarState(path)
	if err != nil || findProject(restored.Roots[root], group.Path).Open {
		t.Fatal("keyboard collapse not persisted", err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(root, "new-group", "new")
	testGit(t, "init", added)
	restarted := &settings{state: restored, statePath: path, values: s.values, systemLanguage: "zh-CN"}
	callbacks := make(chan func(), 1)
	restarted.startProjectScan(func(fn func()) { callbacks <- fn })
	if findProject(restarted.projectTree, repo) == nil {
		t.Fatal("cached tree not immediately restored")
	}
	// Expand while the worker is running; completion must retain the change.
	findProject(restarted.projectTree, group.Path).Open = true
	select {
	case fn := <-callbacks:
		fn()
	case <-time.After(10 * time.Second):
		t.Fatal("scan timed out")
	}
	old := findProject(restarted.projectTree, repo)
	if old == nil || !old.Missing || findProject(restarted.projectTree, added) == nil {
		t.Fatal("merge lost old or new project")
	}
	if !findProject(restarted.projectTree, group.Path).Open || findProject(restarted.projectTree, filepath.Dir(added)).Open {
		t.Fatal("merge changed expansion")
	}
	view := ui.NewTester(restarted.worktreeSidebar, 300, 720)
	view.Frame()
	if !view.HasText("old（已不存在）") {
		t.Fatal("missing label absent", view.Texts())
	}
	testGit(t, "init", repo)
	finishScan(t, restarted)
	if findProject(restarted.projectTree, repo).Missing {
		t.Fatal("reappearing project remains missing")
	}
	// A scan failure must preserve the cache without marking existing paths missing.
	t.Setenv("PATH", "")
	finishScan(t, restarted)
	if restarted.scanProblem == "" || findProject(restarted.projectTree, repo).Missing {
		t.Fatal("failure lost cached project")
	}
	other := t.TempDir()
	restarted.values.ProjectParentDir = other
	finishScan(t, restarted)
	if restarted.projectTree != nil {
		t.Fatal("cache leaked across roots")
	}
	restarted.values.ProjectParentDir = root
	finishScan(t, restarted)
	if findProject(restarted.projectTree, repo) == nil {
		t.Fatal("switching back lost cache")
	}
}

func TestStateFilesAndMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	legacy := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(legacy, []byte(`{"projectParentDir":"/projects","theme":"dark","language":"en"}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := loadOrMigratePreferences(path, legacy)
	if err != nil || p.ProjectParentDir != "/projects" || p.Theme != "dark" || p.Language != "en" {
		t.Fatal("migration failed", p, err)
	}
	p.Language = "zh-Hans"
	if err := savePreferences(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := loadOrMigratePreferences(path, legacy)
	if err != nil || got != p {
		t.Fatal("legacy overwrote new preferences", got, err)
	}
	statePath := filepath.Join(dir, "state.yaml")
	if _, err := loadSidebarState(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("roots: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSidebarState(statePath); err == nil {
		t.Fatal("malformed state accepted")
	}
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &settings{statePath: filepath.Join(blocked, "state.yaml")}
	s.saveSidebarState()
	if s.stateProblem == "" {
		t.Fatal("save failure hidden")
	}
	data, err := os.ReadFile(blocked)
	if err != nil || string(data) != "keep" {
		t.Fatal("save failure damaged file")
	}
}
