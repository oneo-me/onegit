package gitview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetCherryPickAndRevert(t *testing.T) {
	t.Parallel()
	g := Git{Env: append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")}
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "one")
	first, _ := g.Run(repo, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "two")

	// A soft reset stages the difference, a mixed one unstages it, and a
	// hard one drops it.
	if err := g.Reset(repo, first, ResetSoft); err != nil {
		t.Fatal(err)
	}
	if status, _ := g.Run(repo, "status", "--porcelain"); status != "M  a.txt" {
		t.Fatalf("soft: %q", status)
	}
	if err := g.Reset(repo, first, ResetMixed); err != nil {
		t.Fatal(err)
	}
	if status, _ := g.Run(repo, "status", "--porcelain"); status != "M a.txt" {
		t.Fatalf("mixed: %q", status)
	}
	if staged, _ := g.Run(repo, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("mixed staged: %q", staged)
	}
	if err := g.Reset(repo, first, ResetHard); err != nil {
		t.Fatal(err)
	}
	if status, _ := g.Run(repo, "status", "--porcelain"); status != "" {
		t.Fatalf("hard: %q", status)
	}
	if staged, _ := g.Run(repo, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("staged after hard reset: %q", staged)
	}

	// A cherry-pick applies another branch's commit, optionally without
	// recording it and with the origin in its message.
	git(t, repo, "switch", "-q", "-c", "topic")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "topic")
	topic, _ := g.Run(repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "main")
	if err := g.CherryPick(repo, topic, CherryPickOptions{}); err != nil {
		t.Fatal(err)
	}
	if head, _ := g.Run(repo, "rev-parse", "HEAD"); head != first || len(areaFilesForTest(t, g, repo, AreaStaged)) != 1 {
		t.Fatalf("no-commit cherry-pick at %s", head)
	}
	git(t, repo, "reset", "-q", "--hard", first)
	if err := g.CherryPick(repo, topic, CherryPickOptions{Commit: true, AppendOrigin: true}); err != nil {
		t.Fatal(err)
	}
	if message, _ := g.Run(repo, "log", "-1", "--format=%B"); !strings.Contains(message, "topic") || !strings.Contains(message, "cherry picked from commit") {
		t.Fatalf("message: %q", message)
	}

	// A revert undoes that commit again.
	if err := g.Revert(repo, topic, RevertOptions{Commit: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("reverted: %v", err)
	}
	if subject, _ := g.Run(repo, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "Revert") {
		t.Fatalf("revert commit: %q", subject)
	}
}

func TestStashFilesRenameAndDrop(t *testing.T) {
	t.Parallel()
	g := Git{}
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0644)
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("other\n"), 0644)

	// A partial stash saves only the chosen file.
	if err := g.SaveStashFiles(repo, "part", []FileChange{{Path: "a.txt"}}); err != nil {
		t.Fatal(err)
	}
	stashes, err := g.Stashes(repo)
	if err != nil || len(stashes) != 1 || stashes[0].Message != "On main: part" {
		t.Fatalf("stashes %+v %v", stashes, err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "1\n" {
		t.Fatalf("a.txt: %q", data)
	}
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); err != nil {
		t.Fatal(err)
	}
	// Renaming keeps its changes and drops the old entry, and dropping
	// removes it.
	renamed, err := g.RenameStash(repo, stashes[0].Hash, "renamed")
	if err != nil {
		t.Fatal(err)
	}
	stashes, _ = g.Stashes(repo)
	if len(stashes) != 1 || stashes[0].Hash != renamed || stashes[0].Message != "renamed" {
		t.Fatalf("renamed %+v", stashes)
	}
	if err := g.DropStash(repo, renamed); err != nil {
		t.Fatal(err)
	}
	if stashes, _ := g.Stashes(repo); len(stashes) != 0 {
		t.Fatalf("kept %+v", stashes)
	}
}

