package gitview

import (
	"os"
	"path/filepath"
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
	if !view.HasText("My task") || !view.HasText("Agent") || !view.HasText("All commits") || !view.HasText("Branches") {
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

func TestViewOffersChangesAndRunsHostActions(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	var saved State
	var written []string
	v, pump := testView(t, Options{Language: "zh-Hans", OnState: func(s State) { saved = s }, OnWrite: func(op string) { written = append(written, op) }})
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

	// A host action runs one at a time and rereads the view; the worktree
	// then has nothing to show, so the history returns.
	if !v.CanSync() {
		t.Fatal("cannot sync")
	}
	v.Run("stash", "stash failed", func(git Git, path string) error {
		_, err := git.Run(path, "-c", "user.name=t", "-c", "user.email=t@t", "stash", "push", "--include-untracked")
		return err
	})
	if v.Syncing() != "stash" || v.CanSync() {
		t.Fatalf("syncing %q", v.Syncing())
	}
	pump(func() bool { return v.Syncing() == "" })
	if len(written) != 1 || written[0] != "stash" {
		t.Fatalf("written: %v", written)
	}
	pump(func() bool { return v.current.loaded && !v.current.loading })
	if v.section != sectionCommits || v.hasCurrentChanges() {
		t.Fatalf("after stash: section %v, files %v", v.section, v.current.changes.Files)
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
