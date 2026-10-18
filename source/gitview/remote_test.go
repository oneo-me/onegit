package gitview

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	// A branch checked out in another worktree goes with it, unless Git
	// would keep the branch or the worktree has changes.
	for _, b := range []string{"wip", "local"} {
		git(t, mine, "worktree", "add", "-q", filepath.Join(dir, b), b)
	}
	if err := g.DeleteWorktreeBranch(mine, filepath.Join(dir, "wip"), "wip"); err != ErrNotMerged {
		t.Fatalf("unmerged: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "local", "draft.txt"), []byte("draft\n"), 0644)
	if err := g.DeleteWorktreeBranch(mine, filepath.Join(dir, "local"), "local"); err == nil {
		t.Fatal("removed a worktree with changes")
	}
	if _, ok := upstreams()["local"]; !ok {
		t.Fatal("deleted local")
	}
	if checkouts, _ := g.Checkouts(mine); len(checkouts) != 3 {
		t.Fatalf("kept %+v", checkouts)
	}
	os.Remove(filepath.Join(dir, "local", "draft.txt"))
	if err := g.DeleteWorktreeBranch(mine, filepath.Join(dir, "local"), "local"); err != nil {
		t.Fatal(err)
	}
	if _, ok := upstreams()["local"]; ok {
		t.Fatal("kept local")
	}
	if _, err := os.Stat(filepath.Join(dir, "local")); !os.IsNotExist(err) {
		t.Fatalf("kept the worktree: %v", err)
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

func TestCheckoutWorktreesMergeRebaseAndCreate(t *testing.T) {
	t.Parallel()
	g := Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	mine := filepath.Join(dir, "mine")
	git(t, dir, "clone", "-q", origin, mine)
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, mine, "tag", "v1")
	git(t, mine, "switch", "-q", "-c", "topic")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "topic")
	git(t, mine, "push", "-q", "origin", "main", "topic")
	git(t, mine, "switch", "-q", "main")
	git(t, mine, "branch", "-q", "-D", "topic")
	head := func(path string) (branch, upstream string) {
		t.Helper()
		tr, err := g.Tracking(path)
		if err != nil {
			t.Fatal(err)
		}
		return tr.Branch, tr.Upstream
	}

	// A remote branch checks out as a new branch tracking it, a tag
	// detached, and a local branch by name.
	if err := g.Checkout(mine, CheckoutOptions{Branch: "topic", Start: "refs/remotes/origin/topic", Track: true}); err != nil {
		t.Fatal(err)
	}
	if b, up := head(mine); b != "topic" || up != "origin/topic" {
		t.Fatalf("checked out %q tracking %q", b, up)
	}
	if err := g.Checkout(mine, CheckoutOptions{Start: "refs/tags/v1"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := head(mine); b != "" {
		t.Fatalf("tag checked out on %q", b)
	}
	if err := g.Checkout(mine, CheckoutOptions{Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := head(mine); b != "main" {
		t.Fatalf("checked out %q", b)
	}

	// Worktrees check out a local branch, a new branch or a detached tag.
	for i, o := range []CheckoutOptions{{Branch: "topic"}, {Branch: "fix", Start: "refs/tags/v1"}, {Start: "refs/tags/v1"}} {
		wt := filepath.Join(dir, "wt"+strconv.Itoa(i))
		if err := g.AddWorktree(mine, wt, o); err != nil {
			t.Fatal(err)
		}
		if b, _ := head(wt); b != o.Branch {
			t.Fatalf("worktree %d on %q", i, b)
		}
	}
	if err := g.AddWorktree(mine, filepath.Join(dir, "again"), CheckoutOptions{Branch: "topic"}); err == nil {
		t.Fatal("checked out a branch twice")
	}

	// A merge fast-forwards unless told to record a merge commit.
	if err := g.Merge(mine, "refs/heads/topic", IntegrateOptions{}); err != nil {
		t.Fatal(err)
	}
	if merges, _ := g.Run(mine, "rev-list", "--count", "--merges", "HEAD"); merges != "0" {
		t.Fatalf("fast-forward merged: %s", merges)
	}
	git(t, mine, "reset", "-q", "--hard", "refs/tags/v1")
	if err := g.Merge(mine, "refs/heads/topic", IntegrateOptions{NoFastForward: true}); err != nil {
		t.Fatal(err)
	}
	if merges, _ := g.Run(mine, "rev-list", "--count", "--merges", "HEAD"); merges != "1" {
		t.Fatalf("merges: %s", merges)
	}

	// A rebase replays the branch's commits, setting changes aside.
	git(t, mine, "reset", "-q", "--hard", "refs/tags/v1")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "mine")
	os.WriteFile(filepath.Join(mine, "dirty.txt"), []byte("x\n"), 0644)
	git(t, mine, "add", "dirty.txt")
	if err := g.Rebase(mine, "refs/heads/topic", IntegrateOptions{}); err == nil {
		t.Fatal("rebased over staged changes")
	}
	if err := g.Rebase(mine, "refs/heads/topic", IntegrateOptions{Stash: true}); err != nil {
		t.Fatal(err)
	}
	if log, _ := g.Run(mine, "log", "--format=%s"); log != "mine\ntopic\ninit" {
		t.Fatalf("rebased: %q", log)
	}
	if _, err := os.Stat(filepath.Join(mine, "dirty.txt")); err != nil {
		t.Fatal(err)
	}

	// Branches start anywhere without moving the worktree, and tags are
	// lightweight unless they carry a message.
	if err := g.CreateBranch(mine, "later", "refs/remotes/origin/main"); err != nil {
		t.Fatal(err)
	}
	if b, _ := head(mine); b != "main" {
		t.Fatalf("created branch checked out: %q", b)
	}
	if err := g.CreateTag(mine, "light", "HEAD", ""); err != nil {
		t.Fatal(err)
	}
	if err := g.CreateTag(mine, "notes", "refs/heads/later", "Release\n\n# dropped\nnotes\n"); err != nil {
		t.Fatal(err)
	}
	if kinds, _ := g.Run(mine, "for-each-ref", "--format=%(objecttype) %(contents)", "refs/tags/light", "refs/tags/notes"); kinds != "commit mine\n\ntag Release\n\nnotes" {
		t.Fatalf("tags: %q", kinds)
	}

	// A checkout sets uncommitted changes aside and back, or drops them.
	os.WriteFile(filepath.Join(mine, "f.txt"), []byte("1\n"), 0644)
	git(t, mine, "add", "f.txt")
	git(t, mine, "commit", "-q", "-m", "f")
	git(t, mine, "branch", "side")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "main only")
	os.WriteFile(filepath.Join(mine, "f.txt"), []byte("2\n"), 0644)
	if err := g.Checkout(mine, CheckoutOptions{Branch: "side", Changes: StashChanges}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(mine, "f.txt")); string(data) != "2\n" {
		t.Fatalf("stashed change lost: %q", data)
	}
	if stash, _ := g.Run(mine, "stash", "list"); stash != "" {
		t.Fatalf("stash kept: %q", stash)
	}
	if err := g.Checkout(mine, CheckoutOptions{Branch: "main", Changes: DiscardChanges}); err != nil {
		t.Fatal(err)
	}
	if status, _ := g.Run(mine, "status", "--porcelain", "--untracked-files=no"); status != "" {
		t.Fatalf("changes kept: %q", status)
	}
}
