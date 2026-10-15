package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func commitAll(t *testing.T, repo, message string) {
	t.Helper()
	testGit(t, "-C", repo, "add", "-A")
	testGit(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", message)
}

func TestParseStatus(t *testing.T) {
	snapshot, err := parseStatus("## main...origin/main [ahead 2, behind 1]\x00M  staged.txt\x00 M unstaged.txt\x00?? new dir/\x00R  renamed.txt\x00old.txt\x00")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Head.Branch != "main" || snapshot.Head.Upstream != "origin/main" || snapshot.Head.Ahead != 2 || snapshot.Head.Behind != 1 {
		t.Fatalf("head: %+v", snapshot.Head)
	}
	want := []changeEntry{
		{Index: "M", Work: " ", Path: "staged.txt"},
		{Index: " ", Work: "M", Path: "unstaged.txt"},
		{Index: "?", Work: "?", Path: "new dir/"},
		{Index: "R", Work: " ", Path: "renamed.txt", Orig: "old.txt"},
	}
	if len(snapshot.Changes) != len(want) {
		t.Fatalf("changes: %+v", snapshot.Changes)
	}
	for i := range want {
		if snapshot.Changes[i] != want[i] {
			t.Fatalf("change %d: %+v, want %+v", i, snapshot.Changes[i], want[i])
		}
	}
	if _, err := parseStatus("bad\x00"); err == nil {
		t.Fatal("short status entry accepted")
	}
}

func TestParseBranchHeader(t *testing.T) {
	for _, test := range []struct {
		header string
		want   headInfo
	}{
		{"main...origin/main", headInfo{Branch: "main", Upstream: "origin/main"}},
		{"main...origin/main [ahead 1, behind 2]", headInfo{Branch: "main", Upstream: "origin/main", Ahead: 1, Behind: 2}},
		{"feature", headInfo{Branch: "feature"}},
		{"No commits yet on main", headInfo{Branch: "main", Unborn: true}},
		{"HEAD (no branch)", headInfo{Detached: true}},
	} {
		if got := parseBranchHeader(test.header); got != test.want {
			t.Fatalf("%q: %+v, want %+v", test.header, got, test.want)
		}
	}
}

func TestReadStatusBranchesAndCommits(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	testGit(t, "init", "-b", "main", repo)
	if err := writeFile(filepath.Join(repo, "a.txt"), "one"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "first")
	if err := writeFile(filepath.Join(repo, "a.txt"), "two"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "second")
	testGit(t, "-C", repo, "branch", "feature")
	testGit(t, "-C", repo, "branch", "topic/nested")
	if err := writeFile(filepath.Join(repo, "b.txt"), "new"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	snapshot, err := readStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Head.Branch != "main" {
		t.Fatalf("branch: %+v", snapshot.Head)
	}
	if len(snapshot.Changes) != 1 || snapshot.Changes[0].Path != "b.txt" || snapshot.Changes[0].Index != "?" {
		t.Fatalf("changes: %+v", snapshot.Changes)
	}

	branches, err := readBranches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]branchEntry{}
	for _, branch := range branches {
		names[branch.Name] = branch
	}
	if !names["main"].Current || names["main"].Remote || names["feature"].Hash == "" {
		t.Fatalf("branches: %+v", branches)
	}
	if names["feature"].Current || names["feature"].Remote {
		t.Fatalf("feature: %+v", names["feature"])
	}
	if names["topic/nested"].Remote {
		t.Fatalf("local branch with a slash marked remote: %+v", names["topic/nested"])
	}

	commits, err := readCommits(ctx, repo, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].Subject != "second" || commits[1].Subject != "first" {
		t.Fatalf("commits: %+v", commits)
	}
	if len(commits[1].Parents) != 0 || commits[0].Parents[0] != commits[1].Hash {
		t.Fatalf("parents: %+v %+v", commits[0].Parents, commits[1].Parents)
	}
	if commits[0].Short == "" || commits[0].Hash == "" || commits[0].Date.IsZero() {
		t.Fatalf("commit fields: %+v", commits[0])
	}
}

func TestRepositorySectionsRender(t *testing.T) {
	s := &settings{systemLanguage: "zh-CN"}
	s.repo.status.loaded = true
	s.repo.status.value = statusSnapshot{
		Head:    headInfo{Branch: "main", Upstream: "origin/main", Ahead: 1},
		Changes: []changeEntry{{Index: "M", Work: " ", Path: "changed.txt"}},
	}
	s.repo.commits.loaded = true
	s.repo.commits.value = []commitEntry{{Hash: strings.Repeat("a", 40), Short: "aaaaaaa", Author: "Test", Subject: "initial commit"}}

	for _, test := range []struct {
		section string
		want    []string
	}{
		{sectionChanges, []string{"changed.txt"}},
		{sectionCommits, []string{"initial commit", "aaaaaaa"}},
	} {
		s.repo.section = test.section
		view := ui.NewTester(s.repositoryContent, 640, 480)
		view.Frame()
		for _, text := range test.want {
			if !view.HasText(text) {
				t.Fatalf("%s missing %q in %v", test.section, text, view.Texts())
			}
		}
	}
}

func TestBranchStatusLabel(t *testing.T) {
	s := &settings{systemLanguage: "zh-CN"}
	s.repo.status.loaded = true
	s.repo.status.value = statusSnapshot{Head: headInfo{Branch: "main", Upstream: "origin/main", Ahead: 2, Behind: 1}}
	if got := s.branchStatusLabel(); got != "main → origin/main ↑2 ↓1" {
		t.Fatalf("label: %q", got)
	}
	s.repo.status.value = statusSnapshot{Head: headInfo{Detached: true}}
	if got := s.branchStatusLabel(); got != "游离 HEAD" {
		t.Fatalf("detached label: %q", got)
	}
	s.repo.job.problem = "no configured remote"
	if got := s.branchStatusLabel(); got != "Git 操作失败：no configured remote" {
		t.Fatalf("job failure label: %q", got)
	}
}

func TestRunGitJobReportsFailure(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	testGit(t, "init", "-b", "main", repo)
	s := &settings{systemLanguage: "zh-CN"}
	s.overview.SelectedPath = repo
	pending := make(chan func(), 8)
	s.repo.dispatch = func(fn func()) { pending <- fn }
	s.runGitJob("正在获取…", "fetch", "missing-remote")
	select {
	case fn := <-pending:
		fn()
	case <-time.After(10 * time.Second):
		t.Fatal("git job did not finish")
	}
	if s.repo.job.running || s.repo.job.problem == "" {
		t.Fatalf("job result: %+v", s.repo.job)
	}
	if !strings.Contains(s.branchStatusLabel(), "Git 操作失败") {
		t.Fatalf("failure not shown: %q", s.branchStatusLabel())
	}
}

func TestSectionPersistence(t *testing.T) {
	s := &settings{statePath: filepath.Join(t.TempDir(), "state.yaml"), selectedProject: &projectNode{Path: "/repo", Repository: true}, systemLanguage: "zh-CN"}
	s.repo.section = sectionChanges
	s.setSection(sectionCommits)
	state, err := loadSidebarState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.SelectedSections["/repo"] != sectionCommits {
		t.Fatalf("section not persisted: %+v", state.SelectedSections)
	}
	if s.sectionFor(&projectNode{Path: "/repo", Repository: true}) != sectionCommits {
		t.Fatal("section not restored for the project")
	}
	if s.sectionFor(&projectNode{Path: "/other", Repository: true}) != sectionChanges {
		t.Fatal("new project must default to changes")
	}
}

func TestParseBranchesRemoteFlag(t *testing.T) {
	entries := parseBranches("refs/heads/feature/nested\x00feature/nested\x00abc\x00\x00\x00 \x00subject\n" +
		"refs/remotes/origin/HEAD\x00origin/HEAD\x00abc\x00\x00\x00 \x00HEAD subject\n" +
		"refs/remotes/origin/main\x00origin/main\x00abc\x00\x00\x00 \x00remote subject\n")
	if len(entries) != 2 {
		t.Fatalf("entries: %+v", entries)
	}
	if entries[0].Remote || entries[0].Name != "feature/nested" {
		t.Fatalf("local slash branch: %+v", entries[0])
	}
	if !entries[1].Remote || entries[1].Name != "origin/main" {
		t.Fatalf("remote branch: %+v", entries[1])
	}
}

func TestReadCommitsUnborn(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "empty")
	testGit(t, "init", "-b", "main", repo)
	commits, err := readCommits(context.Background(), repo, 10)
	if err != nil || len(commits) != 0 {
		t.Fatalf("unborn history: %+v, %v", commits, err)
	}
}

