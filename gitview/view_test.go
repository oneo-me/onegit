package gitview

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

// testView builds a view whose results return through pump, which runs them
// until cond holds, for tests that need no window.
func testView(t *testing.T, opts Options) (*View, func(cond func() bool)) {
	t.Helper()
	inbox := make(chan func(), 64)
	opts.Dispatch = func(fn func()) { inbox <- fn }
	v := New(opts)
	t.Cleanup(v.Close)
	pump := func(cond func() bool) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for !cond() {
			select {
			case fn := <-inbox:
				fn()
			case <-deadline:
				t.Fatal("timed out")
			}
		}
	}
	return v, pump
}

// startView shows a view, with its toolbar above it, in a headless desktop
// window whose engine runs on the test's goroutine; wait settles it until
// cond holds.
func startView(t *testing.T, opts Options) (*View, *nsgotest.App, func(cond func() bool)) {
	t.Helper()
	var v *View
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		opts.Dispatch = cx.Post
		v = New(opts)
		v.Bind(cx)
		cx.Cleanup(v.Close)
		return core.View{Style: ui.FillRest, Children: []core.Node{v.Toolbar(32), v.Node()}}
	}, nsgotest.Size(1100, 760), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	wait := func(cond func() bool) {
		t.Helper()
		app.WaitUntil(10*time.Second, cond)
	}
	return v, app, wait
}

// menuLabels lists the labels of the context menu shown, its submenus' with
// their path.
func menuLabels(app *nsgotest.App) []string {
	var labels []string
	for _, item := range app.PopupMenu {
		if !item.Separator {
			labels = append(labels, item.Label)
		}
	}
	return labels
}

// shows reports whether a text containing text shows.
func shows(app *nsgotest.App, text string) bool { return len(app.Query("text:"+text)) > 0 }

func testRepo(t *testing.T) string {
	t.Helper()
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	git(t, repo, "tag", "v1")
	return repo
}

func TestViewHidesFilteredRefs(t *testing.T) {
	repo := testRepo(t)
	wt := filepath.Join(t.TempDir(), "task")
	git(t, repo, "worktree", "add", "-q", "-b", "host/task", wt)
	git(t, wt, "commit", "-q", "--allow-empty", "-m", "task work")
	git(t, repo, "branch", "host/old", "main")
	git(t, repo, "branch", "feature", "main")

	v, pump := testView(t, Options{
		RefFilter: func(kind RefKind, name string) bool { return kind != RefBranch || !strings.HasPrefix(name, "host/") },
	})
	v.SetPath(repo)
	pump(func() bool { return v.repo.loaded && !v.repo.loading && v.current.loaded })

	// The hidden branch's commits leave the history; the newest commit is
	// selected first.
	commits := v.repo.commits
	if len(commits) != 1 || commits[0].Subject != "init" || v.history.selected != commits[0].Hash {
		t.Fatalf("history: %+v, selected %q", commits, v.history.selected)
	}
	if b := v.repo.refTrees.branches; len(b) != 2 || b[0].path != "feature" || b[1].path != "main" {
		t.Fatalf("branches: %+v", b)
	}
}

