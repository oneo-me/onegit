package gitview

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOperationConflictResolveAndContinue(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "topic")
	git(t, repo, "switch", "-q", "topic")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("topic\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "topic")
	git(t, repo, "switch", "-q", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("main\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "main")
	g := Git{}
	if err := g.Merge(repo, "refs/heads/topic", IntegrateOptions{}); err == nil {
		t.Fatal("merge did not conflict")
	}
	op, err := g.Operation(repo)
	if err != nil || op.Kind != OperationMerge {
		t.Fatalf("operation %+v: %v", op, err)
	}
	changes, err := g.WorktreeStatus(repo)
	if err != nil {
		t.Fatal(err)
	}
	found := slices.ContainsFunc(changes.Files, func(f FileChange) bool { return f.Path == "a.txt" && f.Status == 'U' })
	if !found {
		t.Fatalf("conflict not listed: %+v", changes.Files)
	}
	if err := g.ResolveConflict(repo, "a.txt", ConflictOurs); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "main\n" {
		t.Fatalf("resolved file %q", data)
	}
	if err := g.ContinueOperation(repo); err != nil {
		t.Fatal(err)
	}
	if op, _ := g.Operation(repo); op.Kind != OperationNone {
		t.Fatalf("operation remains %+v", op)
	}
	if subject, _ := g.Run(repo, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "Merge") {
		t.Fatalf("merge commit %q", subject)
	}
}

func TestOperationAbort(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "topic")
	git(t, repo, "switch", "-q", "topic")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("topic\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "topic")
	git(t, repo, "switch", "-q", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("main\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "main")
	g := Git{}
	before, _ := g.Run(repo, "rev-parse", "HEAD")
	if err := g.Merge(repo, "refs/heads/topic", IntegrateOptions{}); err == nil {
		t.Fatal("merge did not conflict")
	}
	if err := g.AbortOperation(repo); err != nil {
		t.Fatal(err)
	}
	if op, _ := g.Operation(repo); op.Kind != OperationNone {
		t.Fatalf("operation remains %+v", op)
	}
	if head, _ := g.Run(repo, "rev-parse", "HEAD"); head != before {
		t.Fatalf("head %q, want %q", head, before)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "main\n" {
		t.Fatalf("file %q", data)
	}
}

func TestDiscardText(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n2\nthree\n"), 0644)
	g := Git{}
	if err := g.DiscardText(repo, "a.txt", []byte("1\n2\nthree\n")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "1\n2\nthree\n" {
		t.Fatalf("file %q", data)
	}
}

func TestInspectHistoryBlameSearchAndCompare(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("needle\n"), 0644)
	git(t, repo, "add", "b.txt")
	git(t, repo, "commit", "-q", "-m", "third")
	g := Git{}
	history, err := g.FileHistory(repo, "a.txt")
	if err != nil || len(history) != 2 || history[0].Subject != "second" || history[1].Subject != "init" {
		t.Fatalf("history %+v: %v", history, err)
	}
	lines, err := g.Blame(repo, "", "a.txt")
	if err != nil || len(lines) == 0 || lines[0].Hash != history[0].Hash || lines[0].Text != "two" {
		t.Fatalf("blame %+v: %v", lines, err)
	}
	for _, search := range []SearchOptions{
		{Type: SearchContent, Scope: SearchRepository, Query: "needle"},
		{Type: SearchPath, Scope: SearchRepository, Query: "b.txt"},
		{Type: SearchMessage, Scope: SearchRepository, Query: "third"},
		{Type: SearchAuthor, Scope: SearchRepository, Query: "t"},
	} {
		found, err := g.SearchCommits(repo, search)
		if err != nil || len(found) == 0 {
			t.Fatalf("search %+v: %+v, %v", search, found, err)
		}
		if search.Type == SearchContent || search.Type == SearchPath {
			if found[0].Subject != "third" {
				t.Fatalf("search %+v: %+v", search, found[0])
			}
		}
	}
	initHash, _ := g.Run(repo, "rev-parse", "HEAD~2")
	headHash, _ := g.Run(repo, "rev-parse", "HEAD")
	changes, err := g.DiffBetween(repo, initHash, headHash)
	if err != nil || len(changes.Files) != 2 {
		t.Fatalf("compare %+v: %v", changes.Files, err)
	}
	os.WriteFile(filepath.Join(repo, "c.txt"), []byte("new\n"), 0644)
	changes, err = g.DiffBetween(repo, headHash, "")
	if err != nil || !slices.ContainsFunc(changes.Files, func(f FileChange) bool { return f.Path == "c.txt" }) {
		t.Fatalf("worktree compare %+v: %v", changes.Files, err)
	}
	// A reset leaves the commit reachable only from the reflog.
	if _, err := g.Run(repo, "reset", "--hard", "HEAD~1"); err != nil {
		t.Fatal(err)
	}
	reflog, err := g.Reflog(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(reflog, func(c Commit) bool { return c.Hash == headHash }) {
		t.Fatalf("lost commit missing from reflog")
	}
}

func TestMergeRecordsSquashAndNoCommit(t *testing.T) {
	newRepo := func() (string, string) {
		repo := testRepo(t)
		git(t, repo, "branch", "topic")
		git(t, repo, "switch", "-q", "topic")
		os.WriteFile(filepath.Join(repo, "b.txt"), []byte("topic\n"), 0644)
		git(t, repo, "add", "b.txt")
		git(t, repo, "commit", "-q", "-m", "topic")
		git(t, repo, "switch", "-q", "main")
		os.WriteFile(filepath.Join(repo, "c.txt"), []byte("main\n"), 0644)
		git(t, repo, "add", "c.txt")
		git(t, repo, "commit", "-q", "-m", "main")
		return repo, "refs/heads/topic"
	}
	g := Git{}
	repo, topic := newRepo()
	if err := g.Merge(repo, topic, IntegrateOptions{Squash: true}); err != nil {
		t.Fatal(err)
	}
	if op, _ := g.Operation(repo); op.Kind != OperationSquash {
		t.Fatalf("squash operation %+v", op)
	}
	if count, _ := g.Run(repo, "rev-list", "--count", "HEAD"); count != "2" {
		t.Fatalf("commits %q", count)
	}
	if err := g.ContinueOperation(repo); err != nil {
		t.Fatal(err)
	}
	if count, _ := g.Run(repo, "rev-list", "--count", "HEAD"); count != "3" {
		t.Fatalf("squash commit %q", count)
	}

	repo, topic = newRepo()
	if err := g.Merge(repo, topic, IntegrateOptions{NoCommit: true}); err != nil {
		t.Fatal(err)
	}
	if op, _ := g.Operation(repo); op.Kind != OperationMerge {
		t.Fatalf("no-commit operation %+v", op)
	}
	if err := g.ContinueOperation(repo); err != nil {
		t.Fatal(err)
	}
	if count, _ := g.Run(repo, "rev-list", "--count", "HEAD"); count != "4" {
		t.Fatalf("no-commit commit %q", count)
	}
}
