package gitview

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInteractiveRebaseRewordAndDrop(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "third")
	g := Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	base, _ := g.Run(repo, "rev-parse", "HEAD~2")
	commits, err := g.RebaseCommits(repo, base)
	if err != nil || len(commits) != 2 {
		t.Fatalf("commits %d: %v", len(commits), err)
	}
	if err := g.InteractiveRebase(repo, base, []RebaseCommit{
		{Commit: commits[0], Action: RebaseReword, Message: "second reworded"},
		{Commit: commits[1], Action: RebaseDrop},
	}, false); err != nil {
		t.Fatal(err)
	}
	log, _ := g.Run(repo, "log", "--format=%s")
	if log != "second reworded\ninit" {
		t.Fatalf("log %q", log)
	}
	if got, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(got) != "2\n" {
		t.Fatalf("file %q", got)
	}
}

func TestInteractiveRebaseSquash(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "third")
	g := Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	base, _ := g.Run(repo, "rev-parse", "HEAD~2")
	commits, err := g.RebaseCommits(repo, base)
	if err != nil || len(commits) != 2 {
		t.Fatalf("commits %d: %v", len(commits), err)
	}
	if err := g.InteractiveRebase(repo, base, []RebaseCommit{
		{Commit: commits[0], Action: RebasePick},
		{Commit: commits[1], Action: RebaseSquash},
	}, false); err != nil {
		t.Fatal(err)
	}
	log, _ := g.Run(repo, "log", "--format=%s")
	if log != "second\ninit" {
		t.Fatalf("log %q", log)
	}
	if got, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(got) != "3\n" {
		t.Fatalf("file %q", got)
	}
}