func TestViewOffersChangesAndRunsToolbarActions(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "feature")
	git(t, repo, "branch", "host/task")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0644)
	var saved State
	var written, writtenPaths []string
	v, app, wait := startView(t, Options{
		Language: "zh-Hans",
		Git:      Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"),
		RefFilter: func(kind RefKind, name string) bool {
			return kind != RefBranch || !strings.HasPrefix(name, "host/")
		},
		OnState: func(s State) { saved = s },
		OnWrite: func(op, path string) {
			written, writtenPaths = append(written, op), append(writtenPaths, path)
		},
	})
	v.SetPath(repo)
	wait(func() bool { return v.repo.loaded && !v.repo.loading && v.current.loaded && !v.current.loading })

	// Changes found after opening replace the history.
	if v.section != sectionChanges || !v.hasCurrentChanges() {
		t.Fatalf("section %v", v.section)
	}
	app.WaitFor("id:current-changes", time.Second)
	app.WaitFor("id:all-commits", time.Second)

	// The toolbar offers every action. Its stash dialog saves a stash with
	// untracked files, one write at a time, and rereads the view; the
	// emptied changes stay shown.
	for _, op := range []string{OpFetch, OpPull, OpPush, OpStash} {
		app.Find("id:toolbar-" + op)
	}
	app.Press("id:toolbar-" + OpStash)
	app.WaitFor("text:包含未跟踪的文件", time.Second)
	if !v.stashDialog.open {
		t.Fatal("the stash dialog did not open")
	}
	app.Type("id:stash-message", "my work")
	app.Submit("id:stash-message")
	wait(func() bool { return v.Syncing() == "" && len(written) == 1 })
	if written[0] != OpStash || writtenPaths[0] != repo {
		t.Fatalf("written: %v in %v", written, writtenPaths)
	}
	wait(func() bool { return v.current.loaded && !v.current.loading && !v.Loading() && len(v.repo.stashes) == 1 })
	if v.section != sectionChanges || v.hasCurrentChanges() || v.CanStash() {
		t.Fatalf("after stash: section %v, files %v", v.section, v.current.changes.Files)
	}

	// The refs pane lists the stash, which shows the files it saved.
	app.Press("text:On main: my work")
	wait(func() bool { return v.stashFiles.loaded && !v.stashFiles.loading && !v.stashFiles.fileLoading })
	if v.section != sectionStash {
		t.Fatalf("stash: section %v", v.section)
	}
	app.WaitFor("text:new.txt", time.Second)
	for _, f := range v.stashFiles.changes.Files {
		before, after, err := v.git.FileVersions(v.stashFiles.changes, f)
		if err != nil || f.Path == "a.txt" && (string(before) != "1\n" || string(after) != "2\n") ||
			f.Path == "new.txt" && (before != nil || string(after) != "new\n") {
			t.Fatalf("%s: %q, %q, %v", f.Path, before, after, err)
		}
	}

	// Double-clicking the stash asks to apply it, deleting it by default;
	// once gone, it leaves for the history.
	app.PressAtWith("text:On main: my work", core.Point{X: 4, Y: 4}, core.Modifiers{})
	v.openApplyStash(v.repo.stashes[0])
	app.WaitFor("text:应用后删除贮藏", time.Second)
	app.WaitFor("text:stash@{0}", time.Second)
	app.Press("id:apply-stash-dialog >> id:confirm")
	wait(func() bool { return v.Syncing() == "" && !v.Loading() && len(v.repo.stashes) == 0 })
	if v.section != sectionCommits {
		t.Fatalf("applied: section %v", v.section)
	}
	if data, _ := os.ReadFile(filepath.Join(repo, "new.txt")); string(data) != "new\n" {
		t.Fatalf("applied: %q", data)
	}
	git(t, repo, "stash", "push", "-q", "--include-untracked")

	// A clean worktree opens on the history and still offers its changes.
	v.SetPath(t.TempDir())
	v.SetPath(repo)
	wait(func() bool { return v.current.loaded && !v.current.loading })
	if v.section != sectionCommits {
		t.Fatalf("clean: section %v", v.section)
	}
	app.Press("id:current-changes")
	wait(func() bool { return v.current.loaded && !v.current.loading })
	if v.section != sectionChanges {
		t.Fatalf("chosen: section %v", v.section)
	}

	// Collapsing a ref group is remembered.
	app.Press("id:group-tags")
	if open, ok := saved.Groups["tags"]; !ok || !open {
		t.Fatalf("groups: %v", saved.Groups)
	}

	// A write running in one repository leaves another's writes free.
	release := make(chan struct{})
	v.Run("host", "failed", func(Git, string) error { <-release; return nil })
	other := testRepo(t)
	v.SetPath(other)
	wait(func() bool { return v.repo.loaded && !v.repo.loading })
	if v.Syncing() != "" || !v.CanSync() {
		t.Fatalf("other repository: syncing %q", v.Syncing())
	}
	v.SetPath(repo)
	if v.Syncing() != "host" {
		t.Fatalf("back: syncing %q", v.Syncing())
	}
	close(release)
	wait(func() bool { return v.Syncing() == "" })
	if n := len(written); written[n-1] != "host" || writtenPaths[n-1] != repo {
		t.Fatalf("written: %v in %v", written, writtenPaths)
	}
}

