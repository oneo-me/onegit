package gitview

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/diff"
	"github.com/nsgo-dev/nsgo/core"
)

// fileRow is the selector of a file's row in a pane's tree of area.
func fileRow(area Area, name string) string {
	return fmt.Sprint("id:files-", area, " >> text:/^", name, "$/")
}

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
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return v.repo.loaded && !v.repo.loading && v.current.loaded && !v.current.loading })
	app.WaitFor("text:Merge in progress", time.Second)
	app.WaitFor("text:1 conflict remains", time.Second)
	if v.repo.operation.Kind != OperationMerge {
		t.Fatalf("operation %+v", v.repo.operation)
	}
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	app.ChoosePopup("Resolve Conflict", "Use Ours")
	wait(func() bool { return v.Syncing() == "" && !v.Loading() && v.conflictCount() == 0 })
	if data, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(data) != "main\n" {
		t.Fatalf("resolved file %q", data)
	}
	app.WaitFor("text:All conflicts resolved", time.Second)
	app.Press("id:operation-continue")
	wait(func() bool { return v.Syncing() == "" && !v.Loading() && v.repo.operation.Kind == OperationNone })
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
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	p := v.current
	wait(func() bool { return p.loaded && !p.loading && !p.fileLoading && p.compared })
	// The first change's lines go, keeping the second.
	v.discardRegion(p, diff.Region{Old: []int{0}, New: []int{0}})
	app.WaitFor("text:Discard This Change in a.txt?", time.Second)
	app.Press("id:discard-change-dialog >> id:confirm")
	wait(func() bool { return v.Syncing() == "" && !v.Loading() })
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
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && v.section == sectionChanges })
	app.Type("id:commit-message", "amended")
	app.Press("id:amend")
	app.WaitFor("id:commit >> text:Amend Last Commit", time.Second)
	app.Press("id:commit")
	wait(func() bool { return v.Syncing() == "" && !v.Loading() })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); subject != "amended" {
		t.Fatalf("amended %q", subject)
	}
	git(t, repo, "remote", "add", "origin", bare)
	git(t, repo, "push", "-q", "-u", "origin", "main")
	v.Reload()
	wait(func() bool { return !v.Loading() && v.tracking.Upstream == "origin/main" })
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	git(t, repo, "add", "a.txt")
	v.Reload()
	wait(func() bool { return !v.Loading() && v.Syncing() == "" && len(areaFiles(v.current, AreaStaged)) == 1 })
	app.Type("id:commit-message", "pushed")
	app.Press("id:commit-and-push")
	wait(func() bool {
		local, _ := v.git.Run(repo, "rev-parse", "main")
		remote, _ := v.git.Run(bare, "rev-parse", "main")
		return v.Syncing() == "" && !v.Loading() && local == remote
	})
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); subject != "pushed" {
		t.Fatalf("committed %q", subject)
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
	v, app, wait := startView(t, Options{OnState: func(s State) { saved = s }})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && len(v.repo.commits) >= 2 })
	app.RightClick("id:ref-branches/feature")
	app.ChoosePopup("Pin Reference")
	app.WaitFor("id:group-pinned", time.Second)
	if len(saved.Pinned) != 1 {
		t.Fatalf("pinned %+v", saved.Pinned)
	}
	app.Press(commitRow("second"))
	app.PressWith(commitRow("init"), core.Modifiers{Platform: true})

	if keys := v.history.selection.Keys(); len(keys) != 2 {
		t.Fatalf("chosen %v", keys)
	}
	app.RightClick(commitRow("init"))
	app.ChoosePopup("Compare Selected Commits…")
	wait(func() bool { return v.compare.files.loaded && !v.compare.files.loading })
	if v.section != sectionCompare {
		t.Fatalf("section %v", v.section)
	}
	app.WaitFor(fileRow(AreaAll, "a.txt"), time.Second)
}

func TestViewReflogToggle(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "lost")
	lost, _ := vGit(repo, "rev-parse", "HEAD")
	git(t, repo, "reset", "-q", "--hard", "HEAD~1")
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() })
	app.RightClick("id:all-commits")
	app.ChoosePopup("Show Lost Commits (Reflog)")
	wait(func() bool { return v.state.ShowReflog && !v.Loading() && !v.repo.loading })
	found := false
	for _, c := range v.repo.commits {
		if c.Hash == lost {
			found = true
		}
	}
	if !found {
		t.Fatalf("reflog %+v", v.repo.commits)
	}
	app.WaitFor("text:All commits (Reflog)", time.Second)
}

