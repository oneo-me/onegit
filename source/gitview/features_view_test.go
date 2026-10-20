package gitview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewConflictBannerResolvesAndContinues(t *testing.T) {
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
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return v.repo.loaded && !v.repo.loading && v.current.loaded && !v.current.loading })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if v.repo.operation.Kind != OperationMerge || !view.HasText("Merge in progress") || !view.HasText("1 conflict remains") {
		t.Fatalf("banner %+v: %v", v.repo.operation, view.Texts())
	}
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Resolve Conflict", "Use Ours"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && !v.Loading() && v.conflictCount() == 0 })
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "main\n" {
		t.Fatalf("resolved file %q", data)
	}
	view.Frame()
	if !view.HasText("All conflicts resolved") {
		t.Fatalf("resolved banner: %v", view.Texts())
	}
	if err := view.Click("Continue"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && !v.Loading() && v.repo.operation.Kind == OperationNone })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "Merge") {
		t.Fatalf("merge commit %q", subject)
	}
}

func TestViewDiscardsHunk(t *testing.T) {
	repo := testRepo(t)
	lines := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte(lines), 0644)
	git(t, repo, "commit", "-q", "-am", "lines")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n2\n3\n4\n5\n6\n7\n8\n9\nten\n"), 0644)
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	p := v.current
	pump(func() bool { return p.loaded && !p.loading && !p.fileLoading && p.diff != nil })
	if len(p.diff.Hunks()) != 2 {
		t.Fatalf("hunks %+v", p.diff.Hunks())
	}
	v.discardHunk(p, p.diff.Hunks()[0])
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if !view.HasText("Discard This Change in a.txt?") {
		t.Fatalf("dialog: %v", view.Texts())
	}
	if err := view.Click("Discard"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && !v.Loading() })
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "1\n2\n3\n4\n5\n6\n7\n8\n9\nten\n" {
		t.Fatalf("file %q", data)
	}
}

func TestViewAmendsAndPushes(t *testing.T) {
	repo := testRepo(t)
	dir := t.TempDir()
	bare := filepath.Join(dir, "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", bare)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "add", "a.txt")
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	v.messages[repo] = "amended"
	v.amend = true
	view.Frame()
	if err := view.Click("Amend Last Commit"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && !v.Loading() })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); subject != "amended" {
		t.Fatalf("amended %q", subject)
	}
	git(t, repo, "remote", "add", "origin", bare)
	git(t, repo, "push", "-q", "-u", "origin", "main")
	v.Reload()
	pump(func() bool { return !v.Loading() && v.tracking.Upstream == "origin/main" })
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	git(t, repo, "add", "a.txt")
	v.Reload()
	pump(func() bool { return !v.Loading() && v.Syncing() == "" })
	v.messages[repo] = "pushed"
	view.Frame()
	if err := view.Click("Commit & Push"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.Syncing() == "" && !v.Loading() })
	local, _ := v.git.Run(repo, "rev-parse", "main")
	remote, _ := v.git.Run(bare, "rev-parse", "main")
	if local != remote {
		t.Fatalf("pushed %q, want %q", remote, local)
	}
}

func TestViewDraftPersists(t *testing.T) {
	repo := testRepo(t)
	var saved State
	v, pump := testView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() })
	v.messages[repo] = "draft"
	v.saveDraft()
	if saved.Drafts[repo] != "draft" {
		t.Fatalf("drafts %+v", saved.Drafts)
	}
	view := New(Options{State: saved, Dispatch: func(func()) {}})
	if view.messages[repo] != "draft" {
		t.Fatalf("restored messages %+v", view.messages)
	}
	other := testRepo(t)
	v.SetPath(other)
	if v.messages[repo] != "draft" || saved.Drafts[repo] != "draft" {
		t.Fatalf("switch lost draft: %+v / %+v", v.messages, saved.Drafts)
	}
}