func TestViewRemembersRefFolders(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "feature/login", "main")

	var saved State
	v, app, wait := startView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(repo)
	wait(func() bool { return v.repo.loaded && !v.repo.loading })
	app.WaitFor("id:ref-branches/feature/login", time.Second)

	// Collapsing a folder saves it; reopening drops it again.
	app.Press("id:folder-branches/feature")
	app.WaitForGone("id:ref-branches/feature/login", time.Second)
	if open, ok := saved.Groups["branches/feature/"]; !ok || open {
		t.Fatalf("collapsed: %v", saved.Groups)
	}

	// Another view restores it collapsed.
	other, restored, wait := startView(t, Options{State: saved})
	other.SetPath(repo)
	wait(func() bool { return other.repo.loaded && !other.repo.loading })
	restored.WaitFor("id:folder-branches/feature", time.Second)
	if restored.Exists("id:ref-branches/feature/login") {
		t.Fatal("restored the folder open")
	}

	app.Press("id:folder-branches/feature")
	app.WaitFor("id:ref-branches/feature/login", time.Second)
	if _, ok := saved.Groups["branches/feature/"]; ok {
		t.Fatalf("reopened: %v", saved.Groups)
	}
}

func TestViewShowsErrorOutsideRepository(t *testing.T) {
	v, app, wait := startView(t, Options{})
	v.SetPath(t.TempDir())
	wait(func() bool { return v.repo.loaded && !v.repo.loading })
	app.WaitFor("text:This project has no Git history to compare", time.Second)
	if v.CanSync() {
		t.Fatal("a write may start outside a repository")
	}
}

func TestViewFetchDialog(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	seed := filepath.Join(dir, "seed")
	git(t, dir, "clone", "-q", origin, seed)
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, seed, "push", "-q", "origin", "main")
	mine := filepath.Join(dir, "mine")
	git(t, dir, "clone", "-q", origin, mine)
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "theirs")
	git(t, seed, "push", "-q", "origin", "main", "main:other")

	var saved State
	v, app, wait := startView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(mine)
	wait(func() bool { return v.repo.loaded && !v.repo.loading && v.tracking.Upstream != "" })

	// The dialog fetches every remote unless switched to the checked out
	// branch's upstream, which it then remembers.
	app.Press("id:toolbar-" + OpFetch)
	wait(func() bool { return v.dialog.remotes != nil })
	if v.dialog.remote != "origin" || v.dialog.branch != "main" {
		t.Fatalf("dialog %+v", v.dialog)
	}
	app.Press("label:Fetch all remotes")
	if !saved.FetchBranch {
		t.Fatalf("state %+v", saved)
	}
	app.Press("id:sync-dialog >> id:confirm")
	if v.Syncing() != OpFetch {
		t.Fatalf("fetch did not start: %q", v.Syncing())
	}
	wait(func() bool { return v.Syncing() == "" && v.tracking.Behind == 1 })
	if out, _ := v.git.Run(mine, "branch", "-r", "--list", "origin/other"); out != "" {
		t.Fatalf("fetched other branches: %q", out)
	}
	// The pull button names the fetched commit after its label.
	app.WaitFor("id:toolbar-"+OpPull+" >> text:/^1$/", time.Second)
	_ = slices.Contains[[]string]
}

// identity sets who commits in the tests' repositories.
var identity = Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")

// commitRow is the selector of the history's row of a commit's subject.
func commitRow(subject string) string { return "id:history >> text:/^" + subject + "$/" }

