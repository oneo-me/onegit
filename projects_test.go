package main

import (
	"context"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
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
	other := &projectNode{Path: "/projects/repo", Repository: true}
	s := &settings{systemLanguage: "zh-CN", values: preferences{ProjectParentDir: "/projects"}, projectTree: &projectNode{Path: "/projects", Open: true, Children: []*projectNode{group, other}}}
	s.dispatch = func(func()) {}
	app := show(t, s, s.worktreeSidebar)
	// Repeated leaf names keep identities of their own.
	app.Press("id:project:" + group.Path)
	if group.Open {
		t.Fatal("collapse failed")
	}
	app.WaitForGone("id:project:"+leaf.Path, time.Second)
	app.Press("id:project:" + group.Path)
	if !group.Open {
		t.Fatal("expansion failed")
	}
	app.Press("id:project:" + other.Path)
	if s.selectedProject != other {
		t.Fatal("project selection failed")
	}
}

func TestProjectContextMenu(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	group := filepath.Join(root, "group")
	for _, repo := range []string{alpha, beta, filepath.Join(group, "nested")} {
		testGit(t, "init", repo)
	}
	s := &settings{statePath: filepath.Join(t.TempDir(), "state.yaml"), values: preferences{ProjectParentDir: root}, systemLanguage: "en-US"}
	var opened []string
	s.openPath = func(path string, done func(error)) {
		opened = append(opened, path)
		done(nil)
	}
	app := show(t, s, s.worktreeSidebar)
	settled := func() bool { return !s.scanning && !s.overview.Loading && !s.repositoryLoading() }
	s.scanProjects()
	waitFor(t, app, settled)
	if s.selectedProject == nil || s.selectedProject.Path != alpha {
		t.Fatalf("initial selection: %+v", s.selectedProject)
	}

	app.RightClick("id:project:" + group)
	if menu := popupLabels(app); len(menu) != 1 || menu[0] != "Open Folder" {
		t.Fatalf("folder menu: %q", menu)
	}
	app.ChoosePopup("Open Folder")
	if len(opened) != 1 || opened[0] != group {
		t.Fatalf("opened %q", opened)
	}

	app.RightClick("id:project:" + alpha)
	menu := popupLabels(app)
	if len(menu) != 8 || menu[0] != "Open Folder" || menu[1] != "-" || menu[2] != "Pin Project" || menu[3] != "Move to Group" || menu[4] != "-" || menu[5] != "Choose Icon…" || menu[6] != "-" || menu[7] != "Remove Project" {
		t.Fatalf("project menu: %q", menu)
	}
	app.ChoosePopup("Remove Project")
	waitFor(t, app, settled)

	removed := findProject(s.projectTree, alpha)
	if removed == nil || !removed.Removed {
		t.Fatalf("project was not removed: %+v", removed)
	}
	if _, _, visible := sidebarProjects(s.projectTree, nil); visible[removed] {
		t.Fatal("removed project is still visible")
	}
	if s.selectedProject == nil || s.selectedProject.Path != beta {
		t.Fatalf("selection after removal: %+v", s.selectedProject)
	}
	if s.state.SelectedProjects[root] != beta {
		t.Fatalf("saved selection after removal: %q", s.state.SelectedProjects[root])
	}
	saved, err := loadSidebarState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if node := findProject(saved.Roots[root], alpha); node == nil || !node.Removed {
		t.Fatalf("removal was not saved: %+v", node)
	}
	for _, c := range s.commandItems() {
		if c.item.ID == "project/"+alpha {
			t.Fatal("command palette lists the removed project")
		}
	}

	// The toast undoes the removal.
	app.WaitFor("text:Project removed", time.Second)
	app.Press("role:button Undo")
	if removed.Removed {
		t.Fatal("undo did not restore the project")
	}
	app.WaitFor("id:project:"+alpha, time.Second)

	// The palette restores every project removed from the parent directory.
	app.RightClick("id:project:" + alpha)
	app.ChoosePopup("Remove Project")
	found := false
	for _, c := range s.commandItems() {
		found = found || c.item.ID == "projects/restore"
	}
	if !found {
		t.Fatal("restore command is missing")
	}
	s.activateCommand("projects/restore")
	if removed.Removed {
		t.Fatal("restore command did not restore the project")
	}
	app.WaitFor("id:project:"+alpha, time.Second)
}

func TestScanCompletionIgnoresOldDirectory(t *testing.T) {
	s := &settings{values: preferences{ProjectParentDir: t.TempDir()}}
	testGit(t, "init", s.values.ProjectParentDir)
	callbacks := make(chan func(), 2)
	s.dispatch = func(fn func()) { callbacks <- fn }
	s.scanProjects()
	var oldCompletion func()
	select {
	case oldCompletion = <-callbacks:
	case <-time.After(5 * time.Second):
		t.Fatal("first scan did not complete")
	}
	s.values.ProjectParentDir = t.TempDir()
	s.scanProjects()
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

func TestProjectIcon(t *testing.T) {
	root := t.TempDir()
	repo := &projectNode{Path: filepath.Join(root, "repo"), Repository: true}
	logo := filepath.Join(repo.Path, "logo.png")
	broken := filepath.Join(repo.Path, "broken.png")
	if err := os.MkdirAll(repo.Path, 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(logo)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := os.WriteFile(broken, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &settings{
		statePath: filepath.Join(t.TempDir(), "state.yaml"), systemLanguage: "en-US",
		values: preferences{ProjectParentDir: root}, projectRoot: root,
		projectTree: &projectNode{Path: root, Open: true, Children: []*projectNode{repo}},
	}
	var asked []core.PickFilesOptions
	choice := logo
	s.pickIcon = func(o core.PickFilesOptions, done func(string, error)) {
		asked = append(asked, o)
		done(choice, nil)
	}
	app := show(t, s, s.worktreeSidebar)
	choose := func() {
		t.Helper()
		app.RightClick("id:project:" + repo.Path)
		app.ChoosePopup("Choose Icon…")
		waitFor(t, app, func() bool { return !s.choosingIcon })
		if o := asked[len(asked)-1]; o.Start != repo.Path || o.Multiple || len(o.Types) == 0 {
			t.Fatalf("picker options: %+v", o)
		}
	}

	// An image that does not decode leaves the placeholder.
	choice = broken
	choose()
	app.WaitFor("text:Could not use this image", time.Second)
	if repo.Icon != "" {
		t.Fatalf("broken image: icon %q", repo.Icon)
	}

	choice = logo
	choose()
	if repo.Icon != logo || s.projectIcons[logo] == nil || s.projectImage(repo) == nil {
		t.Fatalf("icon was not applied: %q", repo.Icon)
	}
	saved, err := loadSidebarState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if node := findProject(saved.Roots[root], repo.Path); node == nil || node.Icon != logo {
		t.Fatalf("icon was not saved: %+v", node)
	}

	// A restart reads the saved icon in the background.
	s.projectIcons = nil
	if s.projectImage(repo) != nil {
		t.Fatal("icon should load in the background")
	}
	waitFor(t, app, func() bool { return s.projectIcons[logo] != nil })

	app.RightClick("id:project:" + repo.Path)
	app.ChoosePopup("Reset Icon")
	if repo.Icon != "" || s.projectImage(repo) != nil {
		t.Fatal("icon was not reset")
	}
}
