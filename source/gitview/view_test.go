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
	toolbar := ui.NewTester(func(c *ui.Context) { v.Toolbar(c, 32) }, 400, 48)
	for _, label := range []string{"获取", "拉取", "推送", "贮藏", "分支操作"} {
		if _, ok := toolbar.Find(label); !ok {
			t.Fatalf("%s missing from the toolbar", label)
		}
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

	// The branch menu lists the shown local branches and switches to one.
	pump(func() bool { return !v.Loading() && v.tracking.Branch == "main" })
	toolbar.Frame()
	if err := toolbar.Click("分支操作"); err != nil {
		t.Fatal(err)
	}
	if got := toolbar.Menu(); !slices.Equal(got, []string{"feature", "main"}) {
		t.Fatalf("branch menu: %v", got)
	}
	if err := toolbar.ChooseMenuItem("feature"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && !v.Loading() && v.tracking.Branch == "feature" })

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

	v, pump := testView(t, Options{Git: Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")})
	v.SetPath(mine)
	ready := func() bool { return !v.Loading() && v.Syncing() == "" && v.tracking.Branch != "" }
	pump(ready)
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})

	// A local branch's menu offers checking it out, integrating it and
	// branching from it; a new branch is checked out by default.
	if err := view.RightClick("feature"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	for _, item := range []string{"Checkout…", "Checkout as Worktree…", "Merge into 'main'…", "Rebase 'main' on 'feature'…", "New Branch…", "New Tag…", "Delete…", "Copy Branch Name"} {
		if !slices.Contains(menu, item) {
			t.Fatalf("%q missing from %v", item, menu)
		}
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

	// Double-clicking a tag or a branch in the refs pane asks as well.
	view.Advance(time.Second)
	if err := view.Click("Tags"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
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
	view.Frame()
	if !v.action.open || !view.HasText("Checkout Branch") {
		t.Fatalf("branch dialog: %v", view.Texts())
	}
	if err := view.Click("Checkout"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return ready() && v.tracking.Branch == "feature" })

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

func TestViewDeletesBranchWithItsWorktree(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "side")
	wt := filepath.Join(t.TempDir(), "side-wt")
	git(t, repo, "worktree", "add", "-q", wt, "side")
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
	view.Frame()
	if !view.HasText("Also delete worktree side-wt") {
		t.Fatalf("dialog: %v", view.Texts())
	}
	if err := view.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if v.Syncing() != "" || !v.deletion.open {
		t.Fatal("deleted without the worktree")
	}
	if err := view.Click("Also delete worktree side-wt"); err != nil {
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