func TestViewRefAndCommitMenus(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	mine := filepath.Join(dir, "mine")
	git(t, dir, "clone", "-q", origin, mine)
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, mine, "tag", "v1")
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "second")
	git(t, mine, "push", "-q", "origin", "main", "main:other")
	git(t, mine, "branch", "feature")

	var opened string
	v, app, wait := startView(t, Options{Git: identity, OpenWorktree: func(path string) { opened = path }})
	v.SetPath(mine)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch != "" }
	wait(ready)

	// A local branch's menu offers checking it out and branching from it,
	// merging it into the checked out branch; a new branch is checked out
	// by default.
	app.RightClick("id:ref-branches/feature")
	menu := menuLabels(app)
	for _, item := range []string{"Checkout…", "Checkout as Worktree…", "New Branch…", "New Tag…", "Delete…", "Copy Branch Name", "Merge into 'main'…"} {
		if !slices.Contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	app.ChoosePopup("New Branch…")
	app.WaitFor("text:New branch at feature", time.Second)
	app.Type("id:ref-action-dialog >> role:textbox", "feature")
	app.WaitFor("text:A branch named feature already exists.", time.Second)
	app.Type("id:ref-action-dialog >> role:textbox", "-2")
	app.Press("id:ref-action-dialog >> id:confirm")
	wait(func() bool { return ready() && v.tracking.Branch == "feature-2" })

	// A commit's menu copies its hash, and clicking its row selects it.
	app.RightClick(commitRow("init"))
	app.ChoosePopup("Copy Commit Hash")
	initHash, _ := v.git.Run(mine, "rev-parse", "HEAD~1")
	if app.Clipboard != initHash {
		t.Fatalf("copied %q, want %q", app.Clipboard, initHash)
	}
	app.Press(commitRow("init"))
	if v.history.selected != initHash || v.action.open {
		t.Fatalf("selected %q, dialog %v", v.history.selected, v.action.open)
	}

	// Double-clicking a tag in the refs pane asks as well; its menu can
	// push the tag to a remote.
	app.Press("id:group-tags")
	app.RightClick("id:ref-tags/v1")
	if menu := menuLabels(app); !slices.Contains(menu, "Checkout Commit…") || !slices.Contains(menu, "Push to 'origin'…") {
		t.Fatalf("tag menu: %v", menu)
	}
	app.ClosePopup()
	app.Press("id:ref-tags/v1")
	app.Press("id:ref-tags/v1")
	app.WaitFor("text:Checkout Tag", time.Second)
	if !v.action.open {
		t.Fatal("the tag dialog did not open")
	}
	app.Press("id:ref-action-dialog >> id:cancel")
	// A branch switches without asking while there are no changes.
	app.Press("id:ref-branches/feature")
	app.Press("id:ref-branches/feature")
	wait(func() bool { return ready() && v.tracking.Branch == "feature" })
	if v.action.open {
		t.Fatal("branch dialog opened on a clean worktree")
	}

	// Activating a commit asks to check it out detached, here setting
	// uncommitted changes aside and back.
	app.Press(commitRow("init"))
	app.Press(commitRow("init"))
	app.WaitFor("text:Commit to checkout:", time.Second)
	os.WriteFile(filepath.Join(mine, "draft.txt"), []byte("draft\n"), 0644)
	app.Press("label:Stash and reapply")
	app.Press("id:ref-action-dialog >> id:confirm")
	wait(func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.head == initHash })
	if v.tracking.Branch != "" {
		t.Fatalf("still on %q", v.tracking.Branch)
	}
	if _, err := os.Stat(filepath.Join(mine, "draft.txt")); err != nil {
		t.Fatal(err)
	}

	git(t, mine, "switch", "-q", "main")
	v.Reload()
	wait(func() bool { return ready() && v.tracking.Branch == "main" && v.hasCurrentChanges() })

	// With changes, double-clicking a branch asks what to do with them.
	app.Press("id:ref-branches/feature")
	app.Press("id:ref-branches/feature")
	app.WaitFor("text:Checkout Branch", time.Second)
	app.Press("id:ref-action-dialog >> id:cancel")

	// A branch another worktree has checked out opens that worktree from
	// its menu, and double-clicking it does the same instead.
	linked := filepath.Join(dir, "linked")
	git(t, mine, "worktree", "add", "-q", "-b", "linked", linked)
	v.Reload()
	wait(func() bool { return ready() && len(v.repo.checkouts) == 2 })
	app.RightClick("id:ref-branches/linked")
	if menu := menuLabels(app); !slices.Contains(menu, "Open 'linked' Worktree") {
		t.Fatalf("linked menu: %v", menu)
	}
	app.ChoosePopup("Open 'linked' Worktree")
	if filepath.Base(opened) != "linked" {
		t.Fatalf("opened %q", opened)
	}
	opened = ""
	app.Press("id:ref-branches/linked")
	app.Press("id:ref-branches/linked")
	if v.action.open || filepath.Base(opened) != "linked" || v.tracking.Branch != "main" {
		t.Fatalf("opened %q, dialog %v, on %q", opened, v.action.open, v.tracking.Branch)
	}

	// A remote branch's pull starts from it.
	app.Press("id:group-remotes")
	app.RightClick("id:ref-remotes/origin/other")
	app.ChoosePopup("Pull into 'main'…")
	wait(func() bool { return v.dialog.remotes != nil })
	if v.dialog.op != OpPull || v.dialog.remote != "origin" || v.dialog.branch != "other" {
		t.Fatalf("pull dialog %+v", v.dialog)
	}
}

