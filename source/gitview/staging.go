package gitview

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/oneo-me/onevra/source/components"
)

// Git writes of the worktree's index and history.
const (
	opStage  = "stage"
	opCommit = "commit"
)

// pickLines returns old with the changes of hunks applied where take accepts
// their lines: a taken old line is deleted and a taken new line added, while
// the others stay as old has them. Lines are split on \n, as the diff
// viewer's buffers count them, so a \r stays with its line.
func pickLines(old, new []byte, hunks []editor.DiffHunk, take func(side editor.DiffSide, line int) bool) []byte {
	a, b := bytes.Split(old, []byte("\n")), bytes.Split(new, []byte("\n"))
	var out [][]byte
	at := 0
	for _, h := range hunks {
		out = append(out, a[at:h.OldStart]...)
		for i := h.OldStart; i < h.OldEnd; i++ {
			if !take(editor.DiffOld, i) {
				out = append(out, a[i])
			}
		}
		for i := h.NewStart; i < h.NewEnd; i++ {
			if take(editor.DiffNew, i) {
				out = append(out, b[i])
			}
		}
		at = h.OldEnd
	}
	out = append(out, a[at:]...)
	return bytes.Join(out, []byte("\n"))
}

// inHunk reports whether line of side belongs to h.
func inHunk(h editor.DiffHunk, side editor.DiffSide, line int) bool {
	if side == editor.DiffOld {
		return line >= h.OldStart && line < h.OldEnd
	}
	return line >= h.NewStart && line < h.NewEnd
}

// stagingText is the index version of the shown file after staging the
// lines take accepts from its unstaged diff, or unstaging them from its
// staged one.
func stagingText(p *filePane, take func(side editor.DiffSide, line int) bool) []byte {
	hunks := p.diff.Hunks()
	if p.file.Area == AreaStaged {
		// The index keeps the staged lines take leaves.
		return pickLines(p.before, p.after, hunks, func(side editor.DiffSide, line int) bool { return !take(side, line) })
	}
	return pickLines(p.before, p.after, hunks, take)
}

// stageFiles stages or unstages whole files, as their area chooses.
func (v *View) stageFiles(area Area, files []FileChange) {
	if !v.CanSync() || len(files) == 0 {
		return
	}
	failed := v.text("stage_failed")
	if area == AreaStaged {
		failed = v.text("unstage_failed")
	}
	v.syncRepository(opStage, failed, func(git Git, path string) error {
		top, err := git.Top(path)
		if err != nil {
			return err
		}
		if area == AreaStaged {
			return git.Unstage(top, files...)
		}
		return git.Stage(top, files...)
	})
}

// stageLines stages the lines take accepts from the shown file's unstaged
// diff, or unstages them from its staged one. A choice covering every
// change stages or unstages the whole file.
func (v *View) stageLines(p *filePane, take func(side editor.DiffSide, line int) bool) {
	if !v.CanSync() || p.diff == nil || p.file.Area == AreaAll {
		return
	}
	file, text := p.file, stagingText(p, take)
	p.diff.SetSelectedLines(editor.DiffLineRange{}, false)
	whole := p.after
	if file.Area == AreaStaged {
		whole = p.before
	}
	if bytes.Equal(text, whole) {
		v.stageFiles(file.Area, []FileChange{file})
		return
	}
	failed := v.text("stage_failed")
	if file.Area == AreaStaged {
		failed = v.text("unstage_failed")
	}
	v.syncRepository(opStage, failed, func(git Git, path string) error {
		top, err := git.Top(path)
		if err != nil {
			return err
		}
		return git.StageText(top, file.Path, text)
	})
}

// stageHunk stages or unstages the shown file's change i.
func (v *View) stageHunk(p *filePane, h editor.DiffHunk) {
	v.stageLines(p, func(side editor.DiffSide, line int) bool { return inHunk(h, side, line) })
}

// stageSelection stages or unstages the shown file's lines selected by
// their numbers.
func (v *View) stageSelection(p *filePane) {
	r, ok := p.diff.SelectedLines()
	if !ok {
		return
	}
	v.stageLines(p, func(side editor.DiffSide, line int) bool {
		return side == r.Side && line >= r.Start && line <= r.End
	})
}

// areaFiles lists the files of the pane's area.
func areaFiles(p *filePane, area Area) []FileChange {
	var files []FileChange
	for _, f := range p.changes.Files {
		if f.Area == area {
			files = append(files, f)
		}
	}
	return files
}

