package gitview

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type openedFile struct {
	path, data string
}

type savedFile struct {
	name, data string
}

// await receives one file action's result, failing the test when none comes.
func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the file action")
	}
	var zero T
	return zero
}

func TestViewFileMenus(t *testing.T) {
	repo := testRepo(t)
	g := Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	second, _ := g.Run(repo, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)

	opened := make(chan openedFile, 8)
	revealed := make(chan string, 8)
	saved := make(chan savedFile, 8)
	v, app, wait := startView(t, Options{
		Git: g,
		OpenFile: func(path string) error {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			opened <- openedFile{path, string(data)}
			return nil
		},
		RevealFile: func(path string) { revealed <- path },
		SaveFile: func(_, name string, data []byte) (string, error) {
			saved <- savedFile{name, string(data)}
			return filepath.Join(t.TempDir(), name), nil
		},
	})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && v.Syncing() == "" })

	// A worktree file opens itself and shows in the file manager.
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	menu := menuLabels(app)
	for _, item := range []string{"Open File", "Show in File Manager", "Stage", "Discard Changes…", "Copy File Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	app.ChoosePopup("Open File")
	if got := await(t, opened); got.path != filepath.Join(repo, "a.txt") || got.data != "3\n" {
		t.Fatalf("opened %+v", got)
	}
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	app.ChoosePopup("Show in File Manager")
	if got := await(t, revealed); got != filepath.Join(repo, "a.txt") {
		t.Fatalf("revealed %q", got)
	}

	// The second commit's Changes menu offers committing files' actions: its
	// version opens from a temporary copy, saves under the file's name and
	// shows in the Files tab.
	app.Press("id:all-commits")
	app.Press(commitRow("second"))
	wait(func() bool { return v.history.selected == second && v.history.open })
	app.Press("role:tab Changes")
	wait(func() bool {
		return v.history.changes.loaded && !v.history.changes.loading && !v.history.changes.fileLoading
	})
	app.RightClick(fileRow(AreaAll, "a.txt"))
	menu = menuLabels(app)
	for _, item := range []string{"Open File", "Show in File Manager", "Reset File to", "Save as…", "Show in Files", "Copy File Paths", "Copy Absolute Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	app.ChoosePopup("Copy File Paths")
	if app.Clipboard != "a.txt" {
		t.Fatalf("copied %q", app.Clipboard)
	}
	app.RightClick(fileRow(AreaAll, "a.txt"))
	app.ChoosePopup("Open File")
	if got := await(t, opened); filepath.Base(got.path) != "a.txt" || got.path == filepath.Join(repo, "a.txt") || got.data != "2\n" {
		t.Fatalf("opened %+v", got)
	}
	app.RightClick(fileRow(AreaAll, "a.txt"))
	app.ChoosePopup("Save as…")
	if got := await(t, saved); got.name != "a.txt" || got.data != "2\n" {
		t.Fatalf("saved %+v", got)
	}
	app.RightClick(fileRow(AreaAll, "a.txt"))
	app.ChoosePopup("Show in Files")
	wait(func() bool {
		return v.history.detail == detailFiles && v.history.files.loaded && !v.history.files.loading && !v.history.files.fileLoading
	})
	if v.history.files.selected != fileKey(FileChange{Path: "a.txt"}) || v.history.files.path != "a.txt" || v.history.files.text == nil ||
		*v.history.files.text != "2\n" {
		t.Fatalf("files tab: selected %q, path %q", v.history.files.selected, v.history.files.path)
	}

	// The Files tab's own menu saves and copies files but does not reset or
	// switch tabs.
	app.RightClick(fileRow(AreaAll, "a.txt"))
	menu = menuLabels(app)
	for _, item := range []string{"Open File", "Save as…", "Copy File Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from the Files menu %v", item, menu)
		}
	}
	for _, item := range []string{"Reset File to", "Show in Files"} {
		if contains(menu, item) {
			t.Fatalf("%q in the Files menu %v", item, menu)
		}
	}
	app.ClosePopup()

	// Resetting from Changes discards the worktree's version in favour of
	// the commit's, or the one before it.
	app.Press("role:tab Changes")
	wait(func() bool {
		return v.history.changes.loaded && !v.history.changes.loading && !v.history.changes.fileLoading
	})
	app.RightClick(fileRow(AreaAll, "a.txt"))
	app.ChoosePopup("Reset File to", "State at Commit…")
	short := v.repo.commits[commitIndex(v.repo.commits, second)].Short
	app.WaitFor("text:Reset a.txt to its state at "+short+"?", time.Second)
	app.Press("id:restore-dialog >> id:confirm")
	wait(func() bool { return !v.Loading() && v.Syncing() == "" })
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "2\n" {
		t.Fatalf("reset: %q", data)
	}

	app.RightClick(fileRow(AreaAll, "a.txt"))
	app.ChoosePopup("Reset File to", "State Before Commit…")
	if !v.restoring.open || !v.restoring.before {
		t.Fatalf("before dialog: %+v", v.restoring)
	}
	app.Press("id:restore-dialog >> id:confirm")
	wait(func() bool { return !v.Loading() && v.Syncing() == "" })
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "1\n" {
		t.Fatalf("before reset: %q", data)
	}
}