func TestViewCommitMenuListsItsRefs(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	repo := filepath.Join(dir, "repo")
	git(t, dir, "clone", "-q", origin, repo)
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, repo, "tag", "v1")
	git(t, repo, "branch", "feature")
	git(t, repo, "push", "-q", "origin", "main", "main:other")
	git(t, repo, "branch", "-q", "--track", "other", "origin/other")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "second")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "third")

	v, app, wait := startView(t, Options{Git: identity})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch == "main" }
	wait(ready)

	// A commit that refs point at leads its menu with the menu of each ref,
	// as Fork does: its branches, a local branch before its remote, then
	// its tags.
	app.RightClick(commitRow("init"))
	var menu []string
	for _, item := range app.PopupMenu {
		if item.Separator {
			menu = append(menu, "-")
		} else {
			menu = append(menu, item.Label)
		}
	}
	want := []string{"feature", "origin/main", "other", "origin/other", "-", "v1", "-", "New Branch…", "New Tag…"}
	if len(menu) < len(want) || !slices.Equal(menu[:len(want)], want) {
		t.Fatalf("menu %v, want it to start with %v", menu, want)
	}

	// A tag's submenu pushes the tag, and a new branch from a branch's
	// submenu starts at that branch.
	app.ChoosePopup("v1", "Push to 'origin'…")
	if !v.pushTag.open || v.pushTag.tag != "v1" || v.pushTag.remote != "origin" {
		t.Fatalf("push tag %+v", v.pushTag)
	}
	app.Press("id:push-tag-dialog >> id:cancel")
	app.RightClick(commitRow("init"))
	app.ChoosePopup("feature", "New Branch…")
	app.WaitFor("text:New branch at feature", time.Second)
	if !v.action.open || v.action.target.name != "feature" {
		t.Fatalf("branch dialog %+v", v.action.target)
	}
	app.Press("id:ref-action-dialog >> id:cancel")

	// A remote branch's submenu acts on that ref, not on the commit.
	app.RightClick(commitRow("init"))
	app.ChoosePopup("origin/other", "Copy Branch Name")
	if app.Clipboard != "origin/other" {
		t.Fatalf("copied %q", app.Clipboard)
	}

	// A commit no ref points at keeps its plain menu.
	app.RightClick(commitRow("second"))
	if menu := menuLabels(app); menu[0] != "New Branch…" || slices.Contains(menu, "v1") || slices.Contains(menu, "feature") {
		t.Fatalf("menu of a commit without refs: %v", menu)
	}
}