func TestViewFileHistoryAndBlame(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && v.current.loaded && !v.current.loading })
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	app.ChoosePopup("File History…")
	wait(func() bool { return !v.fileHistory.loading })
	app.WaitFor("text:History of a.txt", time.Second)
	app.WaitFor("id:file-history >> text:second", time.Second)
	app.Press("id:file-history-dialog >> id:close")
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	app.ChoosePopup("Blame…")
	wait(func() bool { return !v.blame.loading })
	app.WaitFor("text:Blame of a.txt", time.Second)
	app.WaitFor("id:blame >> text:/^3$/", time.Second)
}

func TestViewSearchesHistory(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("needle\n"), 0644)
	git(t, repo, "add", "b.txt")
	git(t, repo, "commit", "-q", "-m", "third")
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() })
	app.Key("mod+shift+f")
	app.WaitFor("id:search-dialog", time.Second)
	app.Type("id:search-query", "needle")
	v.search.typ = SearchContent
	app.Press("id:search-dialog >> id:search")
	wait(func() bool { return !v.search.loading && v.search.commits != nil })
	app.WaitFor("id:search-results >> text:third", time.Second)
}

func TestViewBulkDeletesBranches(t *testing.T) {
	repo := testRepo(t)
	git(t, repo, "branch", "feature")
	git(t, repo, "branch", "other")
	git(t, repo, "switch", "-q", "-c", "draft")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "draft work")
	git(t, repo, "switch", "-q", "main")
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() })
	app.RightClick("id:group-branches")
	app.ChoosePopup("Delete Multiple Branches…")
	wait(func() bool { return !v.bulk.checking })
	app.Press("id:bulk-all")
	app.WaitFor("text:Unmerged", time.Second)
	if !maps.Equal(v.bulk.unmerged, map[string]bool{"draft": true}) {
		t.Fatalf("unmerged %v", v.bulk.unmerged)
	}
	// An unmerged branch goes only once the user chooses to lose its commits.
	if app.Enabled("id:bulk-dialog >> id:confirm") {
		t.Fatal("offered deleting an unmerged branch")
	}
	app.Press("label:Delete them anyway, losing those commits")
	app.Press("id:bulk-dialog >> id:confirm")
	wait(func() bool {
		out, _ := v.git.Run(repo, "branch", "--format=%(refname:short)")
		return v.Syncing() == "" && !v.Loading() && out == "main"
	})
}

func TestViewInteractiveRebaseDialog(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("2\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "second")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("3\n"), 0644)
	git(t, repo, "commit", "-q", "-am", "third")
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	wait(func() bool { return !v.Loading() && len(v.repo.commits) >= 3 && v.tracking.Branch == "main" })
	app.RightClick(commitRow("init"))
	app.ChoosePopup("Interactively Rebase 'main' to Here…")
	wait(func() bool { return !v.interactive.loading })
	app.WaitFor("text:Interactively Rebase main", time.Second)
	if !v.interactive.open || len(v.interactive.rows) != 2 {
		t.Fatalf("dialog %+v", v.interactive)
	}
	// A row's menu chooses its action, and a reworded one takes its
	// message below the list.
	app.RightClick("id:rebase-row-0")
	app.ChoosePopup("Reword")
	app.Clear("id:reword-message")
	app.Type("id:reword-message", "second reworded")
	app.RightClick("id:rebase-row-1")
	app.ChoosePopup("Drop")
	app.Press("id:interactive-rebase-dialog >> id:confirm")
	wait(func() bool {
		log, _ := v.git.Run(repo, "log", "--format=%s")
		return v.Syncing() == "" && !v.Loading() && log == "second reworded\ninit"
	})
}

// vGit runs git in dir and returns its trimmed output, for tests that only
// need one read.
func vGit(dir string, args ...string) (string, error) {
	cmd := Git{}
	return cmd.Run(dir, args...)
}