// stagingDiff lets the shown file's diff stage or unstage its changes and
// selected lines when it is a worktree's.
func (v *View) stagingDiff(p *filePane) {
	d := p.diff
	d.HunkActions, d.LineSelection = p.file.Area != AreaAll, p.file.Area != AreaAll
	if !d.HunkActions {
		d.RenderHunkActions = nil
		return
	}
	d.RenderHunkActions = func(c *ui.Context, bar ui.Element, a *editor.DiffHunkActions) {
		label := v.text("stage_change")
		if p.file.Area == AreaStaged {
			label = v.text("unstage_change")
		}
		hunk := a.Hunk
		bar.Children(func() {
			b := components.Button(c, label, components.ButtonSecondary).Height(d.Font.Size+8).
				Padding(0, c.Theme().Space(2)).FontSize(max(11, d.Font.Size-1)).Disabled(!v.CanSync())
			if b.Clicked() {
				v.stageHunk(p, hunk)
			}
		})
	}
}

// stagingActions is the shown file's header button staging or unstaging
// the lines selected by their numbers.
func (v *View) stagingActions(c *ui.Context, p *filePane, size float32) {
	if p.file.Area == AreaAll || p.path == "" {
		return
	}
	if p.diff == nil {
		return
	}
	if _, ok := p.diff.SelectedLines(); ok {
		label := v.text("stage_lines")
		if p.file.Area == AreaStaged {
			label = v.text("unstage_lines")
		}
		b := components.Button(c, label, components.ButtonSecondary).Height(size).FontSize(12).Disabled(!v.CanSync())
		if b.Clicked() {
			v.stageSelection(p)
		}
	}
}

// areaRow is the tree row grouping an area's files, with their count, whose
// menu stages or unstages all of them.
func (v *View) areaRow(c *ui.Context, p *filePane, area Area) {
	t := c.Theme()
	files := areaFiles(p, area)
	label, action := v.text("unstaged_changes"), v.text("stage_all")
	if area == AreaStaged {
		label, action = v.text("staged_changes"), v.text("unstage_all")
	}
	ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(6).Children(func() {
		components.Text(c, label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12).Bold().TextColor(t.TextMuted)
		components.Text(c, strconv.Itoa(len(files))).Shrink(0).FontSize(11.5).TextColor(t.TextMuted)
	}).ContextMenu(func(m *ui.Menu) {
		if m.Item(action).Disabled(!v.CanSync()).Chosen() {
			v.stageFiles(area, files)
		}
	})
}

// stageMenu stages or unstages a file from its tree row's menu.
func (v *View) stageMenu(row ui.Element, f FileChange) {
	label := v.text("stage_file")
	if f.Area == AreaStaged {
		label = v.text("unstage_file")
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(label).Disabled(!v.CanSync()).Chosen() {
			v.stageFiles(f.Area, []FileChange{f})
		}
	})
}

// submitFile stages or unstages the file of the worktree's tree row i,
// which a double click or Enter activates.
func (v *View) submitFile(p *filePane, i int) {
	if p != v.current || i < 0 || i >= p.treeState.Rows() {
		return
	}
	if n := p.tree.nodes[p.treeState.Item(i)]; n != nil && n.children == nil && n.file.Area != AreaAll {
		v.stageFiles(n.file.Area, []FileChange{n.file})
	}
}

// commitBox takes the message of a commit of the staged changes, which
// ⌘Enter or its button records.
func (v *View) commitBox(c *ui.Context, p *filePane) {
	t := c.Theme()
	if p.err != "" || !p.loaded {
		return
	}
	staged := len(areaFiles(p, AreaStaged))
	ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
	ui.Column(c).Key("commit").FillWidth().Shrink(0).Gap(t.Space(2)).Padding(t.Space(2)).Children(func() {
		message := v.messages[v.path]
		box := components.TextBox(c, &message, components.TextBoxOptions{Multiline: true}).
			FillWidth().Lines(2, 8).FontSize(13).Label(v.text("commit_message_label")).Placeholder(v.text("commit_placeholder"))
		if message != v.messages[v.path] {
			v.messages[v.path] = message
		}
		ready := staged > 0 && strings.TrimSpace(message) != "" && v.CanSync()
		box.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEnter || k.Mods != v.shortcut() {
				return false
			}
			if staged > 0 && strings.TrimSpace(k.Text()) != "" && v.CanSync() {
				v.messages[v.path] = k.Text()
				v.commit()
			}
			return true
		})
		b := components.Button(c, v.text("commit"), components.ButtonPrimary).FillWidth().Disabled(!ready)
		if b.Clicked() && ready {
			v.commit()
		}
	})
}

// commit records the staged changes with the worktree's message, which a
// successful commit clears.
func (v *View) commit() {
	path, message := v.path, v.messages[v.path]
	v.write(opCommit, v.text("commit_failed"), func(git Git, path string) error {
		top, err := git.Top(path)
		if err != nil {
			return err
		}
		return git.Commit(top, message)
	}, func() {
		if v.messages[path] == message {
			delete(v.messages, path)
		}
	})
}
