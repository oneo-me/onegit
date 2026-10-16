package gitview

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTrackingFetchPullAndPush(t *testing.T) {
	t.Parallel()
	// A rebase commits, so it needs an identity.
	g := Git{Env: append(os.Environ(), "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")}
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	seed := filepath.Join(dir, "seed")
	git(t, dir, "clone", "-q", origin, seed)
	os.WriteFile(filepath.Join(seed, "a.txt"), []byte("1\n"), 0644)
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-q", "-m", "init")
	git(t, seed, "push", "-q", "origin", "main")
	mine := filepath.Join(dir, "mine")
	git(t, dir, "clone", "-q", origin, mine)

	check := func(path string, ahead, behind int, upstream string) {
		t.Helper()
		got, err := g.Tracking(path)
		if err != nil || got.Ahead != ahead || got.Behind != behind || got.Upstream != upstream {
			t.Fatalf("tracking: %+v %v", got, err)
		}
	}
	check(mine, 0, 0, "origin/main")
	// Another clone's commit shows once fetched, and pulls by fast-forward.
	os.WriteFile(filepath.Join(seed, "a.txt"), []byte("2\n"), 0644)
	git(t, seed, "commit", "-q", "-am", "theirs")
	git(t, seed, "push", "-q", "origin", "main")
	check(mine, 0, 0, "origin/main")
	if err := g.Fetch(mine); err != nil {
		t.Fatal(err)
	}
	check(mine, 0, 1, "origin/main")
	pull := PullOptions{Remote: "origin", Branch: "main"}
	if err := g.Pull(mine, pull); err != nil {
		t.Fatal(err)
	}
	check(mine, 0, 0, "origin/main")
	// Diverged work rebases onto the pulled commits, keeping uncommitted
	// changes aside meanwhile.
	os.WriteFile(filepath.Join(seed, "c.txt"), []byte("c\n"), 0644)
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-q", "-m", "theirs again")
	git(t, seed, "push", "-q", "origin", "main")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "local")
	os.WriteFile(filepath.Join(mine, "a.txt"), []byte("dirty\n"), 0644)
	pull.Rebase, pull.Stash = true, true
	if err := g.Pull(mine, pull); err != nil {
		t.Fatal(err)
	}
	check(mine, 1, 0, "origin/main")
	if data, _ := os.ReadFile(filepath.Join(mine, "a.txt")); string(data) != "dirty\n" {
		t.Fatalf("stash lost: %q", data)
	}
	git(t, mine, "checkout", "-q", "--", "a.txt")
	if merges, _ := g.Run(mine, "rev-list", "--count", "--merges", "HEAD"); merges != "0" {
		t.Fatalf("rebase merged: %s", merges)
	}
	// A local commit is pushed to the upstream.
	os.WriteFile(filepath.Join(mine, "b.txt"), []byte("b\n"), 0644)
	git(t, mine, "add", ".")
	git(t, mine, "commit", "-q", "-m", "mine")
	check(mine, 2, 0, "origin/main")
	git(t, mine, "tag", "v1")
	if err := g.PushBranch(mine, PushOptions{Branch: "main", Remote: "origin", To: "main", Tags: true}); err != nil {
		t.Fatal(err)
	}
	check(mine, 0, 0, "origin/main")
	if tags, _ := g.Run(origin, "tag"); tags != "v1" {
		t.Fatalf("tags %q", tags)
	}
	// A rewritten branch is refused unless forced.
	git(t, mine, "commit", "-q", "--amend", "--allow-empty", "-m", "amended")
	push := PushOptions{Branch: "main", Remote: "origin", To: "main"}
	if err := g.PushBranch(mine, push); err == nil {
		t.Fatal("pushed a rewritten branch without force")
	}
	push.Force = true
	if err := g.PushBranch(mine, push); err != nil {
		t.Fatal(err)
	}
	check(mine, 0, 0, "origin/main")
	// A new branch counts the commits no remote has, and is published.
	git(t, mine, "switch", "-q", "-c", "topic")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "topic")
	check(mine, 1, 0, "")
	remotes, err := g.Remotes(mine)
	if err != nil || remotes.Current != "topic" || !slices.Equal(remotes.Names, []string{"origin"}) ||
		!slices.Equal(remotes.Local, []string{"main", "topic"}) || !slices.Equal(remotes.Branches["origin"], []string{"main"}) ||
		remotes.Upstreams["main"] != (RemoteBranch{"origin", "main"}) || remotes.DefaultRemote() != "origin" {
		t.Fatalf("remotes %+v %v", remotes, err)
	}
	if _, ok := remotes.Upstreams["topic"]; ok {
		t.Fatalf("topic tracks %+v", remotes.Upstreams)
	}
	// A branch pushed under another name then tracks it.
	if err := g.PushBranch(mine, PushOptions{Branch: "topic", Remote: "origin", To: "feature"}); err != nil {
		t.Fatal(err)
	}
	check(mine, 0, 0, "origin/feature")
	if err := g.PushBranch(mine, PushOptions{Remote: "origin", To: "main"}); err != ErrDetached {
		t.Fatalf("no branch: %v", err)
	}
	if gitDir, err := g.GitDir(filepath.Join(mine)); err != nil || canonical(gitDir) != canonical(filepath.Join(mine, ".git")) {
		t.Fatalf("git dir %q %v", gitDir, err)
	}
}

