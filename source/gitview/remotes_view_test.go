package gitview

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
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

	v, pump := testView(t, Options{})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.loaded }
	pump(func() bool { return ready() && v.tracking.Branch == "main" })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// The Branches heading creates a branch at the checked out commit and
	// offers the stale branches.
	if err := view.RightClick("Branches"); err != nil {
		t.Fatal(err)
	}
	if got, want := view.Menu(), []string{"New Branch…", "Select Stale Local Branches"}; !slices.Equal(got, want) {
		t.Fatalf("branches menu %q", got)
	}
	if err := view.ChooseMenuItem("New Branch…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.action.open || !view.HasText("New branch at main") {
		t.Fatalf("new branch dialog: %v", view.Texts())
	}
	view.Type("topic")
	view.Frame()
	if err := view.Click("Create Branch"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.tracking.Branch == "topic" })

	// The Tags heading tags the checked out commit.
	view.Advance(time.Second)
	if err := view.RightClick("Tags"); err != nil {
		t.Fatal(err)
	}
	if got, want := view.Menu(), []string{"New Tag…"}; !slices.Equal(got, want) {
		t.Fatalf("tags menu %q", got)
	}
	if err := view.ChooseMenuItem("New Tag…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.action.open || !view.HasText("New tag at topic") {
		t.Fatalf("new tag dialog: %v", view.Texts())
	}
	view.Type("v2")
	view.Frame()
	if err := view.Click("Create Tag"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
		return ready() && slices.ContainsFunc(v.repo.refTrees.tags, func(n *refNode) bool { return n.path == "v2" })
	})

	// Selecting the stale branches offers the old branch for deletion.
	view.Advance(time.Second)
	if err := view.RightClick("Branches"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Select Stale Local Branches"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.stale.open && !v.stale.loading })
	view.Frame()
	if len(v.stale.branches) != 1 || v.stale.branches[0].name != "stale" || !view.HasText("Delete 1 Branch") {
		t.Fatalf("stale dialog %+v: %v", v.stale, view.Texts())
	}
	if err := view.Click("Delete 1 Branch"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
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
	v, pump := testView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(mine)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.loaded }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// The heading adds a remote.
	if err := view.RightClick("Remote branches"); err != nil {
		t.Fatal(err)
	}
	if got, want := view.Menu(), []string{"Add New Remote…"}; !slices.Equal(got, want) {
		t.Fatalf("remotes menu %q", got)
	}
	view.CloseMenu()
	view.Advance(time.Second)
	if err := view.Click("Remote branches"); err != nil {
		t.Fatal(err)
	}
	view.Frame()

	// The remote's menu follows Fork's order, without GitHub for a local
	// remote.
	if err := view.RightClick("origin"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	for _, item := range []string{"Fetch 'origin'…", "Edit 'origin'…", "Delete 'origin'…", "Fetch 'origin' Automatically", "Add New Remote…", "Copy Remote Address"} {
		if !slices.Contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	if slices.Contains(menu, "View on GitHub") {
		t.Fatalf("local remote on GitHub: %v", menu)
	}
	if err := view.ChooseMenuItem("Copy Remote Address"); err != nil {
		t.Fatal(err)
	}
	if view.Clipboard() != origin {
		t.Fatalf("copied %q, want %q", view.Clipboard(), origin)
	}

	// A new remote is added from the dialog and listed by Git.
	view.Advance(time.Second)
	if err := view.RightClick("origin"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Add New Remote…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.remoteDialog.open || !view.HasText("Add New Remote") {
		t.Fatalf("add dialog: %v", view.Texts())
	}
	view.Type("mirror")
	view.Key(0, ui.KeyTab)
	view.Type("https://example.com/mirror.git")
	view.Frame()
	if err := view.Click("Add Remote"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool {
		return ready() && v.repo.remoteURLs["mirror"] == "https://example.com/mirror.git" &&
			slices.ContainsFunc(v.repo.refTrees.remotes, func(n *refNode) bool { return n.path == "mirror" && n.children != nil })
	})
	view.Frame()
	if !view.HasText("mirror") {
		t.Fatalf("remote without branches missing: %v", view.Texts())
	}

	// Fetching one remote from its menu updates its remote branches.
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "fetched")
	git(t, seed, "push", "-q", "origin", "main")
	view.Advance(time.Second)
	if err := view.RightClick("origin"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Fetch 'origin'…"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.Syncing() == "" && v.tracking.Behind == 1 })

	// Fetching automatically is remembered and fetches the remote now.
	view.Advance(time.Second)
	if err := view.RightClick("origin"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Fetch 'origin' Automatically"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && v.autoFetch("origin") })
	if !slices.Contains(saved.AutoFetch[v.repo.root], "origin") {
		t.Fatalf("saved auto-fetch %+v", saved.AutoFetch)
	}

	// Opening a worktree again fetches the automatic remote.
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "later")
	git(t, seed, "push", "-q", "origin", "main")
	v.SetPath("")
	v.SetPath(mine)
	pump(func() bool { return ready() && v.Syncing() == "" && v.tracking.Behind == 2 })

	// Editing the address changes it, and offers the GitHub page.
	view.Advance(time.Second)
	if err := view.RightClick("origin"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Edit 'origin'…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.remoteDialog.open || !view.HasText("Edit Remote 'origin'") {
		t.Fatalf("edit dialog: %v", view.Texts())
	}
	view.Key(ui.Cmd, ui.KeyA)
	view.Type("git@github.com:o/r.git")
	view.Frame()
	if err := view.Click("Save"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.repo.remoteURLs["origin"] == "git@github.com:o/r.git" })
	view.Advance(time.Second)
	if err := view.RightClick("origin"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("View on GitHub"); err != nil {
		t.Fatal(err)
	}
	if got := view.OpenedURLs(); !slices.Contains(got, "https://github.com/o/r") {
		t.Fatalf("opened %q", got)
	}

	// Deleting asks first and forgets the automatic fetch.
	view.Advance(time.Second)
	if err := view.RightClick("origin"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Delete 'origin'…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.remoteDeletion.open || !view.HasText("Delete remote 'origin'?") {
		t.Fatalf("delete dialog: %v", view.Texts())
	}
	if err := view.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.repo.remoteURLs["origin"] == "" })
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

	v, pump := testView(t, Options{Git: Git{Env: env}})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.loaded }
	pump(func() bool { return ready() && v.tracking.Branch == "main" })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// Without remotes, the push dialog offers a private GitHub repository
	// named after the folder.
	v.OpenPush()
	pump(func() bool { return v.dialog.remotes != nil })
	view.Frame()
	if !v.dialog.gh || v.dialog.public || v.dialog.name != filepath.Base(repo) || !view.HasText("Create and Push") {
		t.Fatalf("push dialog %+v: %v", v.dialog, view.Texts())
	}
	v.dialog.name = "owner/project"
	view.Frame()
	if err := view.Click("Create and Push"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.tracking.Upstream == "origin/main" })
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
	pump(func() bool { return v.dialog.remotes != nil })
	view.Frame()
	if v.dialog.gh || !view.HasText(v.text("github_cli_missing")) {
		t.Fatalf("missing gh %+v: %v", v.dialog, view.Texts())
	}

	// A host that runs gh creates the repository there, as this computer
	// does; one that runs only git keeps the plain notice.
	host := &githubHost{env: env}
	v.git = Git{Host: host}
	v.OpenPush()
	pump(func() bool { return v.dialog.remotes != nil })
	view.Frame()
	if !v.dialog.gh || !view.HasText("Create and Push") {
		t.Fatalf("host push dialog %+v: %v", v.dialog, view.Texts())
	}
	if err := view.Click("Create and Push"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.tracking.Upstream == "origin/main" })
	host.mu.Lock()
	runs := host.ghRuns
	host.mu.Unlock()
	if got, _ := os.ReadFile(log); runs != 1 || strings.TrimSpace(string(got)) != "repo create "+filepath.Base(repo)+" --private --source "+repo+" --remote origin" {
		t.Fatalf("host ran gh %d times: %q", runs, got)
	}
	v.git = Git{Host: &countingHost{}}
	git(t, repo, "remote", "remove", "origin")
	v.OpenPush()
	pump(func() bool { return v.dialog.remotes != nil })
	view.Frame()
	if v.dialog.gh || !view.HasText(v.text("no_remotes")) {
		t.Fatalf("git-only host %+v: %v", v.dialog, view.Texts())
	}
}
