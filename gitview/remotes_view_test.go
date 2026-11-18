package gitview

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestViewGroupMenus(t *testing.T) {
	repo := testRepo(t)
	commitAt := func(date, message string) {
		t.Helper()
		cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", message)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("commit at %s: %v %s", date, err, out)
		}
	}
	commitAt("2000-01-01T00:00:00Z", "old work")
	git(t, repo, "branch", "stale")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "recent work")

	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.loaded }
	wait(func() bool { return ready() && v.tracking.Branch == "main" })

	// The Branches heading creates a branch at the checked out commit and
	// offers the stale branches.
	app.RightClick("id:group-branches")
	if got, want := menuLabels(app), []string{"New Branch…", "Select Stale Local Branches", "Push Multiple Branches…", "Delete Multiple Branches…"}; !slices.Equal(got, want) {
		t.Fatalf("branches menu %q", got)
	}
	app.ChoosePopup("New Branch…")
	app.WaitFor("text:New branch at main", time.Second)
	app.Type("id:ref-action-dialog >> role:textbox", "topic")
	app.Press("id:ref-action-dialog >> id:confirm")
	wait(func() bool { return ready() && v.tracking.Branch == "topic" })

	// The Tags heading tags the checked out commit.
	app.RightClick("id:group-tags")
	if got, want := menuLabels(app), []string{"New Tag…", "Push Multiple Tags…", "Delete Multiple Tags…"}; !slices.Equal(got, want) {
		t.Fatalf("tags menu %q", got)
	}
	app.ChoosePopup("New Tag…")
	app.WaitFor("text:New tag at topic", time.Second)
	app.Type("id:ref-action-dialog >> role:textbox >> nth=0", "v2")
	app.Press("id:ref-action-dialog >> id:confirm")
	wait(func() bool {
		return ready() && slices.ContainsFunc(v.repo.refTrees.tags, func(n *refNode) bool { return n.path == "v2" })
	})

	// Selecting the stale branches offers the old branch for deletion.
	app.RightClick("id:group-branches")
	app.ChoosePopup("Select Stale Local Branches")
	wait(func() bool { return v.stale.open && !v.stale.loading })
	if len(v.stale.branches) != 1 || v.stale.branches[0].name != "stale" {
		t.Fatalf("stale dialog %+v", v.stale)
	}
	app.WaitFor("id:stale-dialog >> text:Delete 1 Branch", time.Second)
	app.Press("id:stale-dialog >> id:confirm")
	wait(func() bool {
		return ready() && !slices.ContainsFunc(v.repo.refTrees.branches, func(n *refNode) bool { return n.path == "stale" })
	})
	if out, _ := v.git.Run(repo, "branch", "--list", "stale"); out != "" {
		t.Fatalf("kept stale: %q", out)
	}
}

