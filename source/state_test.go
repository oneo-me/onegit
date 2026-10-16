package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func finishScan(t *testing.T, s *settings) {
	t.Helper()
	callbacks := make(chan func(), 64)
	s.startProjectScan(func(fn func()) { callbacks <- fn })
	awaitNavigation(t, s, callbacks)
}

func awaitNavigation(t *testing.T, s *settings, callbacks <-chan func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for s.scanning || s.overview.Loading || s.repositoryLoading() {
		select {
		case fn := <-callbacks:
			fn()
		case <-deadline:
			t.Fatal("navigation timed out")
		}
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
	awaitNavigation(t, restarted, callbacks)
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

func TestPinnedProjectGroupsAndRestore(t *testing.T) {
	pinned := &projectNode{Path: "/projects/group/repo", Repository: true}
	other := &projectNode{Path: "/projects/other", Repository: true}
	group := &projectNode{Path: "/projects/group", Open: true, savedOpen: true, Children: []*projectNode{pinned}}
	root := &projectNode{Path: "/projects", Children: []*projectNode{group, other}}
	s := &settings{projectRoot: root.Path, projectTree: root, statePath: filepath.Join(t.TempDir(), "state.yaml"), values: preferences{ProjectParentDir: root.Path}, systemLanguage: "zh-CN"}
	view := ui.NewTester(s.worktreeSidebar, 300, 720)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if err := view.Click("repo"); err != nil {
		t.Fatal(err)
	}
	if err := view.RightClick("repo"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("置顶项目"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !pinned.Pinned || s.selectedProject != pinned || view.HasText("group") || !view.HasText("repo") || !view.HasText("other") || !view.HasText("置顶") || !view.HasText("其他项目") {
		t.Fatal("pinning did not move the project out of its hierarchy", view.Texts())
	}
	pins, visible := sidebarProjects(root)
	if len(pins) != 1 || pins[0] != pinned || visible[pinned] || visible[group] || !visible[other] || !group.Open {
		t.Fatal("incorrect projection or expansion changed")
	}
	state, err := loadSidebarState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	restored := findProject(state.Roots[root.Path], pinned.Path)
	if restored == nil || !restored.Pinned {
		t.Fatal("pin was not persisted")
	}
	fresh := &projectNode{Path: root.Path, Children: []*projectNode{{Path: group.Path, Children: []*projectNode{{Path: pinned.Path, Repository: true}}}}}
	merged := mergeProjects(state.Roots[root.Path], fresh, nil)
	if !findProject(merged, pinned.Path).Pinned {
		t.Fatal("scan discarded pinned state")
	}
	if err := view.RightClick("repo"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("取消置顶"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if pinned.Pinned || !view.HasText("group") || !view.HasText("repo") || s.selectedProject != pinned {
		t.Fatal("unpin did not restore hierarchy and selection", view.Texts())
	}
	state, err = loadSidebarState(s.statePath)
	if err != nil || findProject(state.Roots[root.Path], pinned.Path).Pinned {
		t.Fatal("unpin was not persisted", err)
	}
}

func TestPinnedProjectionIgnoresCollapsedAncestors(t *testing.T) {
	first := &projectNode{Path: "/projects/a/same", Repository: true, Pinned: true, Missing: true}
	second := &projectNode{Path: "/projects/b/same", Repository: true, Pinned: true}
	root := &projectNode{Path: "/projects", Children: []*projectNode{
		{Path: "/projects/a", Children: []*projectNode{first}},
		{Path: "/projects/b", Children: []*projectNode{second}},
	}}
	pins, visible := sidebarProjects(root)
	if len(pins) != 2 || pins[0] != first || pins[1] != second || visible[root] {
		t.Fatal("collapsed, missing or same-name projects lost")
	}
	pins, visible = sidebarProjects(second)
	if len(pins) != 1 || visible[second] {
		t.Fatal("repository root not pinned")
	}
	second.Pinned = false
	pins, visible = sidebarProjects(second)
	if len(pins) != 0 || !visible[second] {
		t.Fatal("repository root not restored")
	}
}

func TestSidebarGroupCollapseAndRestore(t *testing.T) {
	pin := &projectNode{Path: "/projects/pin", Repository: true, Pinned: true}
	repo := &projectNode{Path: "/projects/group/repo", Repository: true}
	group := &projectNode{Path: "/projects/group", Open: true, savedOpen: true, Children: []*projectNode{repo}}
	root := &projectNode{Path: "/projects", Children: []*projectNode{pin, group}}
	s := &settings{projectRoot: root.Path, projectTree: root, selectedProject: repo, statePath: filepath.Join(t.TempDir(), "state.yaml"), values: preferences{ProjectParentDir: root.Path}, systemLanguage: "zh-CN"}
	view := ui.NewTester(s.worktreeSidebar, 300, 720)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if !view.HasText("pin") || !view.HasText("repo") {
		t.Fatal("groups should be expanded by default", view.Texts())
	}
	if err := view.Click("置顶"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if view.HasText("pin") || !view.HasText("repo") || *s.state.PinnedOpen || !*s.state.OtherOpen {
		t.Fatal("pinned collapse affected other group", view.Texts())
	}
	if err := view.Click("其他项目"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if view.HasText("repo") || view.HasText("group") || *s.state.OtherOpen || !group.Open || s.selectedProject != repo {
		t.Fatal("collapse lost project state", view.Texts())
	}
	state, err := loadSidebarState(s.statePath)
	if err != nil || state.PinnedOpen == nil || *state.PinnedOpen || state.OtherOpen == nil || *state.OtherOpen {
		t.Fatal("group collapse was not persisted", err)
	}
	restarted := &settings{state: state, projectRoot: root.Path, projectTree: state.Roots[root.Path], statePath: s.statePath, values: s.values, systemLanguage: s.systemLanguage}
	view = ui.NewTester(restarted.worktreeSidebar, 300, 720)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if view.HasText("pin") || view.HasText("repo") {
		t.Fatal("restart did not restore collapse", view.Texts())
	}
	if err := view.Click("其他项目"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !view.HasText("repo") || view.HasText("pin") {
		t.Fatal("expand did not restore nested projects", view.Texts())
	}
	// The clicked disclosure keeps keyboard focus; Space toggles it again.
	view.Key(0, ui.KeySpace)
	view.Frame()
	if *restarted.state.OtherOpen || view.HasText("repo") {
		t.Fatal("keyboard collapse failed", view.Texts())
	}
}

func TestWindowStateStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	s := &settings{statePath: path, state: sidebarState{SidebarWidth: 260, SidebarCollapsed: true}}
	store := windowStateStore{s}
	if states, err := store.LoadWindowStates(); err != nil || states != nil {
		t.Fatal("empty state restored a window", states, err)
	}
	saved := mygo.WindowState{X: 10, Y: 20, Width: 1200, Height: 800, Maximized: true}
	if err := store.SaveWindowStates(map[string]mygo.WindowState{"other": {Width: 1}, mainWindowStateKey: saved}); err != nil {
		t.Fatal(err)
	}
	state, err := loadSidebarState(path)
	if err != nil || state.Window == nil || *state.Window != saved || state.SidebarWidth != 260 || !state.SidebarCollapsed {
		t.Fatal("window state not saved with the sidebar layout", state, err)
	}
	states, err := windowStateStore{&settings{state: state}}.LoadWindowStates()
	if err != nil || len(states) != 1 || states[mainWindowStateKey] != saved {
		t.Fatal("window state not restored", states, err)
	}
}

func TestDataDirMigration(t *testing.T) {
	home := t.TempDir()
	dir, legacy := filepath.Join(home, ".onegit"), filepath.Join(home, ".oneo", "onegit")
	if err := migrateDataDir(dir, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("created a data directory without legacy data", err)
	}
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "state.yaml"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrateDataDir(dir, legacy); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "state.yaml")); err != nil || string(data) != "old" {
		t.Fatal("legacy data not moved", string(data), err)
	}
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "state.yaml"), []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrateDataDir(dir, legacy); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "state.yaml")); string(data) != "old" {
		t.Fatal("legacy data overwrote the data directory")
	}
}