func TestReadCommitsIncludesAllBranches(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	testGit(t, "init", "-b", "main", repo)
	if err := writeFile(filepath.Join(repo, "a.txt"), "one"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "base")
	testGit(t, "-C", repo, "checkout", "-q", "-b", "feature")
	if err := writeFile(filepath.Join(repo, "b.txt"), "two"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "feature work")
	testGit(t, "-C", repo, "checkout", "-q", "main")
	if err := writeFile(filepath.Join(repo, "c.txt"), "three"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "main work")

	commits, err := readCommits(context.Background(), repo, 50)
	if err != nil {
		t.Fatal(err)
	}
	subjects := map[string]bool{}
	for _, entry := range commits {
		subjects[entry.Subject] = true
	}
	// The feature branch is not reachable from HEAD, so it only shows up when
	// the log covers every branch.
	for _, want := range []string{"base", "feature work", "main work"} {
		if !subjects[want] {
			t.Fatalf("missing %q in %+v", want, subjects)
		}
	}
}

func TestReadCommitsWithoutLimitReturnsCompleteHistory(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	testGit(t, "init", "-b", "main", repo)
	for i := 0; i < 5; i++ {
		if err := writeFile(filepath.Join(repo, "a.txt"), fmt.Sprintf("value %d", i)); err != nil {
			t.Fatal(err)
		}
		commitAll(t, repo, fmt.Sprintf("commit %d", i))
	}

	ctx := context.Background()
	all, err := readCommits(ctx, repo, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 || all[0].Subject != "commit 4" || all[4].Subject != "commit 0" {
		t.Fatalf("complete history: %+v", all)
	}

	limited, err := readCommits(ctx, repo, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 3 || limited[0].Subject != "commit 4" {
		t.Fatalf("limited history: %+v", limited)
	}
}

func TestRepositoryPageLayout(t *testing.T) {
	s := &settings{systemLanguage: "zh-CN", values: preferences{Language: "zh-Hans"}, selectedProject: &projectNode{Path: "/projects/demo", Repository: true}}
	s.repo.dispatch = func(func()) {}
	s.overview = projectOverview{
		Worktrees:     []worktree{{Path: "/projects/demo", Branch: "main", Main: true}, {Path: "/projects/demo-feature", Branch: "feature"}},
		SelectedPath:  "/projects/demo",
		selectedIndex: 0,
	}
	s.repo.status.loaded = true
	s.repo.status.value = statusSnapshot{Head: headInfo{Branch: "main"}}

	view := ui.NewTester(s.welcome, 1000, 700)
	view.Frame()
	for _, text := range []string{"demo", "demo-feature", "更改", "全部提交"} {
		if !view.HasText(text) {
			t.Fatalf("repository page missing %q: %v", text, view.Texts())
		}
	}
	view.Frame()
	if err := view.Click("全部提交"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if s.repo.section != sectionCommits {
		t.Fatalf("section not switched: %q", s.repo.section)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0600)
}

func TestReadSidebarRefsCommitHashes(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	testGit(t, "init", "-b", "main", repo)
	if err := writeFile(filepath.Join(repo, "a.txt"), "one"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "first")
	if err := writeFile(filepath.Join(repo, "a.txt"), "two"); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "second")
	testGit(t, "-C", repo, "tag", "light")
	testGit(t, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "tag", "-a", "annotated", "-m", "note")
	if err := writeFile(filepath.Join(repo, "a.txt"), "three"); err != nil {
		t.Fatal(err)
	}
	testGit(t, "-C", repo, "stash", "push")

	ctx := context.Background()
	refs, err := readSidebarRefs(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := readCommits(ctx, repo, 50)
	if err != nil {
		t.Fatal(err)
	}
	full := map[string]string{}
	for _, entry := range commits {
		full[entry.Subject] = entry.Hash
	}
	tags := map[string]string{}
	for _, tag := range refs.Tags {
		tags[tag.Name] = tag.Hash
	}
	if tags["light"] != full["second"] || tags["annotated"] != full["second"] {
		t.Fatalf("tag hashes: %+v, second=%s", tags, full["second"])
	}
	if len(refs.Stashes) != 1 || refs.Stashes[0].Hash == "" {
		t.Fatalf("stash hash: %+v", refs.Stashes)
	}
	if !strings.HasPrefix(refs.Stashes[0].Name, "stash@{0}") {
		t.Fatalf("stash name: %q", refs.Stashes[0].Name)
	}
	reachable := false
	for _, entry := range commits {
		if entry.Hash == refs.Stashes[0].Hash {
			reachable = true
		}
	}
	if !reachable {
		t.Fatalf("stash commit %s absent from the all-refs log", refs.Stashes[0].Hash)
	}
}

func TestSelectRefJumpsToCommit(t *testing.T) {
	s := &settings{systemLanguage: "zh-CN", selectedProject: &projectNode{Path: "/repo", Repository: true}}
	s.repo.section = sectionChanges
	s.repo.branches.loaded = true
	s.repo.branches.value = []branchEntry{
		{Name: "main", Hash: "aaaa"},
		{Name: "origin/main", Hash: "bbbb", Remote: true},
	}
	s.repo.refs.loaded = true
	s.repo.refs.value = sidebarRefs{Tags: []refEntry{{Name: "v1", Hash: "cccc"}}}
	s.repo.commits.loaded = true
	s.repo.commits.value = []commitEntry{
		{Hash: "cccc", Short: "ccccccc", Author: "Test", Subject: "tagged"},
		{Hash: "bbbb", Short: "bbbbbbb", Author: "Test", Subject: "remote"},
		{Hash: "aaaa", Short: "aaaaaaa", Author: "Test", Subject: "local"},
	}

	view := ui.NewTester(s.branchTree, 300, 600)
	view.Frame()
	if err := view.Click("main"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	if s.repo.selectedRef != (refSelection{Group: "branches", Name: "main"}) || s.repo.jumpHash != "aaaa" || s.repo.section != sectionCommits {
		t.Fatalf("branch selection: %+v jump=%q section=%q", s.repo.selectedRef, s.repo.jumpHash, s.repo.section)
	}
	// The content view consumes the pending jump and reveals the tip commit.
	content := ui.NewTester(s.repositoryContent, 640, 480)
	content.Frame()
	if s.repo.jumpHash != "" || s.repo.commitIndex != 2 {
		t.Fatalf("jump not applied: hash=%q index=%d", s.repo.jumpHash, s.repo.commitIndex)
	}

	// Tags default collapsed; opening the group lets its rows be selected too.
	open := true
	s.repo.treeGroups["tags"] = &open
	view.Frame()
	if err := view.Click("v1"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	content.Frame()
	if s.repo.selectedRef != (refSelection{Group: "tags", Name: "v1"}) || s.repo.commitIndex != 0 {
		t.Fatalf("tag selection: %+v index=%d", s.repo.selectedRef, s.repo.commitIndex)
	}
}
