package gitview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/diff"
	"github.com/nsgo-dev/nsgo/core"
)

func TestPickLinesAppliesChosenChanges(t *testing.T) {
	old := []byte("a\r\nb\r\nc\r\nd\r\ne\r\n")
	new := []byte("a\r\nB\r\nc\r\nd\r\nE\r\nf\r\n")
	all := func(diffSide, int) bool { return true }
	if got := pickLines(old, new, all); string(got) != string(new) {
		t.Fatalf("all: %q", got)
	}
	first := diff.Region{Old: []int{1}, New: []int{1}}
	if got := pickLines(old, new, func(side diffSide, line int) bool { return inRegion(first, side, line) }); string(got) != "a\r\nB\r\nc\r\nd\r\ne\r\n" {
		t.Fatalf("first hunk: %q", got)
	}
	// Taking only the added f keeps e, which its deletion would drop.
	added := func(side diffSide, line int) bool { return side == sideNew && line == 5 }
	if got := pickLines(old, new, added); string(got) != "a\r\nb\r\nc\r\nd\r\ne\r\nf\r\n" {
		t.Fatalf("added line: %q", got)
	}
}

func TestLineStatsCountsShortestEdit(t *testing.T) {
	for _, tc := range []struct {
		before, after  string
		added, deleted int
	}{
		{"", "a\nb\n", 2, 0},
		{"a\nb\n", "", 0, 2},
		{"a\nb\nc\n", "a\nB\nc\nd\n", 2, 1},
		{"x\ny\n", "x\ny\n", 0, 0},
	} {
		if added, deleted := lineStats([]byte(tc.before), []byte(tc.after)); added != tc.added || deleted != tc.deleted {
			t.Errorf("%q → %q: +%d −%d, want +%d −%d", tc.before, tc.after, added, deleted, tc.added, tc.deleted)
		}
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
	v, app, wait := startView(t, Options{Git: identity})
	v.SetPath(repo)
	p := v.current
	shown := func() bool { return p.loaded && !p.loading && !p.fileLoading && v.Syncing() == "" }
	wait(func() bool { return shown() && p.compared })
	if v.section != sectionChanges || p.selected != areaKeys[AreaUnstaged]+"a.txt" {
		t.Fatalf("selected %q", p.selected)
	}
	app.WaitFor("text:Staged Changes", time.Second)

	// The first change is staged alone; the file then shows in both areas.
	v.stageRegion(p, diff.Region{Old: []int{0}, New: []int{0}})
	wait(func() bool { return shown() && p.compared && len(p.changes.Files) == 3 })
	if index, _ := v.git.IndexFile(repo, "a.txt"); string(index) != "one\n2\n3\n4\n5\n6\n7\n8\n9\n10\n" {
		t.Fatalf("index: %q", index)
	}

	// Double-clicking the unstaged row stages the rest of the file, whose
	// selection follows it to the staged area.
	app.Press(fileRow(AreaUnstaged, "a.txt"))
	app.Press(fileRow(AreaUnstaged, "a.txt"))
	wait(func() bool { return shown() && len(p.changes.Files) == 2 })
	if p.selected != areaKeys[AreaStaged]+"a.txt" {
		t.Fatalf("selected %q", p.selected)
	}

	// Committing records the staged file and leaves the untracked one.
	app.Type("id:commit-message", "edit a")
	app.Press("id:commit")
	wait(func() bool { return shown() && len(p.changes.Files) == 1 })
	if subject, _ := v.git.Run(repo, "log", "-1", "--format=%s"); subject != "edit a" || v.messages[repo] != "" {
		t.Fatalf("commit %q, message %q", subject, v.messages[repo])
	}
	if f := p.changes.Files[0]; f.Path != "new.txt" || f.Area != AreaUnstaged {
		t.Fatalf("left: %+v", p.changes.Files)
	}
}

func TestViewStagesAndDiscardsChosenFiles(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0644)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("n\n"), 0644)
	os.WriteFile(filepath.Join(repo, "other.txt"), []byte("o\n"), 0644)
	v, app, wait := startView(t, Options{})
	v.SetPath(repo)
	p := v.current
	shown := func(files int) func() bool {
		return func() bool { return p.loaded && !p.loading && v.Syncing() == "" && len(p.changes.Files) == files }
	}
	wait(shown(3))
	areas := func() string {
		var s []string
		for _, f := range p.changes.Files {
			s = append(s, fmt.Sprintf("%d %s", f.Area, f.Path))
		}
		return strings.Join(s, ", ")
	}

	// The shortcut stages the chosen file; its menu offers the same.
	app.Press(fileRow(AreaUnstaged, "other.txt"))
	app.RightClick(fileRow(AreaUnstaged, "other.txt"))
	var menu []string
	for _, item := range app.PopupMenu {
		if item.Separator {
			menu = append(menu, "-")
		} else {
			menu = append(menu, item.Label)
		}
	}
	if got := strings.Join(menu, ","); got != "File History…,Blame…,-,Stage,Discard Changes…,Stage All Changes,-,Stash 1 File…,-,Copy File Paths,Copy Absolute Paths" {
		t.Fatalf("menu: %s", got)
	}
	app.ClosePopup()
	app.Key("mod+s")
	wait(func() bool { return shown(3)() && areas() == "1 other.txt, 2 a.txt, 2 new.txt" })

	// The staged list's menu unstages.
	app.RightClick(fileRow(AreaStaged, "other.txt"))
	app.ChoosePopup("Unstage")
	wait(func() bool { return shown(3)() && areas() == "2 a.txt, 2 new.txt, 2 other.txt" })

	// Discarding the chosen files restores the tracked one and deletes the
	// untracked one after a confirmation.
	app.Press(fileRow(AreaUnstaged, "a.txt"))
	app.PressWith(fileRow(AreaUnstaged, "new.txt"), core.Modifiers{Platform: true})
	app.RightClick(fileRow(AreaUnstaged, "a.txt"))
	app.ChoosePopup("Discard Changes…")
	app.WaitFor("text:Discard changes to 2 files?", time.Second)
	app.Press("id:discard-dialog >> id:confirm")
	wait(shown(1))
	if got := areas(); got != "2 other.txt" {
		t.Fatalf("left: %s", got)
	}
	if a, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(a) != "1\n" {
		t.Fatalf("a.txt: %q", a)
	}
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new.txt: %v", err)
	}
}