func TestViewFolderMenus(t *testing.T) {
	repo := testRepo(t)
	g := Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	os.MkdirAll(filepath.Join(repo, "sub"), 0o755)
	os.WriteFile(filepath.Join(repo, "sub", "b.txt"), []byte("b\n"), 0o644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "folder")
	added, _ := g.Run(repo, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(repo, "sub", "b.txt"), []byte("changed\n"), 0o644)

	opened := make(chan string, 8)
	revealed := make(chan string, 8)
	v, app, wait := startView(t, Options{
		Git:        g,
		OpenFile:   func(path string) error { opened <- path; return nil },
		RevealFile: func(path string) { revealed <- path },
	})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && v.Syncing() == "" })

	// A changed directory opens in the file manager and shows itself there
	// instead of standing for the files below it.
	app.RightClick(fileRow(AreaUnstaged, "sub"))
	menu := menuLabels(app)
	for _, item := range []string{"Open Folder", "Show in File Manager", "Stage", "Discard Changes…", "Stash 1 File…", "Copy File Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from the folder menu %v", item, menu)
		}
	}
	for _, item := range []string{"Open File", "Stage All Changes", "Save as…"} {
		if contains(menu, item) {
			t.Fatalf("%q in the folder menu %v", item, menu)
		}
	}
	app.ChoosePopup("Open Folder")
	if got := await(t, opened); got != filepath.Join(repo, "sub") {
		t.Fatalf("opened %q", got)
	}
	app.RightClick(fileRow(AreaUnstaged, "sub"))
	app.ChoosePopup("Show in File Manager")
	if got := await(t, revealed); got != filepath.Join(repo, "sub") {
		t.Fatalf("revealed %q", got)
	}

	// A commit's Files tab offers the same for its directory rows, leaving
	// out the items only one file has.
	app.Press("id:all-commits")
	app.Press(commitRow("folder"))
	wait(func() bool { return v.history.selected == added && v.history.open })
	app.Press("role:tab Files")
	wait(func() bool {
		return v.history.files.loaded && !v.history.files.loading && !v.history.files.fileLoading
	})
	app.RightClick(fileRow(AreaAll, "sub"))
	menu = menuLabels(app)
	for _, item := range []string{"Open Folder", "Show in File Manager", "Copy File Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from the Files folder menu %v", item, menu)
		}
	}
	for _, item := range []string{"Open File", "Save as…", "Show in Files"} {
		if contains(menu, item) {
			t.Fatalf("%q in the Files folder menu %v", item, menu)
		}
	}
	app.ChoosePopup("Open Folder")
	if got := await(t, opened); got != filepath.Join(repo, "sub") {
		t.Fatalf("opened %q from the Files tab", got)
	}

	// A folder the worktree no longer has opens the nearest directory there.
	if err := os.RemoveAll(filepath.Join(repo, "sub")); err != nil {
		t.Fatal(err)
	}
	app.RightClick(fileRow(AreaAll, "sub"))
	app.ChoosePopup("Open Folder")
	if got := await(t, opened); got != repo {
		t.Fatalf("opened %q for the folder gone from the worktree", got)
	}
}
