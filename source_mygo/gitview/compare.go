package gitview

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// compareState is the range shown in sectionCompare: two commits, or a
// commit and the worktree when to is empty.
type compareState struct {
	from, to string
	files    *filePane
}

// clearCompare forgets the comparison of another worktree.
func (v *View) clearCompare() {
	v.compare = compareState{files: v.compare.files}
	v.clearFiles(v.compare.files, "")
	v.compare.files.key = ""
}

// openCompare shows the files from and to differ in, or those from differs
// in from the worktree when to is empty.
func (v *View) openCompare(from, to string) {
	if from == "" || from == to {
		return
	}
	v.compare.from, v.compare.to = from, to
	v.section, v.ref, v.seekChanges = sectionCompare, "", false
	v.loadCompare()
	v.targetChanged()
	v.invalidate()
}

// loadCompare reads the compared range's files.
func (v *View) loadCompare() {
	if v.compare.from == "" {
		return
	}
	key := "compare:" + v.compare.from + ":" + v.compare.to
	from, to := v.compare.from, v.compare.to
	v.loadFiles(v.compare.files, key, func(git Git) (Changes, error) {
		return git.DiffBetween(v.path, from, to)
	})
}

// compareLabel names the comparison's two sides.
func (v *View) compareLabel() string {
	args := map[string]any{"From": v.commitLabel(v.compare.from), "To": v.text("working_tree")}
	if v.compare.to != "" {
		args["To"] = v.commitLabel(v.compare.to)
	}
	return v.textf("compare_title", args)
}

// compareView shows the compared range above its file tree and diff.
func (v *View) compareView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().MinHeight(0).MinWidth(0).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Gap(t.Space(2)).Padding(t.Space(1.5), t.Space(2)).
			Children(func() {
				components.Icon(c, lucide.GitCompare).Size(15, 15).Shrink(0).TextColor(t.Accent)
				components.Text(c, v.compareLabel()).FontSize(12.5).Bold().Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if components.Button(c, v.text("close_details"), components.ButtonGhost).Height(24).FontSize(12).
					Padding(0, t.Space(2)).Clicked() {
					v.section, v.ref = sectionCommits, ""
					v.targetChanged()
				}
			})
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
		ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
			v.filesView(c, v.compare.files, v.text("no_changes"))
		})
	})
}

// compareItems adds comparing a commit with the worktree, or the two
// chosen commits, to a commit's menu.
func (v *View) compareItems(g *menuGroups, commit Commit) {
	g.group()
	if g.item(v.text("compare_worktree")).Chosen() {
		v.openCompare(commit.Hash, "")
	}
	if selected := v.chosenCommits(commit); len(selected) == 2 {
		if g.item(v.text("compare_selected")).Chosen() {
			v.openCompare(selected[0].Hash, selected[1].Hash)
		}
	}
}

// chosenCommits returns the commits chosen in the history when the menu's
// commit is one of them, oldest first, else nil. Entries' rows are left out.
func (v *View) chosenCommits(menuCommit Commit) []Commit {
	var hashes []string
	for key := range v.history.chosen.All() {
		hash, ok := key.(string)
		if !ok {
			continue
		}
		if _, entry := entryID(hash); entry {
			continue
		}
		hashes = append(hashes, hash)
	}
	if len(hashes) < 2 {
		return nil
	}
	found := false
	var commits []Commit
	for _, hash := range hashes {
		if hash == menuCommit.Hash {
			found = true
		}
		if i := commitIndex(v.repo.commits, hash); i >= 0 {
			commits = append(commits, v.repo.commits[i])
		}
	}
	if !found {
		return nil
	}
	slices.SortStableFunc(commits, func(a, b Commit) int {
		return commitIndex(v.repo.commits, b.Hash) - commitIndex(v.repo.commits, a.Hash)
	})
	return commits
}
