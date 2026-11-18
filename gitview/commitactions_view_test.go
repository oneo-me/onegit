package gitview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	v, app, wait := startView(t, Options{
		Git: identity,
		SaveFile: func(_, name string, data []byte) (string, error) {
			saved = data
			close(savedPatch)
			return filepath.Join(t.TempDir(), name), nil
		},
	})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch == "main" }
	wait(ready)
	initHash, _ := v.git.Run(repo, "rev-parse", "HEAD~1")
	topicHash, _ := v.git.Run(repo, "rev-parse", "topic")

	// A commit's menu offers the history actions Fork keeps there.
	app.RightClick(commitRow("second"))
	menu := menuLabels(app)
	for _, item := range []string{"Checkout Commit…", "Reset 'main' to Here…", "Cherry-pick…", "Revert…", "Save as Patch…", "Copy Commit Info"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	app.ClosePopup()

	// Resetting to an older commit asks for the reset type first.
	app.RightClick(commitRow("init"))
	app.ChoosePopup("Reset 'main' to Here…")
	app.WaitFor("text:Reset Current Branch to Revision", time.Second)
	app.Press("label:Hard — Discard all local changes")
	app.Press("id:reset-dialog >> id:confirm")
	wait(func() bool { return ready() && v.repo.head == initHash })
	if status, _ := v.git.Run(repo, "status", "--porcelain"); status != "" {
		t.Fatalf("reset left changes: %q", status)
	}

	// Cherry-picking another branch's commit records it, appending its
	// origin.
	app.RightClick(commitRow("topic work"))
	app.ChoosePopup("Cherry-pick…")
	app.WaitFor("label:Commit the changes", time.Second)
	app.Press("label:Append origin to commit message")
	app.Press("id:cherry-pick-dialog >> id:confirm")
	wait(func() bool { return ready() && v.repo.head != initHash })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); subject != "topic work" {
		t.Fatalf("cherry-picked %q", subject)
	}
	if body, _ := v.git.Run(repo, "log", "-1", "--format=%B"); !strings.Contains(body, "cherry picked from commit "+topicHash) {
		t.Fatalf("origin missing: %q", body)
	}

	// A revert records the opposite commit.
	app.RightClick(commitRow("topic work") + " >> nth=0")
	app.ChoosePopup("Revert…")
	app.WaitFor("id:revert-dialog", time.Second)
	head := v.repo.head
	app.Press("id:revert-dialog >> id:confirm")
	wait(func() bool { return ready() && v.repo.head != head })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "Revert") {
		t.Fatalf("reverted %q", subject)
	}

	// Copying a commit's summary and saving its patch.
	app.RightClick(commitRow("init"))
	app.ChoosePopup("Copy Commit Info")
	if got := app.Clipboard; !strings.Contains(got, " - init") {
		t.Fatalf("copied %q", got)
	}
	app.RightClick(commitRow("init"))
	app.ChoosePopup("Save as Patch…")
	select {
	case <-savedPatch:
		if !strings.Contains(string(saved), "Subject: [PATCH] init") {
			t.Fatalf("patch: %q", saved)
		}
	case <-time.After(10 * time.Second):
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
	v, app, wait := startView(t, Options{Git: identity, OnState: func(s State) { saved = s }})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch == "main" }
	wait(ready)

	// The rebase dialog names both branches, offers the branches the rebase
	// moves and reports the expected conflicts.
	app.RightClick("id:ref-branches/feature")
	app.ChoosePopup("Rebase 'main' on 'feature'…")
	wait(func() bool { return !v.rebase.loading })
	if len(v.rebase.branches) != 1 || v.rebase.branches[0] != "side" {
		t.Fatalf("branches: %v", v.rebase.branches)
	}
	for _, text := range []string{"text:Rebase:", "text:On:", "label:Update dependent branches", "label:Stash and reapply local changes"} {
		app.WaitFor(text, time.Second)
	}
	if v.rebase.check == RebaseCheckClean {
		app.WaitFor("text:Rebase can be done without conflicts", time.Second)
	}
	app.Press("label:Update dependent branches")
	app.WaitFor("id:rebase-dialog >> text:/^side$/", time.Second)
	app.Press("label:Stash and reapply local changes")
	if !saved.RebaseUpdateRefs || !saved.RebaseAutostash {
		t.Fatalf("state %+v", saved)
	}
	if !v.rebase.updateRefs || !v.rebase.autostash || len(v.rebase.branches) != 1 {
		t.Fatalf("dialog %+v", v.rebase)
	}
	app.Press("id:rebase-dialog >> id:confirm")
	wait(func() bool {
		log, _ := v.git.Run(repo, "log", "--format=%s", "main")
		return ready() && log == "main work\nfeature work\ninit"
	})
	if side, _ := v.git.Run(repo, "rev-parse", "side"); side != v.repo.head {
		log, _ := v.git.Run(repo, "log", "--oneline", "--all", "--decorate")
		t.Fatalf("side at %s, main at %s: %s", side, v.repo.head, log)
	}
}

func TestViewStashMenu(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0644)
	git(t, repo, "stash", "push", "-q", "-m", "work")
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && len(v.repo.stashes) == 1 })
	stash := "id:stash-" + v.repo.stashes[0].Hash

	// A stash's menu applies, renames or deletes it, or saves its patch.
	app.RightClick(stash)
	menu := menuLabels(app)
	for _, item := range []string{"Apply…", "Rename…", "Save as Patch…", "Delete…"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	app.ClosePopup()

	// Renaming stores the same changes under the new message.
	app.RightClick(stash)
	app.ChoosePopup("Rename…")
	app.WaitFor("text:Rename Stash", time.Second)
	app.Type("id:stash-rename-message", "-renamed")
	app.Press("id:stash-rename-dialog >> id:confirm")
	wait(func() bool {
		return !v.Loading() && len(v.repo.stashes) == 1 && strings.HasSuffix(v.repo.stashes[0].Message, "renamed")
	})

	// Deleting asks first and leaves no stash.
	app.RightClick("id:stash-" + v.repo.stashes[0].Hash)
	app.ChoosePopup("Delete…")
	app.WaitFor("text:Delete stash?", time.Second)
	app.Press("id:stash-delete-dialog >> id:confirm")
	wait(func() bool { return !v.Loading() && len(v.repo.stashes) == 0 })
}

func TestViewStashesChosenFilesAndCopiesPaths(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0644)
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("other\n"), 0644)
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	shown := func(files int) func() bool {
		return func() bool { return !v.Loading() && v.Syncing() == "" && len(v.current.changes.Files) == files }
	}
	wait(shown(2))

	// The file menu copies the paths and can stash just that file.
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	menu := menuLabels(app)
	for _, item := range []string{"Stash 1 File…", "Copy File Paths", "Copy Absolute Paths"} {
		if !contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	app.ChoosePopup("Copy File Paths")
	if app.Clipboard != "a.txt" {
		t.Fatalf("copied %q", app.Clipboard)
	}
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	app.ChoosePopup("Stash 1 File…")
	app.WaitFor("text:Stash Files", time.Second)
	if !v.stashDialog.partial {
		t.Fatal("the stash dialog is not partial")
	}
	app.Type("id:stash-message", "partial")
	app.Press("id:stash-dialog >> id:confirm")
	wait(func() bool { return shown(1)() && len(v.repo.stashes) == 1 })
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
