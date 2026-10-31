package gitview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestViewCommitActions(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	git(t, repo, "switch", "-q", "-c", "topic", "HEAD~1")
	os.WriteFile(filepath.Join(repo, "topic.txt"), []byte("topic\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "topic work")
	git(t, repo, "switch", "-q", "main")
	var saved []byte
	savedPatch := make(chan struct{})
	v, pump := testView(t, Options{
		Git: Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"),
		SaveFile: func(_, name string, data []byte) (string, error) {
			saved = data
			close(savedPatch)
			return filepath.Join(t.TempDir(), name), nil
		},
	})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch == "main" }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	initHash, _ := v.git.Run(repo, "rev-parse", "HEAD~1")
	topicHash, _ := v.git.Run(repo, "rev-parse", "topic")

	// A commit's menu offers the history actions Fork keeps there.
	if err := view.RightClick("second"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	for _, item := range []string{"Checkout Commit…", "Reset 'main' to Here…", "Cherry-pick…", "Revert…", "Save as Patch…", "Copy Commit Info"} {
		if !strings.Contains(strings.Join(menu, "\n"), item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	view.CloseMenu()

	// Resetting to an older commit asks for the reset type first.
	view.Advance(time.Second)
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Reset 'main' to Here…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.reset.open || !view.HasText("Reset Current Branch to Revision") || !view.HasText("Hard — Discard all local changes") {
		t.Fatalf("reset dialog: %v", view.Texts())
	}
	if err := view.Click("Hard — Discard all local changes"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.repo.head == initHash })
	if status, _ := v.git.Run(repo, "status", "--porcelain"); status != "" {
		t.Fatalf("reset left changes: %q", status)
	}

	// Cherry-picking another branch's commit records it, appending its
	// origin.
	view.Advance(time.Second)
	if err := view.RightClick("topic work"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Cherry-pick…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.cherryPick.open || !view.HasText("Cherry Pick") || !view.HasText("Commit the changes") {
		t.Fatalf("cherry dialog: %v", view.Texts())
	}
	if err := view.Click("Append origin to commit message"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.Click("Cherry Pick"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.repo.head != initHash })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); subject != "topic work" {
		t.Fatalf("cherry-picked %q", subject)
	}
	if body, _ := v.git.Run(repo, "log", "-1", "--format=%B"); !strings.Contains(body, "cherry picked from commit "+topicHash) {
		t.Fatalf("origin missing: %q", body)
	}

	// A revert records the opposite commit.
	view.Advance(time.Second)
	if err := view.RightClick("topic work"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Revert…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.revert.open || !view.HasText("Revert") {
		t.Fatalf("revert dialog: %v", view.Texts())
	}
	if err := view.Click("Revert"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "Revert") {
		t.Fatalf("reverted %q", subject)
	}

	// Copying a commit's summary and saving its patch.
	view.Advance(time.Second)
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Copy Commit Info"); err != nil {
		t.Fatal(err)
	}
	if got := view.Clipboard(); !strings.Contains(got, " - init") {
		t.Fatalf("copied %q", got)
	}
	view.Advance(time.Second)
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Save as Patch…"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	select {
	case <-savedPatch:
		if !strings.Contains(string(saved), "Subject: [PATCH] init") {
			t.Fatalf("patch: %q", saved)
		}
	case <-deadline:
		t.Fatal("timed out saving the patch")
	}
}

func TestViewRebaseDialog(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "feature")
	git(t, repo, "switch", "-q", "feature")
	os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "feature work")
	git(t, repo, "switch", "-q", "main")
	os.WriteFile(filepath.Join(repo, "main.txt"), []byte("main\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "main work")
	// The branch points into the commits the rebase replays, so it moves too.
	git(t, repo, "branch", "side")

	var saved State
	v, pump := testView(t, Options{
		Git:     Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"),
		OnState: func(s State) { saved = s },
	})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch == "main" }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// The rebase dialog names both branches, offers the branches the rebase
	// moves and reports the expected conflicts.
	if err := view.RightClick("feature"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Rebase 'main' on 'feature'…"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.rebase.loading })
	view.Frame()
	if len(v.rebase.branches) != 1 || v.rebase.branches[0] != "side" {
		t.Fatalf("branches: %v", v.rebase.branches)
	}
	for _, text := range []string{"Rebase:", "On:", "Update dependent branches", "side", "Stash and reapply local changes"} {
		if !view.HasText(text) {
			t.Fatalf("%q missing from %v", text, view.Texts())
		}
	}
	if v.rebase.check == RebaseCheckClean && !view.HasText("Rebase can be done without conflicts") {
		t.Fatalf("clean check missing from %v", view.Texts())
	}
	if err := view.Click("Update dependent branches"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.Click("Stash and reapply local changes"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !saved.RebaseUpdateRefs || !saved.RebaseAutostash {
		t.Fatalf("state %+v", saved)
	}
	if !v.rebase.updateRefs || !v.rebase.autostash || len(v.rebase.branches) != 1 {
		t.Fatalf("dialog %+v", v.rebase)
	}
	if err := view.Click("Rebase"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && len(v.repo.commits) > 0 })
	if log, _ := v.git.Run(repo, "log", "--format=%s", "main"); log != "main work\nfeature work\ninit" {
		t.Fatalf("main log: %q", log)
	}
	if side, _ := v.git.Run(repo, "rev-parse", "side"); side != v.repo.head {
		log, _ := v.git.Run(repo, "log", "--oneline", "--all", "--decorate")
		t.Fatalf("side at %s, main at %s: %s", side, v.repo.head, log)
	}
}

func TestViewStashMenu(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0644)
	git(t, repo, "stash", "push", "-q", "-m", "work")
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() && len(v.repo.stashes) == 1 })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	stash := "On main: work"

	// A stash's menu applies, renames or deletes it, or saves its patch.
	if err := view.RightClick(stash); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	for _, item := range []string{"Apply…", "Rename…", "Save as Patch…", "Delete…"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	view.CloseMenu()
	view.Advance(time.Second)

	// Renaming stores the same changes under the new message.
	if err := view.RightClick(stash); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Rename…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.stashRename.open || !view.HasText("Rename Stash") {
		t.Fatalf("rename dialog: %v", view.Texts())
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("-renamed")
	view.Frame()
	if err := view.Click("Rename"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
		return !v.Loading() && len(v.repo.stashes) == 1 && strings.HasSuffix(v.repo.stashes[0].Message, "renamed")
	})
	view.Frame()

	// Deleting asks first and leaves no stash.
	renamed := v.repo.stashes[0].Message
	if err := view.RightClick(renamed); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Delete…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.stashDeletion.open || !view.HasText("Delete stash?") {
		t.Fatalf("delete dialog: %v", view.Texts())
	}
	if err := view.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.Loading() && len(v.repo.stashes) == 0 })
}

func TestViewStashesChosenFilesAndCopiesPaths(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0644)
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("other\n"), 0644)
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	shown := func(files int) func() bool {
		return func() bool { return !v.Loading() && v.Syncing() == "" && len(v.current.changes.Files) == files }
	}
	pump(shown(2))
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// The file menu copies the paths and can stash just that file.
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	for _, item := range []string{"Stash 1 File…", "Copy File Paths", "Copy Absolute Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	view.CloseMenu()
	view.Advance(time.Second)
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
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
	if err := view.ChooseMenuItem("Stash 1 File…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.stashDialog.partial || !view.HasText("Stash Files") {
		t.Fatalf("stash dialog: %v", view.Texts())
	}
	view.Type("partial")
	view.Frame()
	if err := view.Click("Save Stash"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return shown(1)() && len(v.repo.stashes) == 1 })
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); err != nil {
		t.Fatal(err)
	}
	if status, _ := v.git.Run(repo, "status", "--porcelain"); !strings.Contains(status, "b.txt") {
		t.Fatalf("status: %q", status)
	}
}

// contains reports whether list holds item.
func contains(list []string, item string) bool {
	for _, got := range list {
		if got == item {
			return true
		}
	}
	return false
}
