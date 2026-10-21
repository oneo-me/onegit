package gitview

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// testView builds a view whose results return through pump, which runs them
// until cond holds.
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

func viewFrame(v *View) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { v.Layout(c) })
	}
}

func testRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("1\n"), 0644)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	git(t, repo, "tag", "v1")
	return repo
}

func TestViewHidesFilteredRefsAndShowsEntries(t *testing.T) {
	repo := testRepo(t)
	wt := filepath.Join(t.TempDir(), "task")
	git(t, repo, "worktree", "add", "-q", "-b", "host/task", wt)
	git(t, wt, "commit", "-q", "--allow-empty", "-m", "task work")
	git(t, repo, "branch", "host/old", "main")
	git(t, repo, "branch", "feature", "main")
	os.WriteFile(filepath.Join(wt, "draft.txt"), []byte("draft\n"), 0644)

	v, pump := testView(t, Options{
		RefFilter: func(kind RefKind, name string) bool { return kind != RefBranch || !strings.HasPrefix(name, "host/") },
		Entries: func() []Entry {
			return []Entry{{ID: "task", Title: "My task", Head: "host/task", Watch: wt,
				Lead:    func(c *ui.Context) { components.Text(c, "Lead") },
				Author:  func(c *ui.Context) { components.Text(c, "Agent") },
				Changes: func(git Git) (Changes, error) { return git.WorktreeChanges(wt, "main") },
				Files:   func(git Git) (Changes, error) { return git.WorktreeFiles(wt) }}}
		},
	})
	v.SetPath(repo)
	pump(func() bool { return v.repo.loaded && !v.repo.loading && v.current.loaded })

	// The entry's row stands above the commit it names, whose history joins
	// although its branch is hidden; the newest commit is selected first.
	commits := v.repo.commits
	if len(commits) != 3 || commits[0].Hash != entryPrefix+"task" || commits[1].Subject != "task work" ||
		commits[0].Parents[0] != commits[1].Hash || len(commits[1].Refs) != 0 || v.history.selected != commits[1].Hash {
		t.Fatalf("history: %+v, selected %q", commits, v.history.selected)
	}
	if b := v.repo.refTrees.branches; len(b) != 2 || b[0].path != "feature" || b[1].path != "main" {
		t.Fatalf("branches: %+v", b)
	}
	if v.repo.count != 2 {
		t.Fatalf("count: %d", v.repo.count)
	}
	view := ui.NewTester(viewFrame(v), 1000, 700)
	if !view.HasText("My task") || !view.HasText("Lead") || !view.HasText("Agent") || !view.HasText("All commits") || !view.HasText("Branches") {
		t.Fatalf("view: %v", view.Texts())
	}

	// Choosing the row shows its changes through the host's reader.
	for range 2 {
		if err := view.Click("My task"); err != nil {
			t.Fatal(err)
		}
	}
	if entry, _ := v.Selected(); entry != "task" {
		t.Fatalf("selected: %q", entry)
	}
	pump(func() bool { return v.history.changes.loaded && !v.history.changes.loading })
	view.Frame()
	if !view.HasText("draft.txt") {
		t.Fatalf("entry changes: %v", view.Texts())
	}

	// A host reveals the entry's row after choosing another one.
	v.revealCommit(commits[1].Hash)
	v.RevealEntry("task")
	if entry, _ := v.Selected(); entry != "task" || v.history.cursor != 0 {
		t.Fatalf("revealed: %q at %d", entry, v.history.cursor)
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
	v, pump := testView(t, Options{
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
	pump(func() bool { return v.repo.loaded && !v.repo.loading && v.current.loaded && !v.current.loading })

	// Changes found after opening replace the history.
	if v.section != sectionChanges || !v.hasCurrentChanges() {
		t.Fatalf("section %v", v.section)
	}
	view := ui.NewTester(viewFrame(v), 1000, 700)
	if !view.HasText("当前更改") || !view.HasText("全部提交") {
		t.Fatalf("view: %v", view.Texts())
	}

	// The toolbar offers every action. Its stash dialog saves a stash with
	// untracked files, one write at a time, and rereads the view; the
	// emptied changes stay shown.
	toolbar := ui.NewTester(func(c *ui.Context) { v.Toolbar(c, 32) }, 600, 48)
	for _, label := range []string{"获取", "拉取", "推送", "贮藏"} {
		if _, ok := toolbar.Find(label); !ok {
			t.Fatalf("%s missing from the toolbar", label)
		}
	}
	if _, ok := toolbar.Find("分支操作"); ok {
		t.Fatal("the toolbar still offers a branch menu")
	}
	if err := toolbar.Click("贮藏"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.stashDialog.open || !view.HasText("包含未跟踪的文件") {
		t.Fatalf("stash dialog: %v", view.Texts())
	}
	view.Type("my work")
	view.Key(0, ui.KeyEnter)
	if v.Syncing() != OpStash || v.CanSync() {
		t.Fatalf("syncing %q", v.Syncing())
	}
	pump(func() bool { return v.Syncing() == "" })
	if len(written) != 1 || written[0] != OpStash || writtenPaths[0] != repo {
		t.Fatalf("written: %v in %v", written, writtenPaths)
	}
	pump(func() bool { return v.current.loaded && !v.current.loading })
	if v.section != sectionChanges || v.hasCurrentChanges() {
		t.Fatalf("after stash: section %v, files %v", v.section, v.current.changes.Files)
	}
	pump(func() bool { return !v.Loading() && len(v.repo.stashes) == 1 })
	toolbar.Frame()
	view.Frame()
	if !view.HasText("当前更改") || !view.HasText("已暂存的更改") || view.HasText("没有更改") || v.CanStash() {
		t.Fatalf("after stash: %v", view.Texts())
	}

	// The refs pane lists the stash, which shows the files it saved.
	if err := view.Click("On main: my work"); err != nil {
		t.Fatalf("%v: %v", err, view.Texts())
	}
	pump(func() bool { return v.stashFiles.loaded && !v.stashFiles.loading && !v.stashFiles.fileLoading })
	view.Frame()
	if v.section != sectionStash || !view.HasText("a.txt") || !view.HasText("new.txt") {
		t.Fatalf("stash: section %v, %v", v.section, view.Texts())
	}
	for _, f := range v.stashFiles.changes.Files {
		before, after, err := v.git.FileVersions(v.stashFiles.changes, f)
		if err != nil || f.Path == "a.txt" && (string(before) != "1\n" || string(after) != "2\n") ||
			f.Path == "new.txt" && (before != nil || string(after) != "new\n") {
			t.Fatalf("%s: %q, %q, %v", f.Path, before, after, err)
		}
	}

	// Double-clicking the stash asks to apply it, deleting it by default;
	// once gone, it leaves for the history.
	view.Advance(time.Second)
	for range 2 {
		if err := view.Click("On main: my work"); err != nil {
			t.Fatal(err)
		}
	}
	view.Frame()
	if !v.applyStash.open || !view.HasText("应用后删除贮藏") || !view.HasText("stash@{0}") {
		t.Fatalf("apply dialog: %v", view.Texts())
	}
	if err := view.Click("应用"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && !v.Loading() && len(v.repo.stashes) == 0 })
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
	pump(func() bool { return v.current.loaded && !v.current.loading })
	view.Frame()
	if v.section != sectionCommits || !view.HasText("当前更改") {
		t.Fatalf("clean: section %v, %v", v.section, view.Texts())
	}
	if err := view.Click("当前更改"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.current.loaded && !v.current.loading })
	if v.section != sectionChanges {
		t.Fatalf("chosen: section %v", v.section)
	}

	// Collapsing a ref group is remembered.
	view.Frame()
	if err := view.Click("标签"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if open, ok := saved.Groups["tags"]; !ok || !open {
		t.Fatalf("groups: %v", saved.Groups)
	}

	// A write running in one repository leaves another's writes free.
	release := make(chan struct{})
	v.Run("host", "failed", func(Git, string) error { <-release; return nil })
	other := testRepo(t)
	v.SetPath(other)
	pump(func() bool { return v.repo.loaded && !v.repo.loading })
	if v.Syncing() != "" || !v.CanSync() {
		t.Fatalf("other repository: syncing %q", v.Syncing())
	}
	v.SetPath(repo)
	if v.Syncing() != "host" {
		t.Fatalf("back: syncing %q", v.Syncing())
	}
	close(release)
	pump(func() bool { return v.Syncing() == "" })
	if n := len(written); written[n-1] != "host" || writtenPaths[n-1] != repo {
		t.Fatalf("written: %v in %v", written, writtenPaths)
	}
}

func TestViewRemembersRefFolders(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "feature/login", "main")

	var saved State
	v, pump := testView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(repo)
	pump(func() bool { return v.repo.loaded && !v.repo.loading })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	if !slices.Contains(view.Texts(), "login") {
		t.Fatalf("open folder: %v", view.Texts())
	}

	// Collapsing a folder saves it; reopening drops it again.
	if err := view.Click("feature"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if open, ok := saved.Groups["branches/feature/"]; !ok || open || slices.Contains(view.Texts(), "login") {
		t.Fatalf("collapsed: %v, %v", saved.Groups, view.Texts())
	}

	// Another view restores it collapsed.
	other, pump := testView(t, Options{State: saved})
	other.SetPath(repo)
	pump(func() bool { return other.repo.loaded && !other.repo.loading })
	restored := ui.NewTester(viewFrame(other), 1000, 700)
	if !restored.HasText("feature") || slices.Contains(restored.Texts(), "login") {
		t.Fatalf("restored: %v", restored.Texts())
	}

	view.Advance(time.Second)
	if err := view.Click("feature"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if _, ok := saved.Groups["branches/feature/"]; ok || !slices.Contains(view.Texts(), "login") {
		t.Fatalf("reopened: %v, %v", saved.Groups, view.Texts())
	}
}

func TestViewShowsErrorOutsideRepository(t *testing.T) {
	v, pump := testView(t, Options{})
	v.SetPath(t.TempDir())
	pump(func() bool { return v.repo.loaded && !v.repo.loading })
	view := ui.NewTester(viewFrame(v), 800, 600)
	if !view.HasText("This project has no Git history to compare") || v.CanSync() {
		t.Fatalf("view: %v", view.Texts())
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
	v, pump := testView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(mine)
	pump(func() bool { return v.repo.loaded && !v.repo.loading && v.tracking.Upstream != "" })
	view := ui.NewTester(viewFrame(v), 1000, 700)

	// The dialog fetches every remote unless switched to the checked out
	// branch's upstream, which it then remembers.
	v.OpenFetch()
	pump(func() bool { return v.dialog.remotes != nil })
	view.Frame()
	if v.dialog.remote != "origin" || v.dialog.branch != "main" || !view.HasText("Fetch all remotes") {
		t.Fatalf("dialog %+v: %v", v.dialog, view.Texts())
	}
	if err := view.Click("Fetch all remotes"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !saved.FetchBranch {
		t.Fatalf("state %+v", saved)
	}
	// The switch took focus; the fetch button follows Cancel.
	view.Key(0, ui.KeyTab)
	view.Key(0, ui.KeyTab)
	view.Key(0, ui.KeyEnter)
	if v.Syncing() != OpFetch {
		t.Fatalf("fetch did not start: %q", v.Syncing())
	}
	pump(func() bool { return v.Syncing() == "" && v.tracking.Behind == 1 })
	if out, _ := v.git.Run(mine, "branch", "-r", "--list", "origin/other"); out != "" {
		t.Fatalf("fetched other branches: %q", out)
	}
	// The pull button names the fetched commit after its label.
	toolbar := ui.NewTester(func(c *ui.Context) { v.Toolbar(c, 32) }, 600, 48)
	if texts := toolbar.Texts(); !slices.Equal(texts, []string{"Fetch", "Fetch", "Pull", "Pull", "1", "Push", "Push", "Stash", "Stash"}) {
		t.Fatalf("toolbar: %v", texts)
	}
}

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
	v, pump := testView(t, Options{
		Git:          Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"),
		OpenWorktree: func(path string) { opened = path },
	})
	v.SetPath(mine)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch != "" }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// A local branch's menu offers checking it out and branching from it,
	// but not merging the checked out commit; a new branch is checked out
	// by default.
	if err := view.RightClick("feature"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	for _, item := range []string{"Checkout…", "Checkout as Worktree…", "New Branch…", "New Tag…", "Delete…", "Copy Branch Name"} {
		if !slices.Contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
	}
	if !slices.Contains(menu, "Merge into 'main'…") {
		t.Fatalf("no merge: %v", menu)
	}
	if err := view.ChooseMenuItem("New Branch…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !v.action.open || !view.HasText("New branch at feature") {
		t.Fatalf("dialog: %v", view.Texts())
	}
	view.Frame()
	view.Type("feature")
	view.Frame()
	if !view.HasText("A branch named feature already exists.") {
		t.Fatalf("taken name: %v", view.Texts())
	}
	view.Type("-2")
	view.Frame()
	if err := view.Click("Create Branch"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.tracking.Branch == "feature-2" })

	// A commit's menu copies its hash, and clicking its row still selects
	// it.
	view.Frame()
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Copy Commit Hash"); err != nil {
		t.Fatal(err)
	}
	initHash, _ := v.git.Run(mine, "rev-parse", "HEAD~1")
	if view.Clipboard() != initHash {
		t.Fatalf("copied %q, want %q", view.Clipboard(), initHash)
	}
	view.Advance(time.Second)
	if err := view.Click("init"); err != nil {
		t.Fatal(err)
	}
	if v.history.selected != initHash || v.action.open {
		t.Fatalf("selected %q, dialog %v", v.history.selected, v.action.open)
	}

	// Double-clicking a tag in the refs pane asks as well; its menu can
	// push the tag to a remote.
	view.Advance(time.Second)
	if err := view.Click("Tags"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	if err := view.RightClick("v1"); err != nil {
		t.Fatal(err)
	}
	if menu := view.Menu(); !slices.Contains(menu, "Checkout Commit…") || !slices.Contains(menu, "Push to 'origin'…") {
		t.Fatalf("tag menu: %v", menu)
	}
	view.CloseMenu()
	for range 2 {
		if err := view.Click("v1"); err != nil {
			t.Fatal(err)
		}
	}
	view.Frame()
	if !v.action.open || !view.HasText("Checkout Tag") {
		t.Fatalf("tag dialog: %v", view.Texts())
	}
	if err := view.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	for range 2 {
		if err := view.Click("feature"); err != nil {
			t.Fatal(err)
		}
	}
	// A branch switches without asking while there are no changes.
	pump(func() bool { return ready() && v.tracking.Branch == "feature" })
	if v.action.open {
		t.Fatalf("branch dialog opened on a clean worktree: %v", view.Texts())
	}

	// Double-clicking a commit asks to check it out detached, here setting
	// uncommitted changes aside and back.
	view.Advance(time.Second)
	for range 2 {
		if err := view.Click("init"); err != nil {
			t.Fatal(err)
		}
	}
	view.Frame()
	if !v.action.open || !view.HasText("Checkout Commit") || !view.HasText("Commit to checkout:") {
		t.Fatalf("commit dialog: %v", view.Texts())
	}
	os.WriteFile(filepath.Join(mine, "draft.txt"), []byte("draft\n"), 0644)
	if err := view.Click("Stash and reapply"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.Click("Checkout"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.Loading() && v.Syncing() == "" && v.repo.head == initHash })
	if v.tracking.Branch != "" {
		t.Fatalf("still on %q", v.tracking.Branch)
	}
	if _, err := os.Stat(filepath.Join(mine, "draft.txt")); err != nil {
		t.Fatal(err)
	}

	git(t, mine, "switch", "-q", "main")
	v.Reload()
	pump(func() bool { return ready() && v.tracking.Branch == "main" })

	// With changes, double-clicking a branch asks what to do with them.
	pump(func() bool { return v.hasCurrentChanges() })
	view.Advance(time.Second)
	for range 2 {
		if err := view.Click("feature"); err != nil {
			t.Fatal(err)
		}
	}
	view.Frame()
	if !v.action.open || !view.HasText("Checkout Branch") {
		t.Fatalf("branch dialog: %v", view.Texts())
	}
	if err := view.Click("Cancel"); err != nil {
		t.Fatal(err)
	}

	// A branch another worktree has checked out opens that worktree from
	// its menu, and double-clicking it does the same instead.
	linked := filepath.Join(dir, "linked")
	git(t, mine, "worktree", "add", "-q", "-b", "linked", linked)
	v.Reload()
	pump(func() bool { return ready() && len(v.repo.checkouts) == 2 })
	view.Advance(time.Second)
	if err := view.RightClick("linked"); err != nil {
		t.Fatal(err)
	}
	if menu := view.Menu(); !slices.Contains(menu, "Open 'linked' Worktree") {
		t.Fatalf("linked menu: %v", menu)
	}
	if err := view.ChooseMenuItem("Open 'linked' Worktree"); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(opened) != "linked" {
		t.Fatalf("opened %q", opened)
	}
	view.Advance(time.Second)
	for range 2 {
		if err := view.Click("linked"); err != nil {
			t.Fatal(err)
		}
	}
	view.Frame()
	if v.action.open || filepath.Base(opened) != "linked" || v.tracking.Branch != "main" {
		t.Fatalf("opened %q, dialog %v, on %q", opened, v.action.open, v.tracking.Branch)
	}

	// A remote branch's pull starts from it.
	if err := view.Click("Remote branches"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.RightClick("other"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Pull into 'main'…"); err != nil {
		t.Fatalf("%v: %v", err, view.Menu())
	}
	pump(func() bool { return v.dialog.remotes != nil })
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

	v, pump := testView(t, Options{
		Git: Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"),
	})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch == "main" }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// A commit that refs point at leads its menu with the menu of each ref,
	// as Fork does: its branches, a local branch before its remote, then
	// its tags.
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	want := []string{"feature", "origin/main", "other", "origin/other", "-", "v1", "-", "New Branch…", "New Tag…"}
	if len(menu) < len(want) || !slices.Equal(menu[:len(want)], want) {
		t.Fatalf("menu %v, want it to start with %v", menu, want)
	}

	// A tag's submenu pushes the tag, and a new branch from a branch's
	// submenu starts at that branch.
	if err := view.ChooseMenuItem("v1", "Push to 'origin'…"); err != nil {
		t.Fatalf("%v: %v", err, menu)
	}
	view.Frame()
	if !v.pushTag.open || v.pushTag.tag != "v1" || v.pushTag.remote != "origin" {
		t.Fatalf("push tag %+v", v.pushTag)
	}
	if err := view.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("feature", "New Branch…"); err != nil {
		t.Fatalf("%v: %v", err, view.Menu())
	}
	view.Frame()
	if !v.action.open || v.action.target.name != "feature" || !view.HasText("New branch at feature") {
		t.Fatalf("branch dialog %+v: %v", v.action.target, view.Texts())
	}
	if err := view.Click("Cancel"); err != nil {
		t.Fatal(err)
	}

	// A remote branch's submenu acts on that ref, not on the commit.
	view.Advance(time.Second)
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("origin/other", "Copy Branch Name"); err != nil {
		t.Fatalf("%v: %v", err, view.Menu())
	}
	if view.Clipboard() != "origin/other" {
		t.Fatalf("copied %q", view.Clipboard())
	}

	// A commit no ref points at keeps its plain menu.
	view.Advance(time.Second)
	if err := view.RightClick("second"); err != nil {
		t.Fatal(err)
	}
	if menu := view.Menu(); menu[0] != "New Branch…" || slices.Contains(menu, "v1") || slices.Contains(menu, "feature") {
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
	v, pump := testView(t, Options{OnWrite: func(op, _ string) { wrote = append(wrote, op) }})
	v.SetPath(repo)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && len(v.repo.checkouts) > 0 }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// The shown worktree's branch stays; another worktree's goes only with
	// that worktree.
	if err := view.RightClick("main"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Delete…"); err == nil {
		t.Fatal("offered deleting the shown worktree's branch")
	}
	view.Key(0, ui.KeyEscape)
	view.Advance(time.Second)
	if err := view.RightClick("side"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Delete…"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.deletion.checking })
	view.Frame()
	if !view.HasText("Also delete worktree side-wt") || !v.deletion.unmerged {
		t.Fatalf("dialog: %v", view.Texts())
	}
	// It needs the worktree and, as its commit is unmerged, the user's
	// choice to lose it.
	for _, choice := range []string{"", "Also delete worktree side-wt"} {
		if choice != "" {
			if err := view.Click(choice); err != nil {
				t.Fatal(err)
			}
			view.Frame()
		}
		if err := view.Click("Delete"); err != nil {
			t.Fatal(err)
		}
		view.Frame()
		if v.Syncing() != "" || !v.deletion.open {
			t.Fatalf("deleted without %q", choice)
		}
	}
	if err := view.Click("Delete anyway, losing those commits"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && len(v.repo.checkouts) == 1 })
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

	v, pump := testView(t, Options{})
	v.SetPath(wt)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch != "" }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	branch := func(name string) *refNode {
		i := slices.IndexFunc(v.repo.refTrees.branches, func(n *refNode) bool { return n.path == name })
		return v.repo.refTrees.branches[i]
	}
	menu := func(ref string) []string {
		t.Helper()
		view.Advance(time.Second)
		if err := view.RightClick(ref); err != nil {
			t.Fatal(err)
		}
		return view.Menu()
	}

	// A branch another worktree has checked out leaves out what cannot
	// apply to it, and fast-forwards only once its upstream moves on.
	want := []string{"Fast-Forward to 'origin/main'", "Push to 'origin'…", "-", "Merge into '123'…", "Rebase '123' on 'main'…",
		"Interactively Rebase '123' onto 'main'…", "-", "New Branch…", "New Tag…", "-", "Tracking", "Rename…", "Pin Reference", "-", "Copy Branch Name"}
	if got := menu("main"); !slices.Equal(got, want) {
		t.Fatalf("menu %q", got)
	}
	if err := view.ChooseMenuItem("Fast-Forward to 'origin/main'"); err == nil {
		t.Fatal("fast-forwarded to the same commit")
	}
	theirs := filepath.Join(dir, "theirs")
	git(t, dir, "clone", "-q", origin, theirs)
	git(t, theirs, "commit", "-q", "--allow-empty", "-m", "theirs")
	git(t, theirs, "push", "-q", "origin", "main")
	git(t, mine, "fetch", "-q")
	v.Reload()
	pump(ready)
	menu("main")
	if err := view.ChooseMenuItem("Fast-Forward to 'origin/main'"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && branch("main").behind == 0 })
	if head, _ := v.git.Run(mine, "log", "-1", "--format=%s"); head != "theirs" {
		t.Fatalf("main worktree at %q", head)
	}

	// Its upstream changes from the tracking submenu.
	menu("main")
	if err := view.ChooseMenuItem("Tracking", "None"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && branch("main").upstream == RemoteBranch{} })
	if got := menu("main"); slices.Contains(got, "Fast-Forward to 'origin/main'") {
		t.Fatalf("untracked menu %q", got)
	}

	// A branch no worktree has checks out, pushes from its own dialog and
	// renames.
	got := menu("feature")
	for _, item := range []string{"Checkout…", "Checkout as Worktree…", "Push to 'origin'…", "Delete…"} {
		if !slices.Contains(got, item) {
			t.Fatalf("%q missing from %q", item, got)
		}
	}
	if err := view.ChooseMenuItem("Push to 'origin'…"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.dialog.remotes != nil })
	if v.dialog.op != OpPush || v.dialog.branch != "feature" || v.dialog.to != (RemoteBranch{"origin", "feature"}) {
		t.Fatalf("push dialog %+v", v.dialog)
	}
	v.dialog.open = false
	menu("feature")
	if err := view.ChooseMenuItem("Rename…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if !view.HasText("Rename feature") {
		t.Fatalf("rename dialog: %v", view.Texts())
	}
	view.Frame()
	view.Type("-2")
	view.Frame()
	if err := view.Click("Rename"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && hasRef(v.repo.refTrees.branches, "feature-2") })
}