func TestRebaseCheckAndDependentBranches(t *testing.T) {
	t.Parallel()
	g := Git{Env: append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")}
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "base")
	git(t, repo, "switch", "-q", "-c", "topic")
	os.WriteFile(filepath.Join(repo, "topic.txt"), []byte("topic\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "topic")
	git(t, repo, "branch", "side")
	git(t, repo, "switch", "-q", "main")
	os.WriteFile(filepath.Join(repo, "main.txt"), []byte("main\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "main work")

	// The branch in the replay range is offered for --update-refs, and the
	// rebase is expected to apply cleanly. The checked out branch, which is
	// the one rebased here, stays out.
	git(t, repo, "switch", "-q", "topic")
	branches, err := g.LocalBranchesInRange(repo, "refs/heads/topic", "refs/heads/main")
	if err != nil || len(branches) != 1 || branches[0] != "side" {
		t.Fatalf("branches %v %v", branches, err)
	}
	check, err := g.CheckRebase(repo, "refs/heads/topic", "refs/heads/main")
	if err != nil || check == RebaseCheckConflict {
		t.Fatalf("clean check %v %v", check, err)
	}

	// A conflicting change on both branches is expected to conflict.
	git(t, repo, "switch", "-q", "topic")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("topic\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "topic edit")
	git(t, repo, "switch", "-q", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("main\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "main edit")
	if check, _ := g.CheckRebase(repo, "refs/heads/topic", "refs/heads/main"); check == RebaseCheckUnknown {
		t.Skip("git replay is not available")
	} else if check != RebaseCheckConflict {
		t.Fatalf("conflicting check %v", check)
	}
}

func TestCommitAndStashPatches(t *testing.T) {
	t.Parallel()
	g := Git{}
	repo := testRepo(t)
	hash, _ := g.Run(repo, "rev-parse", "HEAD")
	patch, err := g.CommitPatch(repo, hash)
	if err != nil || !strings.Contains(string(patch), "Subject: [PATCH] init") {
		t.Fatalf("commit patch %q %v", patch, err)
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0644)
	if err := g.SaveStash(repo, "work", false); err != nil {
		t.Fatal(err)
	}
	stashes, _ := g.Stashes(repo)
	patch, err = g.StashPatch(repo, stashes[0].Hash)
	if err != nil || !strings.Contains(string(patch), "changed") {
		t.Fatalf("stash patch %q %v", patch, err)
	}
}

// areaFilesForTest lists the files of area in the worktree at repo.
func areaFilesForTest(t *testing.T, g Git, repo string, area Area) []FileChange {
	t.Helper()
	changes, err := g.WorktreeStatus(repo)
	if err != nil {
		t.Fatal(err)
	}
	var files []FileChange
	for _, f := range changes.Files {
		if f.Area == area {
			files = append(files, f)
		}
	}
	return files
}

func TestRebaseMovesDependentBranches(t *testing.T) {
	t.Parallel()
	g := Git{Env: append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")}
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	git(t, repo, "switch", "-q", "-c", "feature")
	os.WriteFile(filepath.Join(repo, "f.txt"), []byte("f\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "feature")
	git(t, repo, "switch", "-q", "main")
	os.WriteFile(filepath.Join(repo, "m.txt"), []byte("m\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "main work")
	git(t, repo, "branch", "side")
	if err := g.Rebase(repo, "refs/heads/feature", IntegrateOptions{UpdateRefs: true, Stash: true}); err != nil {
		t.Fatal(err)
	}
	main, _ := g.Run(repo, "rev-parse", "HEAD")
	side, _ := g.Run(repo, "rev-parse", "side")
	if main != side {
		t.Fatalf("side %s, main %s", side, main)
	}
}

func TestPushTag(t *testing.T) {
	t.Parallel()
	g := Git{}
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	repo := filepath.Join(dir, "repo")
	git(t, dir, "clone", "-q", origin, repo)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	git(t, repo, "tag", "v1")
	if err := g.PushTag(repo, "origin", "v1"); err != nil {
		t.Fatal(err)
	}
	if tags, _ := g.Run(origin, "tag"); tags != "v1" {
		t.Fatalf("remote tags %q", tags)
	}
}