func TestViewPinsRefsAndComparesCommits(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "feature")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	var saved State
	v, pump := testView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() && v.repo.count >= 2 })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if err := view.RightClick("feature"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Pin Reference"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if len(saved.Pinned) != 1 || !view.HasText("Pinned") {
		t.Fatalf("pinned %+v: %v", saved.Pinned, view.Texts())
	}
	if err := view.Click("second"); err != nil {
		t.Fatal(err)
	}
	if err := view.ClickWith(ui.Cmd, "init"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Compare Selected Commits…"); err != nil {
		t.Fatalf("%v: %v", err, view.Menu())
	}
	pump(func() bool { return v.compare.files.loaded && !v.compare.files.loading })
	if v.section != sectionCompare {
		t.Fatalf("section %v", v.section)
	}
	view.Frame()
	if !view.HasText("a.txt") {
		t.Fatalf("comparison: %v", view.Texts())
	}
}

func TestViewReflogToggle(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "lost")
	lost, _ := vGit(repo, "rev-parse", "HEAD")
	git(t, repo, "reset", "-q", "--hard", "HEAD~1")
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if err := view.RightClick("All commits"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Show Lost Commits (Reflog)"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return v.state.ShowReflog && !v.Loading() && !v.repo.loading })
	view.Frame()
	found := false
	for _, c := range v.repo.commits {
		if c.Hash == lost {
			found = true
		}
	}
	if !found || !view.HasText("All commits (Reflog)") {
		t.Fatalf("reflog %+v: %v", v.repo.commits, view.Texts())
	}
}

func TestViewFileHistoryAndBlame(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() && v.current.loaded && !v.current.loading })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("File History…"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.fileHistory.loading })
	view.Frame()
	if !view.HasText("History of a.txt") || !view.HasText("second") {
		t.Fatalf("history: %v", view.Texts())
	}
	v.fileHistory.open = false
	view.Frame()
	if err := view.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Blame…"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return !v.blame.loading })
	view.Frame()
	if !view.HasText("Blame of a.txt") || !view.HasText("3") {
		t.Fatalf("blame: %v", view.Texts())
	}
}

func TestViewSearchesHistory(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("needle\n"), 0644)
	git(t, repo, "add", "b.txt")
	git(t, repo, "commit", "-q", "-m", "third")
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() })
	v.openHistorySearch()
	v.search.query, v.search.typ, v.search.scope = "needle", SearchContent, SearchRepository
	v.runHistorySearch()
	pump(func() bool { return !v.search.loading })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if !view.HasText("Search History") || !view.HasText("third") {
		t.Fatalf("search: %v", view.Texts())
	}
}

func TestViewBulkDeletesBranches(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "feature")
	git(t, repo, "branch", "other")
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if err := view.RightClick("Branches"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Delete Multiple Branches…"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	for i := range v.bulk.refs {
		v.bulk.refs[i].On = true
	}
	v.runBulk()
	pump(func() bool { return v.Syncing() == "" && !v.Loading() })
	if out, _ := v.git.Run(repo, "branch", "--format=%(refname:short)"); strings.Contains(out, "feature") || strings.Contains(out, "other") {
		t.Fatalf("branches %q", out)
	}
}

func TestViewInteractiveRebaseDialog(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "third")
	v, pump := testView(t, Options{})
	v.SetPath(repo)
	pump(func() bool { return !v.Loading() && v.repo.count >= 3 })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	view.Frame()
	if err := view.RightClick("init"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("Interactively Rebase 'main' to Here…"); err != nil {
		t.Fatalf("%v: %v", err, view.Menu())
	}
	pump(func() bool { return !v.interactive.loading })
	view.Frame()
	if !v.interactive.open || len(v.interactive.rows) != 2 || !view.HasText("Interactively Rebase main") {
		t.Fatalf("dialog %+v: %v", v.interactive, view.Texts())
	}
	v.interactive.rows[0].Action = RebaseReword
	v.interactive.rows[0].Message = "second reworded"
	v.interactive.rows[1].Action = RebaseDrop
	v.runInteractiveRebase()
	pump(func() bool { return v.Syncing() == "" && !v.Loading() })
	if log, _ := v.git.Run(repo, "log", "--format=%s"); log != "second reworded\ninit" {
		t.Fatalf("log %q", log)
	}
}

// vGit runs git in dir and returns its trimmed output, for tests that only
// need one read.
func vGit(dir string, args ...string) (string, error) {
	cmd := Git{}
	return cmd.Run(dir, args...)
}