func TestDeleteRefs(t *testing.T) {
	t.Parallel()
	g := Git{}
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	mine := filepath.Join(dir, "mine")
	git(t, dir, "clone", "-q", origin, mine)
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, mine, "push", "-q", "origin", "main")
	git(t, mine, "switch", "-q", "-c", "topic")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "topic")
	git(t, mine, "push", "-q", "-u", "origin", "topic")
	git(t, mine, "branch", "-q", "--track", "follower", "origin/topic")
	git(t, mine, "branch", "-q", "--track", "local", "main")
	git(t, mine, "switch", "-q", "-c", "wip", "main")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "wip")
	git(t, mine, "switch", "-q", "main")
	git(t, mine, "tag", "v1")
	git(t, mine, "push", "-q", "origin", "v1")

	upstreams := func() map[string]RemoteBranch {
		t.Helper()
		refs, err := g.Refs(mine)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]RemoteBranch{}
		for _, r := range refs.Branches {
			m[r.Name] = r.Upstream
		}
		return m
	}
	if u := upstreams(); u["topic"] != (RemoteBranch{"origin", "topic"}) || u["follower"] != u["topic"] ||
		u["wip"] != (RemoteBranch{}) || u["local"] != (RemoteBranch{}) {
		t.Fatalf("upstreams %+v", u)
	}
	// A branch whose commits only it has, or that is checked out, stays.
	if err := g.DeleteBranch(mine, "wip"); err == nil {
		t.Fatal("deleted an unmerged branch")
	}
	if err := g.DeleteBranch(mine, "main"); err == nil {
		t.Fatal("deleted the checked out branch")
	}
	// A branch its upstream contains is deleted, then the remote's copy,
	// which leaves the branches tracking it without an upstream.
	if err := g.DeleteBranch(mine, "topic"); err != nil {
		t.Fatal(err)
	}
	if err := g.DeleteRemoteBranch(mine, "origin", "topic"); err != nil {
		t.Fatal(err)
	}
	if out, _ := g.Run(origin, "branch", "--list", "topic"); out != "" {
		t.Fatalf("remote kept topic: %q", out)
	}
	if u := upstreams(); u["follower"] != (RemoteBranch{}) {
		t.Fatalf("gone upstream kept: %+v", u)
	}
	// A remote branch the remote no longer has is dropped.
	git(t, mine, "update-ref", "refs/remotes/origin/stale", "HEAD")
	if err := g.DeleteRemoteBranch(mine, "origin", "stale"); err != nil {
		t.Fatal(err)
	}
	if out, _ := g.Run(mine, "branch", "-r", "--list", "origin/stale"); out != "" {
		t.Fatalf("kept stale remote branch: %q", out)
	}
	// A tag is deleted from the remote, which may not have it, and locally.
	for range 2 {
		if err := g.DeleteRemoteTag(mine, "origin", "v1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.DeleteTag(mine, "v1"); err != nil {
		t.Fatal(err)
	}
	if out, _ := g.Run(origin, "tag"); out != "" {
		t.Fatalf("remote tags %q", out)
	}
	if out, _ := g.Run(mine, "tag"); out != "" {
		t.Fatalf("local tags %q", out)
	}
}