func TestViewDeletesBranchWithItsWorktree(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "side")
	wt := filepath.Join(t.TempDir(), "side-wt")
	git(t, repo, "worktree", "add", "-q", wt, "side")
	git(t, wt, "commit", "-q", "--allow-empty", "-m", "side work")
	var wrote []string
	v, app, wait := startView(t, Options{OnWrite: func(op, _ string) { wrote = append(wrote, op) }})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && len(v.repo.checkouts) > 0 }
	wait(ready)

	// The shown worktree's branch stays; another worktree's goes only with
	// that worktree.
	app.RightClick("id:ref-branches/main")
	if slices.Contains(menuLabels(app), "Delete…") {
		t.Fatal("offered deleting the shown worktree's branch")
	}
	app.ClosePopup()
	app.RightClick("id:ref-branches/side")
	app.ChoosePopup("Delete…")
	wait(func() bool { return !v.deletion.checking })
	app.WaitFor("label:Also delete worktree side-wt", time.Second)
	if !v.deletion.unmerged {
		t.Fatal("the unmerged branch was not marked")
	}
	// It needs the worktree and, as its commit is unmerged, the user's
	// choice to lose it.
	if app.Enabled("id:delete-dialog >> id:confirm") {
		t.Fatal("deletion offered without the worktree")
	}
	app.Press("label:Also delete worktree side-wt")
	if app.Enabled("id:delete-dialog >> id:confirm") {
		t.Fatal("deletion offered without losing the commits")
	}
	app.Press("label:Delete anyway, losing those commits")
	app.Press("id:delete-dialog >> id:confirm")
	wait(func() bool { return ready() && len(v.repo.checkouts) == 1 })
	if out, _ := v.git.Run(repo, "branch", "--list", "side"); out != "" {
		t.Fatalf("kept side: %q", out)
	}
	if !slices.Equal(wrote, []string{OpWorktree}) {
		t.Fatalf("wrote %v", wrote)
	}
}

func TestViewBranchMenu(t *testing.T) {
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	mine := filepath.Join(dir, "mine")
	git(t, dir, "clone", "-q", origin, mine)
	git(t, mine, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, mine, "push", "-q", "origin", "main")
	git(t, mine, "branch", "feature")
	wt := filepath.Join(dir, "wt")
	git(t, mine, "worktree", "add", "-q", "-b", "123", wt)
	git(t, wt, "commit", "-q", "--allow-empty", "-m", "work")

	v, app, wait := startView(t, Options{})
	v.SetPath(wt)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch != "" }
	wait(ready)
	branch := func(name string) *refNode {
		i := slices.IndexFunc(v.repo.refTrees.branches, func(n *refNode) bool { return n.path == name })
		return v.repo.refTrees.branches[i]
	}
	menu := func(ref string) []string {
		t.Helper()
		app.RightClick("id:ref-branches/" + ref)
		var labels []string
		for _, item := range app.PopupMenu {
			if item.Separator {
				labels = append(labels, "-")
			} else {
				labels = append(labels, item.Label)
			}
		}
		return labels
	}
	disabled := func(label string) bool {
		for _, item := range app.PopupMenu {
			if item.Label == label {
				return item.Disabled
			}
		}
		return true
	}

	// A branch another worktree has checked out leaves out what cannot
	// apply to it, and fast-forwards only once its upstream moves on.
	want := []string{"Fast-Forward to 'origin/main'", "Push to 'origin'…", "-", "Merge into '123'…", "Rebase '123' on 'main'…",
		"Interactively Rebase '123' onto 'main'…", "-", "New Branch…", "New Tag…", "-", "Tracking", "Rename…", "Pin Reference", "-", "Copy Branch Name"}
	if got := menu("main"); !slices.Equal(got, want) {
		t.Fatalf("menu %q", got)
	}
	if !disabled("Fast-Forward to 'origin/main'") {
		t.Fatal("offered fast-forwarding to the same commit")
	}
	app.ClosePopup()
	theirs := filepath.Join(dir, "theirs")
	git(t, dir, "clone", "-q", origin, theirs)
	git(t, theirs, "commit", "-q", "--allow-empty", "-m", "theirs")
	git(t, theirs, "push", "-q", "origin", "main")
	git(t, mine, "fetch", "-q")
	v.Reload()
	wait(func() bool { return ready() && branch("main").behind == 1 })
	menu("main")
	app.ChoosePopup("Fast-Forward to 'origin/main'")
	wait(func() bool { return ready() && branch("main").behind == 0 })
	if head, _ := v.git.Run(mine, "log", "-1", "--format=%s"); head != "theirs" {
		t.Fatalf("main worktree at %q", head)
	}

	// Its upstream changes from the tracking submenu.
	menu("main")
	app.ChoosePopup("Tracking", "None")
	wait(func() bool { return ready() && branch("main").upstream == RemoteBranch{} })
	if got := menu("main"); slices.Contains(got, "Fast-Forward to 'origin/main'") {
		t.Fatalf("untracked menu %q", got)
	}
	app.ClosePopup()

	// A branch no worktree has checks out, pushes from its own dialog and
	// renames.
	got := menu("feature")
	for _, item := range []string{"Checkout…", "Checkout as Worktree…", "Push to 'origin'…", "Delete…"} {
		if !slices.Contains(got, item) {
			t.Fatalf("%q missing from %q", item, got)
		}
	}
	app.ChoosePopup("Push to 'origin'…")
	wait(func() bool { return v.dialog.remotes != nil })
	if v.dialog.op != OpPush || v.dialog.branch != "feature" || v.dialog.to != (RemoteBranch{"origin", "feature"}) {
		t.Fatalf("push dialog %+v", v.dialog)
	}
	app.Press("id:sync-dialog >> id:cancel")
	menu("feature")
	app.ChoosePopup("Rename…")
	app.WaitFor("text:Rename feature", time.Second)
	app.Type("id:ref-action-dialog >> role:textbox", "-2")
	app.Press("id:ref-action-dialog >> id:confirm")
	wait(func() bool { return ready() && hasRef(v.repo.refTrees.branches, "feature-2") })
}

