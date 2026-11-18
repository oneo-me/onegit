package gitview

import (
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
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
func (v *View) compareView(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	label := core.Text{ContentFunc: func() string { v.track(); return v.compareLabel() }, MaxLines: 1,
		Style: core.Style{FontSize: 12.5, FontWeight: ui.Bold, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)}
	closeButton := ui.Button{Text: v.text("close_details"), Variant: ui.ButtonGhost, Size: ui.SizeSmall, TestID: "close-compare", OnPress: func() {
		v.section, v.ref = sectionCommits, ""
		v.targetChanged()
	}}
	header := row(8, ui.Icon(lucide.GitCompare, 15, colors.Accent), label, closeButton)
	header.Style.Padding, header.Style.FlexShrink = core.Symmetric(6, 8), 0
	return core.View{Style: ui.FillRest, Children: []core.Node{header, hairline(cx), v.filesView(cx, v.compare.files, v.text("no_changes"))}}
}

// compareItems adds comparing a commit with the worktree, or the two
// chosen commits, to a commit's menu.
func (v *View) compareItems(g *menuGroups, commit Commit) {
	g.group()
	g.item(v.text("compare_worktree"), false, func() { v.openCompare(commit.Hash, "") })
	if selected := v.chosenCommits(commit); len(selected) == 2 {
		g.item(v.text("compare_selected"), false, func() { v.openCompare(selected[0].Hash, selected[1].Hash) })
	}
}

// chosenCommits returns the commits chosen in the history when the menu's
// commit is one of them, oldest first, else nil.
func (v *View) chosenCommits(menuCommit Commit) []Commit {
	var hashes []string
	if s := v.history.selection; s != nil {
		for _, key := range s.Keys() {
			if hash, ok := key.(string); ok {
				hashes = append(hashes, hash)
			}
		}
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
