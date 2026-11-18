package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"go.yaml.in/yaml/v3"
)

func finishScan(t *testing.T, s *settings) {
	t.Helper()
	callbacks := make(chan func(), 64)
	s.dispatch = func(fn func()) { callbacks <- fn }
	s.scanProjects()
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
	testGit(t, "init", filepath.Join(root, "group", "kept"))
	gone := filepath.Join(root, "gone", "repo")
	testGit(t, "init", gone)
	path := filepath.Join(t.TempDir(), "state.yaml")
	s := &settings{statePath: path, values: preferences{ProjectParentDir: root}, systemLanguage: "zh-CN"}
	finishScan(t, s)
	group := findProject(s.projectTree, filepath.Dir(repo))
	if group == nil || group.Open {
		t.Fatal("new group must be collapsed")
	}
	group.Open = true
	if err := s.saveSidebarState(); err != nil {
		t.Fatal(err)
	}
	restored, err := loadSidebarState(path)
	if err != nil || !findProject(restored.Roots[root], group.Path).Open {
		t.Fatal("expansion not persisted", err)
	}
	group.Open = false
	findProject(s.projectTree, repo).Pinned = true
	s.state.SelectedProjects = map[string]string{root: repo}
	if err := s.saveSidebarState(); err != nil {
		t.Fatal(err)
	}
	if restored, err = loadSidebarState(path); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(root, "new-group", "new")
	testGit(t, "init", added)
	restarted := &settings{state: restored, statePath: path, values: s.values, systemLanguage: "zh-CN"}
	callbacks := make(chan func(), 64)
	restarted.dispatch = func(fn func()) { callbacks <- fn }
	restarted.scanProjects()
	if findProject(restarted.projectTree, repo) == nil {
		t.Fatal("cached tree not immediately restored")
	}
	// Expand while the worker is running; completion must retain the change.
	findProject(restarted.projectTree, group.Path).Open = true
	awaitNavigation(t, restarted, callbacks)
	if findProject(restarted.projectTree, repo) != nil || findProject(restarted.projectTree, filepath.Dir(gone)) != nil {
		t.Fatal("missing project or its emptied folder kept")
	}
	if findProject(restarted.projectTree, added) == nil {
		t.Fatal("merge lost new project")
	}
	if restarted.selectedProject == nil || restarted.selectedProject.Path == repo {
		t.Fatal("selection did not fall back from the missing project")
	}
	if !findProject(restarted.projectTree, group.Path).Open || findProject(restarted.projectTree, filepath.Dir(added)).Open {
		t.Fatal("merge changed expansion")
	}
	testGit(t, "init", repo)
	finishScan(t, restarted)
	if node := findProject(restarted.projectTree, repo); node == nil || node.Missing || node.Pinned {
		t.Fatal("reappearing project not listed afresh")
	}
	// A missing parent directory, like an unmounted volume, keeps the cache.
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	finishScan(t, restarted)
	if err := os.Rename(root+"-moved", root); err != nil {
		t.Fatal(err)
	}
	if restarted.scanProblem == "" || findProject(restarted.projectTree, repo) == nil {
		t.Fatal("missing parent directory dropped the cache")
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

func TestMergeKeepsRemovedProjects(t *testing.T) {
	old := &projectNode{Path: "/projects", Children: []*projectNode{{Path: "/projects/repo", Repository: true, Removed: true}}}
	fresh := &projectNode{Path: "/projects", Children: []*projectNode{{Path: "/projects/repo", Repository: true}}}
	merged := mergeProjects(old, fresh, map[string]bool{})
	if len(merged.Children) != 1 || !merged.Children[0].Removed {
		t.Fatalf("scan restored a removed project: %+v", merged.Children)
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
	s.dispatch = func(func()) {}
	app := show(t, s, s.worktreeSidebar)
	app.Press("id:project:" + pinned.Path)
	app.RightClick("id:project:" + pinned.Path)
	app.ChoosePopup("置顶项目")
	app.WaitForGone("id:project:"+group.Path, time.Second)
	if !pinned.Pinned || s.selectedProject != pinned || !app.Exists("id:project:"+pinned.Path) || !app.Exists("id:project:"+other.Path) ||
		!app.Exists("text:置顶") || !app.Exists("text:未分组项目") {
		t.Fatal("pinning did not move the project out of its hierarchy")
	}
	pins, _, visible := sidebarProjects(root, nil)
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
	app.RightClick("id:project:" + pinned.Path)
	app.ChoosePopup("取消置顶")
	app.WaitFor("id:project:"+group.Path, time.Second)
	if pinned.Pinned || s.selectedProject != pinned {
		t.Fatal("unpin did not restore hierarchy and selection")
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
	pins, _, visible := sidebarProjects(root, nil)
	if len(pins) != 2 || pins[0] != first || pins[1] != second || visible[root] {
		t.Fatal("collapsed, missing or same-name projects lost")
	}
	pins, _, visible = sidebarProjects(second, nil)
	if len(pins) != 1 || visible[second] {
		t.Fatal("repository root not pinned")
	}
	second.Pinned = false
	pins, _, visible = sidebarProjects(second, nil)
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
	app := show(t, s, s.worktreeSidebar)
	if !app.Exists("id:project:"+pin.Path) || !app.Exists("id:project:"+repo.Path) {
		t.Fatal("groups should be expanded by default")
	}
	app.Press("id:pinned")
	app.WaitForGone("id:project:"+pin.Path, time.Second)
	if !app.Exists("id:project:"+repo.Path) || *s.state.PinnedOpen || s.state.OtherOpen != nil && !*s.state.OtherOpen {
		t.Fatal("pinned collapse affected other group")
	}
	app.Press("id:group:" + ungroupedID)
	app.WaitForGone("id:project:"+repo.Path, time.Second)
	if app.Exists("id:project:"+group.Path) || *s.state.OtherOpen || !group.Open || s.selectedProject != repo {
		t.Fatal("collapse lost project state")
	}
	state, err := loadSidebarState(s.statePath)
	if err != nil || state.PinnedOpen == nil || *state.PinnedOpen || state.OtherOpen == nil || *state.OtherOpen {
		t.Fatal("group collapse was not persisted", err)
	}
	restarted := &settings{state: state, projectRoot: root.Path, projectTree: state.Roots[root.Path], statePath: s.statePath, values: s.values, systemLanguage: s.systemLanguage}
	view := show(t, restarted, restarted.worktreeSidebar)
	if view.Exists("id:project:"+pin.Path) || view.Exists("id:project:"+repo.Path) {
		t.Fatal("restart did not restore collapse")
	}
	view.Press("id:group:" + ungroupedID)
	view.WaitFor("id:project:"+repo.Path, time.Second)
	if view.Exists("id:project:" + pin.Path) {
		t.Fatal("expand opened the pinned group")
	}
}

// The window's frame and state restore from state.yaml.
func TestWindowState(t *testing.T) {
	var state sidebarState
	if err := yaml.Unmarshal([]byte("window:\n  x: 10\n  y: 20\n  width: 1200\n  height: 800\n  maximized: true\nsidebarWidth: 260\n"), &state); err != nil {
		t.Fatal(err)
	}
	f := state.Window.frame()
	if f == nil || *f != (core.WindowFrame{X: 10, Y: 20, Move: true, Width: 1200, Height: 800}) || !state.Window.Maximized || state.SidebarWidth != 260 {
		t.Fatalf("restored %+v, frame %+v", state, f)
	}
	if (windowState{}).frame() != nil {
		t.Fatal("an empty state placed the window")
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

func TestProjectGroupsMoveReorderAndRestore(t *testing.T) {
	alpha := &projectNode{Path: "/projects/alpha", Repository: true}
	beta := &projectNode{Path: "/projects/nested/beta", Repository: true}
	nested := &projectNode{Path: "/projects/nested", Open: true, savedOpen: true, Children: []*projectNode{beta}}
	root := &projectNode{Path: "/projects", Children: []*projectNode{alpha, nested}}
	s := &settings{projectRoot: root.Path, projectTree: root, statePath: filepath.Join(t.TempDir(), "state.yaml"), values: preferences{ProjectParentDir: root.Path}, systemLanguage: "en-US"}
	app := show(t, s, s.worktreeSidebar)
	app.WaitFor("text:Ungrouped Projects", time.Second)
	// Headings count their projects, not folders, and empty groups show none.
	app.WaitFor("id:group:"+ungroupedID+"-group >> text:/^2$/", time.Second)
	if app.Exists("id:pinned-group >> text:/^0$/") {
		t.Fatal("an empty group shows a count")
	}

	// A group made from a project's menu takes the project.
	app.RightClick("id:project:" + beta.Path)
	app.ChoosePopup("Move to Group", "New Group…")
	app.Type("id:group-name", "Work")
	app.Submit("id:group-name")
	if s.groupDialog.open || beta.Group == "" {
		t.Fatal("new group did not take the project", beta.Group)
	}
	app.WaitFor("text:Work", time.Second)
	app.WaitForGone("id:project:"+nested.Path, time.Second)
	app.WaitFor("id:group:"+ungroupedID+"-group >> text:/^1$/", time.Second)
	app.RightClick("id:group:" + ungroupedID)
	app.ChoosePopup("New Group…")
	app.Type("id:group-name", "Later")
	app.Submit("id:group-name")
	app.RightClick("id:project:" + alpha.Path)
	app.ChoosePopup("Move to Group", "Later")
	order := func(groups []*projectGroup) string {
		var names []string
		for _, group := range groups {
			names = append(names, group.ID+"="+group.Name)
		}
		return strings.Join(names, ",")
	}
	if got := order(s.projectGroups()); got != "g1=Work,g2=Later,ungrouped=" || alpha.Group != "g2" {
		t.Fatal("unexpected groups", got, alpha.Group)
	}

	// Dragging a heading onto another group moves it there, in either
	// direction.
	drag := func(from, to string) {
		t.Helper()
		src, target := app.Find("id:"+from).Bounds(), app.Find("id:"+to+"-group").Bounds()
		dy := target.Origin.Y + target.Size.Height/2 - (src.Origin.Y + src.Size.Height/2)
		app.Pan("id:"+from, core.Point{Y: dy / 2}, core.Point{Y: dy})
	}
	drag("group:"+ungroupedID, "group:g1")
	if got := order(s.projectGroups()); got != "ungrouped=,g1=Work,g2=Later" {
		t.Fatal("drag up did not reorder", got)
	}
	drag("group:g1", "group:g2")
	if got := order(s.projectGroups()); got != "ungrouped=,g2=Later,g1=Work" {
		t.Fatal("drag down did not reorder", got)
	}

	app.RightClick("id:group:g1")
	app.ChoosePopup("Rename Group…")
	if s.groupDialog.name != "Work" {
		t.Fatalf("rename starts from %q", s.groupDialog.name)
	}
	app.Clear("id:group-name")
	app.Type("id:group-name", "Client")
	app.Press("id:group-dialog >> id:confirm")
	app.WaitFor("text:Client", time.Second)
	if app.Exists("text:Work") || beta.Group != "g1" {
		t.Fatal("rename lost the group's projects")
	}

	// A restart restores the order, names and members.
	state, err := loadSidebarState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &settings{state: state, projectRoot: root.Path, projectTree: state.Roots[root.Path], statePath: s.statePath, values: s.values, systemLanguage: s.systemLanguage}
	if got := order(restarted.projectGroups()); got != "ungrouped=,g2=Later,g1=Client" || findProject(restarted.projectTree, beta.Path).Group != "g1" {
		t.Fatal("groups were not persisted", got)
	}

	// Deleting a group returns its projects to Ungrouped Projects.
	app.RightClick("id:group:g1")
	app.ChoosePopup("Delete Group")
	app.WaitFor("id:project:"+nested.Path, time.Second)
	if beta.Group != "" || app.Exists("text:Client") {
		t.Fatal("deleted group kept its projects")
	}
	app.RightClick("id:group:" + ungroupedID)
	if menu := popupLabels(app); len(menu) != 1 || menu[0] != "New Group…" {
		t.Fatalf("ungrouped menu: %q", menu)
	}
}