func TestViewRemoteMenu(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	seed := filepath.Join(dir, "seed")
	git(t, dir, "clone", "-q", origin, seed)
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, seed, "push", "-q", "origin", "main")
	mine := filepath.Join(dir, "mine")
	git(t, dir, "clone", "-q", origin, mine)

	var saved State
	v, app, wait := startView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(mine)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.loaded }
	wait(ready)
	remote := "id:folder-remotes/origin"

	// The heading adds a remote.
	app.RightClick("id:group-remotes")
	if got, want := menuLabels(app), []string{"Add New Remote…"}; !slices.Equal(got, want) {
		t.Fatalf("remotes menu %q", got)
	}
	app.ClosePopup()
	app.Press("id:group-remotes")

	// The remote's menu follows Fork's order, without GitHub for a local
	// remote.
	app.RightClick(remote)
	menu := menuLabels(app)
	for _, item := range []string{"Fetch 'origin'…", "Edit 'origin'…", "Delete 'origin'…", "Fetch 'origin' Automatically", "Add New Remote…", "Copy Remote Address"} {
		if !slices.Contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	if slices.Contains(menu, "View on GitHub") {
		t.Fatalf("local remote on GitHub: %v", menu)
	}
	app.ChoosePopup("Copy Remote Address")
	if app.Clipboard != origin {
		t.Fatalf("copied %q, want %q", app.Clipboard, origin)
	}

	// A new remote is added from the dialog and listed by Git.
	app.RightClick(remote)
	app.ChoosePopup("Add New Remote…")
	app.WaitFor("text:Add New Remote", time.Second)
	app.Type("id:remote-name", "mirror")
	app.Type("id:remote-url", "https://example.com/mirror.git")
	app.Press("id:remote-dialog >> id:confirm")
	wait(func() bool {
		return ready() && v.repo.remoteURLs["mirror"] == "https://example.com/mirror.git" &&
			slices.ContainsFunc(v.repo.refTrees.remotes, func(n *refNode) bool { return n.path == "mirror" && n.children != nil })
	})
	app.WaitFor("id:folder-remotes/mirror", time.Second)

	// Fetching one remote from its menu updates its remote branches.
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "fetched")
	git(t, seed, "push", "-q", "origin", "main")
	app.RightClick(remote)
	app.ChoosePopup("Fetch 'origin'…")
	wait(func() bool { return ready() && v.Syncing() == "" && v.tracking.Behind == 1 })

	// Fetching automatically is remembered and fetches the remote now.
	app.RightClick(remote)
	app.ChoosePopup("Fetch 'origin' Automatically")
	wait(func() bool { return v.Syncing() == "" && v.autoFetch("origin") })
	if !slices.Contains(saved.AutoFetch[v.repo.root], "origin") {
		t.Fatalf("saved auto-fetch %+v", saved.AutoFetch)
	}

	// Opening a worktree again fetches the automatic remote.
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "later")
	git(t, seed, "push", "-q", "origin", "main")
	v.SetPath("")
	v.SetPath(mine)
	wait(func() bool { return ready() && v.Syncing() == "" && v.tracking.Behind == 2 })

	// Editing the address changes it, and offers the GitHub page.
	app.RightClick(remote)
	app.ChoosePopup("Edit 'origin'…")
	app.WaitFor("text:Edit Remote 'origin'", time.Second)
	app.Clear("id:remote-url")
	app.Type("id:remote-url", "git@github.com:o/r.git")
	app.Press("id:remote-dialog >> id:confirm")
	wait(func() bool { return ready() && v.repo.remoteURLs["origin"] == "git@github.com:o/r.git" })
	app.RightClick(remote)
	app.ChoosePopup("View on GitHub")
	if got := app.OpenedURLs; !slices.Contains(got, "https://github.com/o/r") {
		t.Fatalf("opened %q", got)
	}

	// Deleting asks first and forgets the automatic fetch.
	app.RightClick(remote)
	app.ChoosePopup("Delete 'origin'…")
	app.WaitFor("text:Delete remote 'origin'?", time.Second)
	app.Press("id:remote-delete-dialog >> id:confirm")
	wait(func() bool { return ready() && v.repo.remoteURLs["origin"] == "" })
	if slices.Contains(saved.AutoFetch[v.repo.root], "origin") {
		t.Fatalf("kept auto-fetch %+v", saved.AutoFetch)
	}
	if out, _ := v.git.Run(mine, "remote"); strings.Contains(out, "origin") {
		t.Fatalf("kept remote %q", out)
	}
}

func TestGitHubWebURL(t *testing.T) {
	t.Parallel()
	for remote, want := range map[string]string{
		"https://github.com/o/r.git":      "https://github.com/o/r",
		"https://user@GitHub.com/o/r/":    "https://github.com/o/r",
		"git@github.com:o/r.git":          "https://github.com/o/r",
		"ssh://git@github.com/o/r.git":    "https://github.com/o/r",
		"git://github.com/o/r":            "https://github.com/o/r",
		"https://gitlab.com/o/github.com": "",
		"/tmp/github.com":                 "",
		"https://github.com":              "",
		"ssh://git@www.github.com/o/r":    "https://github.com/o/r",
	} {
		if got := githubWebURL(remote); got != want {
			t.Errorf("githubWebURL(%q) = %q, want %q", remote, got, want)
		}
	}
}

// The GitHub page follows the branch's upstream, else the default remote.
func TestViewGitHubPage(t *testing.T) {
	t.Parallel()
	v := &View{}
	v.repo.remoteURLs = map[string]string{"origin": "git@github.com:o/r.git", "fork": "https://github.com/me/r", "lab": "https://gitlab.com/o/r"}
	v.repo.github = map[string]bool{"origin": true, "fork": true, "lab": false}
	for upstream, want := range map[string]string{
		"fork/main": "https://github.com/me/r", "": "https://github.com/o/r", "lab/main": "",
	} {
		v.tracking.Upstream = upstream
		if got := v.GitHubPage(); got != want {
			t.Errorf("upstream %q: %q, want %q", upstream, got, want)
		}
	}
}