// With Worktrees set, the refs pane lists the repository's worktrees between
// its entries and the branches once it has more than one; double-clicking
// another opens it through OpenWorktree.
func TestViewListsWorktrees(t *testing.T) {
	// The view shows a path as Git lists it.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "demo")
	git(t, root, "init", "-q", "-b", "main", repo)
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")

	var opened string
	v, app, wait := startView(t, Options{Worktrees: true, OpenWorktree: func(path string) { opened = path }})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && len(v.repo.checkouts) == 1 })
	if app.Exists("id:group-worktrees") {
		t.Fatal("the only worktree listed")
	}

	other := filepath.Join(root, "demo-topic")
	git(t, repo, "worktree", "add", "-q", "-b", "topic", other)
	v.Reload()
	wait(func() bool { return !v.Loading() && len(v.repo.checkouts) == 2 })
	group := app.WaitFor("id:group-worktrees", time.Second)
	all, branches := app.Find("id:all-commits"), app.Find("id:group-branches")
	if group.Bounds().Origin.Y <= all.Bounds().Origin.Y || group.Bounds().Origin.Y >= branches.Bounds().Origin.Y {
		t.Fatalf("worktrees group out of place:\n%s", app.Dump())
	}
	app.Press("id:ref-worktrees/" + repo)
	app.Press("id:ref-worktrees/" + repo)
	if opened != "" {
		t.Fatalf("the shown worktree opened %q", opened)
	}
	app.Press("id:ref-worktrees/" + other)
	app.Press("id:ref-worktrees/" + other)
	if filepath.Base(opened) != "demo-topic" {
		t.Fatalf("opened %q", opened)
	}

	// Without the option, the group stays out.
	plain, view, wait := startView(t, Options{})
	plain.SetPath(repo)
	wait(func() bool { return !plain.Loading() && len(plain.repo.checkouts) == 2 })
	if view.Exists("id:group-worktrees") {
		t.Fatal("worktrees listed without the option")
	}
}
