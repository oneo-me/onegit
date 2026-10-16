package gitview

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHistoryReadsCommitsRefsAndCheckouts(t *testing.T) {
	t.Parallel()
	g := Git{}
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	if commits, err := g.Commits(repo, Refs{}); err != nil || len(commits) != 0 {
		t.Fatalf("repository without commits: %v %v", commits, err)
	}
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0644)
	os.MkdirAll(filepath.Join(repo, "dir"), 0755)
	os.WriteFile(filepath.Join(repo, "dir", "b.txt"), []byte("b\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	git(t, repo, "tag", "-a", "v1", "-m", "first")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second", "-m", "Body line.",
		"-m", "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nco-authored-by: Bot <NoReply@anthropic.com>\nCo-authored-by: Ann <ann@example.com>")
	wt := filepath.Join(t.TempDir(), "side")
	git(t, repo, "worktree", "add", "-q", "-b", "side", wt)
	git(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	commits, err := g.Commits(wt, Refs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].Subject != "second" || !strings.HasPrefix(commits[0].Body, "Body line.\n\nCo-Authored-By:") ||
		!slices.Equal(commits[0].CoAuthors, []Person{{"Claude Opus 5.5", "noreply@anthropic.com"}, {"Ann", "ann@example.com"}}) ||
		len(commits[1].CoAuthors) != 0 ||
		commits[0].Author != "t" || len(commits[0].Parents) != 1 || commits[0].Parents[0] != commits[1].Hash ||
		len(commits[1].Parents) != 0 || !slices.Contains(commits[1].Refs, "tag: v1") ||
		!slices.Contains(commits[0].Refs, "HEAD -> side") || !slices.Contains(commits[0].Refs, "main") || commits[0].AuthorDate.IsZero() {
		t.Fatalf("commits: %+v", commits)
	}

	refs, err := g.Refs(repo)
	if err != nil {
		t.Fatal(err)
	}
	names := func(list []Ref) string {
		var s []string
		for _, r := range list {
			s = append(s, r.Name)
		}
		return strings.Join(s, ",")
	}
	if names(refs.Branches) != "main,side" || names(refs.Remotes) != "origin/main" || names(refs.Tags) != "v1" ||
		refs.Tags[0].Hash != commits[1].Hash {
		t.Fatalf("refs: %+v", refs)
	}

	git(t, repo, "remote", "add", "origin", "git@github.com:o/r.git")
	git(t, repo, "remote", "add", "web", "https://user@GitHub.com/o/r")
	git(t, repo, "remote", "add", "other", "https://gitlab.com/o/github.com")
	git(t, repo, "remote", "add", "local", "/tmp/github.com")
	if remotes, err := g.GitHubRemotes(wt); err != nil || len(remotes) != 4 || !remotes["origin"] || !remotes["web"] || remotes["other"] || remotes["local"] {
		t.Fatalf("GitHub remotes: %v %v", remotes, err)
	}

	checkouts, err := g.Checkouts(wt)
	if err != nil {
		t.Fatal(err)
	}
	top, _ := filepath.EvalSymlinks(repo)
	side, _ := filepath.EvalSymlinks(wt)
	if len(checkouts) != 2 || checkouts[0].Branch != "main" || checkouts[1].Branch != "side" ||
		checkouts[0].Head != commits[0].Hash || canonical(checkouts[0].Path) != top || canonical(checkouts[1].Path) != side {
		t.Fatalf("checkouts: %+v", checkouts)
	}

	describe := func(changes Changes) string {
		var s []string
		for _, f := range changes.Files {
			before, after, err := g.FileVersions(changes, f)
			if err != nil {
				t.Fatal(err)
			}
			s = append(s, fmt.Sprintf("%c %s +%d -%d %q %q", f.Status, f.Path, f.Added, f.Deleted, before, after))
		}
		return strings.Join(s, "\n")
	}
	// The worktree's later edit stays out of the commits' files.
	os.WriteFile(filepath.Join(wt, "a.txt"), []byte("3\n"), 0644)
	changes, err := g.CommitChanges(wt, commits[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got := describe(changes); got != `M a.txt +1 -1 "1\n" "2\n"` {
		t.Fatalf("second commit:\n%s", got)
	}
	root, err := g.CommitChanges(wt, commits[1].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got := describe(root); got != "A a.txt +1 -0 \"\" \"1\\n\"\nA dir/b.txt +1 -0 \"\" \"b\\n\"" {
		t.Fatalf("root commit:\n%s", got)
	}
	files, err := g.CommitFiles(wt, commits[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got := describe(files); got != "A a.txt +0 -0 \"\" \"2\\n\"\nA dir/b.txt +0 -0 \"\" \"b\\n\"" {
		t.Fatalf("files:\n%s", got)
	}
	// The worktree's files are read from disk, untracked ones included and
	// deleted ones left out.
	os.WriteFile(filepath.Join(wt, "new.txt"), []byte("n\n"), 0644)
	os.Remove(filepath.Join(wt, "dir", "b.txt"))
	files, err = g.WorktreeFiles(wt)
	if err != nil {
		t.Fatal(err)
	}
	if got := describe(files); got != "A a.txt +0 -0 \"\" \"3\\n\"\nA new.txt +0 -0 \"\" \"n\\n\"" {
		t.Fatalf("worktree files:\n%s", got)
	}
}

func canonical(path string) string {
	resolved, _ := filepath.EvalSymlinks(path)
	return resolved
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}