func TestAddRefFolder(t *testing.T) {
	t.Parallel()
	nodes := addRefFolder(nil, "team/origin")
	nodes = addRefFolder(nodes, "origin")
	nodes = addRefFolder(nodes, "team/origin")
	if len(nodes) != 2 || nodes[0].path != "origin" || nodes[1].path != "team" {
		t.Fatalf("folders %+v", nodes)
	}
	children := nodes[1].children
	if len(children) != 1 || children[0].path != "team/origin" || children[0].children == nil {
		t.Fatalf("nested remote %+v", children)
	}
}

func TestViewPushCreatesGitHubRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the gh stand-in is a POSIX script")
	}
	repo := testRepo(t)
	dir := t.TempDir()
	bare := filepath.Join(dir, "github.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", bare)
	// A stand-in for the GitHub CLI records its arguments and adds the
	// remote as gh repo create --source does.
	bin := filepath.Join(dir, "bin")
	os.Mkdir(bin, 0755)
	script := "#!/bin/sh\necho \"$@\" > \"$GH_LOG\"\ngit -C \"$6\" remote add origin \"$GH_REMOTE\"\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "gh.log")
	env := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GH_LOG="+log, "GH_REMOTE="+bare)

	v, app, wait := startView(t, Options{Git: Git{Env: env}})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.loaded }
	wait(func() bool { return ready() && v.tracking.Branch == "main" })

	// Without remotes, the push dialog offers a private GitHub repository
	// named after the folder.
	v.OpenPush()
	wait(func() bool { return v.dialog.remotes != nil })
	app.WaitFor("id:sync-dialog >> text:Create and Push", time.Second)
	if !v.dialog.gh || v.dialog.public || v.dialog.name != filepath.Base(repo) {
		t.Fatalf("push dialog %+v", v.dialog)
	}
	app.Clear("id:github-name")
	app.Type("id:github-name", "owner/project")
	app.Press("id:sync-dialog >> id:confirm")
	wait(func() bool { return ready() && v.tracking.Upstream == "origin/main" })
	if got, _ := os.ReadFile(log); strings.TrimSpace(string(got)) != "repo create owner/project --private --source "+repo+" --remote origin" {
		t.Fatalf("gh %q", got)
	}
	if out, _ := v.git.Run(bare, "rev-parse", "main"); out != v.repo.head {
		t.Fatalf("pushed %q, want %q", out, v.repo.head)
	}

	// Without the GitHub CLI, the dialog says how to get it.
	v.git = Git{Env: append(os.Environ(), "PATH=")}
	dirs := ghDirs
	ghDirs = nil
	t.Cleanup(func() { ghDirs = dirs })
	git(t, repo, "remote", "remove", "origin")
	v.OpenPush()
	wait(func() bool { return v.dialog.remotes != nil })
	app.WaitFor("text:"+v.text("github_cli_missing"), time.Second)
	if v.dialog.gh {
		t.Fatalf("missing gh %+v", v.dialog)
	}
	app.Press("id:sync-dialog >> id:cancel")

	// A host that runs gh creates the repository there, as this computer
	// does; one that runs only git keeps the plain notice.
	host := &githubHost{env: env}
	v.git = Git{Host: host}
	v.OpenPush()
	wait(func() bool { return v.dialog.remotes != nil })
	app.WaitFor("id:sync-dialog >> text:Create and Push", time.Second)
	if !v.dialog.gh {
		t.Fatalf("host push dialog %+v", v.dialog)
	}
	app.Press("id:sync-dialog >> id:confirm")
	wait(func() bool { return ready() && v.tracking.Upstream == "origin/main" })
	host.mu.Lock()
	runs := host.ghRuns
	host.mu.Unlock()
	if got, _ := os.ReadFile(log); runs != 1 || strings.TrimSpace(string(got)) != "repo create "+filepath.Base(repo)+" --private --source "+repo+" --remote origin" {
		t.Fatalf("host ran gh %d times: %q", runs, got)
	}
	v.git = Git{Host: &countingHost{}}
	git(t, repo, "remote", "remove", "origin")
	v.OpenPush()
	wait(func() bool { return v.dialog.remotes != nil })
	app.WaitFor("text:"+v.text("no_remotes"), time.Second)
	if v.dialog.gh {
		t.Fatalf("git-only host %+v", v.dialog)
	}
}
