package gitview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
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
	v, pump := testView(t, Options{
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
	pump(func() bool { return !v.Loading() && v.Syncing() == "" })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// A worktree file opens itself and shows in the file manager.
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	for _, item := range []string{"Open File", "Show in File Manager", "Stage", "Discard Changes…", "Copy File Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	if err := view.ChooseMenuItem("Open File"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, opened); got.path != filepath.Join(repo, "a.txt") || got.data != "3\n" {
		t.Fatalf("opened %+v", got)
	}
	view.Advance(time.Second)
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Show in File Manager"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, revealed); got != filepath.Join(repo, "a.txt") {
		t.Fatalf("revealed %q", got)
	}

	// The second commit's Changes menu offers committing files' actions: its
	// version opens from a temporary copy, saves under the file's name and
	// shows in the Files tab.
	view.Advance(time.Second)
	if err := view.Click("All commits"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	if err := view.Click("second"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.history.selected == second && v.history.open })
	if err := view.Click("Changes"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
		return v.history.changes.loaded && !v.history.changes.loading && !v.history.changes.fileLoading
	})
	view.Frame()
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	menu = view.Menu()
	for _, item := range []string{"Open File", "Show in File Manager", "Reset File to", "Save as…", "Show in Files", "Copy File Paths", "Copy Absolute Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	if err := view.ChooseMenuItem("Copy File Paths"); err != nil {
		t.Fatal(err)
	}
	if view.Clipboard() != "a.txt" {
		t.Fatalf("copied %q", view.Clipboard())
	}
	view.Advance(time.Second)
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Open File"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, opened); filepath.Base(got.path) != "a.txt" || got.path == filepath.Join(repo, "a.txt") || got.data != "2\n" {
		t.Fatalf("opened %+v", got)
	}
	view.Advance(time.Second)
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Save as…"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, saved); got.name != "a.txt" || got.data != "2\n" {
		t.Fatalf("saved %+v", got)
	}
	view.Advance(time.Second)
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Show in Files"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
		return v.history.detail == detailFiles && v.history.files.loaded && !v.history.files.loading && !v.history.files.fileLoading
	})
	view.Frame()
	if v.history.files.selected != fileKey(FileChange{Path: "a.txt"}) || v.history.files.path != "a.txt" || v.history.files.view == nil ||
		v.history.files.view.Buffer.String() != "2\n" {
		t.Fatalf("files tab: selected %q, path %q", v.history.files.selected, v.history.files.path)
	}

	// The Files tab's own menu saves and copies files but does not reset or
	// switch tabs.
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	menu = view.Menu()
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
	view.CloseMenu()

	// Resetting from Changes discards the worktree's version in favour of
	// the commit's, or the one before it.
	view.Advance(time.Second)
	if err := view.Click("Changes"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
		return v.history.changes.loaded && !v.history.changes.loading && !v.history.changes.fileLoading
	})
	view.Frame()
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Reset File to", "State at Commit…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	short := v.repo.commits[commitIndex(v.repo.commits, second)].Short
	if !v.restoring.open || !strings.Contains(strings.Join(view.Texts(), "\n"), "Reset a.txt to its state at "+short+"?") {
		t.Fatalf("reset dialog: %v", view.Texts())
	}
	if err := view.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.Loading() && v.Syncing() == "" })
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "2\n" {
		t.Fatalf("reset: %q", data)
	}

	view.Frame()
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Reset File to", "State Before Commit…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.restoring.open || !v.restoring.before {
		t.Fatalf("before dialog: %+v", v.restoring)
	}
	if err := view.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.Loading() && v.Syncing() == "" })
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
	v, pump := testView(t, Options{
		Git:        g,
		OpenFile:   func(path string) error { opened <- path; return nil },
		RevealFile: func(path string) { revealed <- path },
	})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() && v.Syncing() == "" })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()

	// A changed directory opens in the file manager and shows itself there
	// instead of standing for the files below it.
	if err := view.RightClick("sub"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
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
	if err := view.ChooseMenuItem("Open Folder"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, opened); got != filepath.Join(repo, "sub") {
		t.Fatalf("opened %q", got)
	}
	view.Advance(time.Second)
	if err := view.RightClick("sub"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Show in File Manager"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, revealed); got != filepath.Join(repo, "sub") {
		t.Fatalf("revealed %q", got)
	}

	// A commit's Files tab offers the same for its directory rows, leaving
	// out the items only one file has.
	view.Advance(time.Second)
	if err := view.Click("All commits"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	if err := view.Click("folder"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.history.selected == added && v.history.open })
	if err := view.Click("Files"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
		return v.history.files.loaded && !v.history.files.loading && !v.history.files.fileLoading
	})
	view.Frame()
	if err := view.RightClick("sub"); err != nil {
		t.Fatal(err)
	}
	menu = view.Menu()
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
	if err := view.ChooseMenuItem("Open Folder"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, opened); got != filepath.Join(repo, "sub") {
		t.Fatalf("opened %q from the Files tab", got)
	}

	// A folder the worktree no longer has opens the nearest directory there.
	view.Advance(time.Second)
	if err := os.RemoveAll(filepath.Join(repo, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := view.RightClick("sub"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Open Folder"); err != nil {
		t.Fatal(err)
	}
	if got := await(t, opened); got != repo {
		t.Fatalf("opened %q for the folder gone from the worktree", got)
	}
}
