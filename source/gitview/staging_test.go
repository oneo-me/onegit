package gitview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestPickLinesAppliesChosenChanges(t *testing.T) {
	old := []byte("a\r\nb\r\nc\r\nd\r\ne\r\n")
	new := []byte("a\r\nB\r\nc\r\nd\r\nE\r\nf\r\n")
	d := &editor.DiffState{Old: editor.NewBuffer(string(old)), New: editor.NewBuffer(string(new))}
	hunks := d.Hunks()
	if len(hunks) != 2 {
		t.Fatalf("hunks: %+v", hunks)
	}
	all := func(editor.DiffSide, int) bool { return true }
	if got := pickLines(old, new, hunks, all); string(got) != string(new) {
		t.Fatalf("all: %q", got)
	}
	first := func(side editor.DiffSide, line int) bool { return inHunk(hunks[0], side, line) }
	if got := pickLines(old, new, hunks, first); string(got) != "a\r\nB\r\nc\r\nd\r\ne\r\n" {
		t.Fatalf("first hunk: %q", got)
	}
	// Taking only the added f keeps e, which its deletion would drop.
	added := func(side editor.DiffSide, line int) bool { return side == editor.DiffNew && line == 5 }
	if got := pickLines(old, new, hunks, added); string(got) != "a\r\nb\r\nc\r\nd\r\ne\r\nf\r\n" {
		t.Fatalf("added line: %q", got)
	}
}

func TestWorktreeStatusStagesAndCommits(t *testing.T) {
	t.Parallel()
	g := Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	write := func(name, text string) { os.WriteFile(filepath.Join(repo, name), []byte(text), 0644) }
	describe := func() string {
		changes, err := g.WorktreeStatus(repo)
		if err != nil {
			t.Fatal(err)
		}
		var s []string
		for _, f := range changes.Files {
			before, after, err := g.FileVersions(changes, f)
			if err != nil {
				t.Fatal(err)
			}
			s = append(s, fmt.Sprintf("%d %c %s %q %q", f.Area, f.Status, f.Path, before, after))
		}
		return strings.Join(s, "\n")
	}

	// Before the first commit, staged files are additions.
	write("a.txt", "1\n")
	write("[b].txt", "b\n")
	if err := g.Stage(repo, FileChange{Path: "a.txt"}, FileChange{Path: "[b].txt"}); err != nil {
		t.Fatal(err)
	}
	if got := describe(); got != "1 A [b].txt \"\" \"b\\n\"\n1 A a.txt \"\" \"1\\n\"" {
		t.Fatalf("unborn:\n%s", got)
	}
	if err := g.Unstage(repo, FileChange{Path: "[b].txt"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Commit(repo, "init\n\n# comment\n"); err != nil {
		t.Fatal(err)
	}
	if subject, _ := g.Run(repo, "log", "-1", "--format=%B"); subject != "init" {
		t.Fatalf("message: %q", subject)
	}

	// Part of a change is staged as text, the rest stays unstaged.
	write("a.txt", "1\n2\n3\n")
	if err := g.StageText(repo, "a.txt", []byte("1\n2\n")); err != nil {
		t.Fatal(err)
	}
	want := "1 M a.txt \"1\\n\" \"1\\n2\\n\"\n2 A [b].txt \"\" \"b\\n\"\n2 M a.txt \"1\\n2\\n\" \"1\\n2\\n3\\n\""
	if got := describe(); got != want {
		t.Fatalf("partial:\n%s\nwant:\n%s", got, want)
	}
	if err := g.Unstage(repo, FileChange{Path: "a.txt"}); err != nil {
		t.Fatal(err)
	}
	if got := describe(); got != "2 A [b].txt \"\" \"b\\n\"\n2 M a.txt \"1\\n\" \"1\\n2\\n3\\n\"" {
		t.Fatalf("unstaged:\n%s", got)
	}
}

func TestViewStagesChangesAndCommits(t *testing.T) {
	repo := testRepo(t)
	lines := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte(lines), 0644)
	git(t, repo, "commit", "-q", "-am", "lines")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n2\n3\n4\n5\n6\n7\n8\n9\nten\n"), 0644)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("n\n"), 0644)
	v, pump := testView(t, Options{Git: Git{}.withEnv("GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")})
	v.SetPath(repo)
	p := v.current
	shown := func() bool { return p.loaded && !p.loading && !p.fileLoading && v.Syncing() == "" }
	pump(func() bool { return shown() && p.diff != nil })
	view := ui.NewTester(viewFrame(v), 1000, 700)
	if v.section != sectionChanges || !view.HasText("Changes") || view.HasText("Staged Changes") || p.selected != areaKeys[AreaUnstaged]+"a.txt" {
		t.Fatalf("selected %q: %v", p.selected, view.Texts())
	}

	// The first change is staged alone; the file then shows in both areas.
	v.stageHunk(p, p.diff.Hunks()[0])
	pump(func() bool { return shown() && p.diff != nil && len(p.changes.Files) == 3 })
	if index, _ := v.git.IndexFile(repo, "a.txt"); string(index) != "one\n2\n3\n4\n5\n6\n7\n8\n9\n10\n" {
		t.Fatalf("index: %q", index)
	}
	view.Frame()
	if !view.HasText("Staged Changes") {
		t.Fatalf("view: %v", view.Texts())
	}

	// Double-clicking the unstaged row stages the rest of the file, whose
	// selection follows it to the staged area.
	for range 2 {
		if err := view.Click("a.txt"); err != nil {
			t.Fatal(err)
		}
	}
	pump(func() bool { return shown() && len(p.changes.Files) == 2 })
	if p.selected != areaKeys[AreaStaged]+"a.txt" {
		t.Fatalf("selected %q", p.selected)
	}

	// Committing records the staged file and leaves the untracked one.
	v.messages[repo] = "edit a"
	view.Frame()
	if err := view.Click("Commit"); err != nil {
		t.Fatal(err)
	}
	pump(func() bool { return shown() && len(p.changes.Files) == 1 })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); subject != "edit a" || v.messages[repo] != "" {
		t.Fatalf("commit %q, message %q", subject, v.messages[repo])
	}
	if f := p.changes.Files[0]; f.Path != "new.txt" || f.Area != AreaUnstaged {
		t.Fatalf("left: %+v", p.changes.Files)
	}
}
