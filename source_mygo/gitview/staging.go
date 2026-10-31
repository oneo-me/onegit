package gitview

import (
	"bytes"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/oneo-me/onevra/source/components"
)

// Git writes of the worktree's index and history.
const (
	opStage   = "stage"
	opDiscard = "discard"
	opCommit  = "commit"
)

// Heights of a worktree's file lists: the default share the unstaged list
// takes and each list's least height.
const (
	unstagedShare = 50
	areaMinHeight = 96
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
	if !v.CanStage() || len(files) == 0 {
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
	if !v.CanStage() || p.diff == nil || p.file.Area == AreaAll {
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

// discardHunk asks to discard the shown file's change i from its worktree,
// keeping the index and the file's other changes.
func (v *View) discardHunk(p *filePane, h editor.DiffHunk) {
	if !v.CanStage() || p.file.Area != AreaUnstaged || p.file.Status != 'M' || p.diff == nil {
		return
	}
	text := pickLines(p.before, p.after, p.diff.Hunks(), func(side editor.DiffSide, line int) bool {
		return !inHunk(h, side, line)
	})
	if bytes.Equal(text, p.after) {
		return
	}
	v.askDiscardChange(p.file.Path, text, v.text("discard_hunk"))
}

// discardSelection asks to discard the lines selected in the shown file's
// unstaged diff from its worktree, keeping the index and the other lines.
func (v *View) discardSelection(p *filePane) {
	r, ok := p.diff.SelectedLines()
	if !ok || !v.CanStage() || p.file.Area != AreaUnstaged || p.file.Status != 'M' {
		return
	}
	text := pickLines(p.before, p.after, p.diff.Hunks(), func(side editor.DiffSide, line int) bool {
		return !(side == r.Side && line >= r.Start && line <= r.End)
	})
	if bytes.Equal(text, p.after) {
		return
	}
	v.askDiscardChange(p.file.Path, text, v.text("discard_lines"))
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
			ui.Row(c).Gap(c.Theme().Space(1)).Children(func() {
				b := components.Button(c, label, components.ButtonSecondary).Height(d.Font.Size+8).
					Padding(0, c.Theme().Space(2)).FontSize(max(11, d.Font.Size-1)).Disabled(!v.CanStage())
				if b.Clicked() {
					v.stageHunk(p, hunk)
				}
				if p.file.Area == AreaUnstaged && p.file.Status == 'M' {
					discard := components.Button(c, v.text("discard_hunk"), components.ButtonSecondary).Height(d.Font.Size+8).
						Padding(0, c.Theme().Space(2)).FontSize(max(11, d.Font.Size-1)).Disabled(!v.CanStage())
					if discard.Clicked() {
						v.discardHunk(p, hunk)
					}
				}
			})
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
		ui.Row(c).Gap(c.Theme().Space(1)).Children(func() {
			b := components.Button(c, label, components.ButtonSecondary).Height(size).FontSize(12).Disabled(!v.CanStage())
			if b.Clicked() {
				v.stageSelection(p)
			}
			if p.file.Area == AreaUnstaged && p.file.Status == 'M' {
				discard := components.Button(c, v.text("discard_lines"), components.ButtonSecondary).Height(size).FontSize(12).Disabled(!v.CanStage())
				if discard.Clicked() {
					v.discardSelection(p)
				}
			}
		})
	}
}

// areaList is the list of an area's files under a header naming it with
// their count, whose menu stages or unstages all of them.
func (v *View) areaList(c *ui.Context, p *filePane, area Area) {
	t := c.Theme()
	files := areaFiles(p, area)
	label := v.text("unstaged_changes")
	if area == AreaStaged {
		label = v.text("staged_changes")
	}
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Gap(6).Padding(t.Space(2), 18, t.Space(1), 18).Children(func() {
			components.Text(c, label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12).Bold().TextColor(t.TextMuted)
			components.Text(c, strconv.Itoa(len(files))).Shrink(0).FontSize(11.5).TextColor(t.TextMuted)
		}).ContextMenu(func(m *ui.Menu) {
			action := v.text("stage_all")
			if area == AreaStaged {
				action = v.text("unstage_all")
			}
			if m.Item(action).Disabled(!v.CanStage() || len(files) == 0).Chosen() {
				v.stageFiles(area, files)
			}
			if area == AreaUnstaged && m.Item(v.text("discard_all")).Disabled(!v.CanStage() || len(files) == 0).Chosen() {
				v.askDiscard(files)
			}
		})
		v.fileList(c, p, area, "")
	})
}

// chosenFiles lists the files of the rows of area's list that a menu or
// shortcut acts on, a directory's with it: the chosen rows when key is one
// of them or empty, and otherwise key's row.
func (p *filePane) chosenFiles(area Area, key string) []FileChange {
	l := &p.lists[area]
	var keys []string
	if key == "" || l.chosen.Has(key) {
		for k := range l.chosen.All() {
			keys = append(keys, k.(string))
		}
	} else {
		keys = []string{key}
	}
	var files []FileChange
	seen := map[string]bool{}
	var add func(string)
	add = func(key string) {
		n := p.tree.nodes[key]
		switch {
		case n == nil:
		case n.children != nil:
			for _, child := range n.children {
				add(child)
			}
		case !seen[key]:
			seen[key] = true
			files = append(files, n.file)
		}
	}
	for _, key := range keys {
		add(key)
	}
	slices.SortFunc(files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	return files
}

// filesMenu opens, reveals, stages, unstages, discards or stashes from a
// row's menu the files of the chosen rows, or of the row alone when it is
// not chosen, and copies their paths. A directory row opens its folder.
func (v *View) filesMenu(c *ui.Context, row ui.Element, p *filePane, area Area, key string) {
	row.ContextMenu(func(m *ui.Menu) {
		files, dir := p.chosenFiles(area, key), p.rowDir(key)
		one := len(files) == 1 && dir == ""
		disabled := !v.CanStage() || len(files) == 0
		g := &menuGroups{m: m}
		v.fileActions(g, p, files, dir)
		var conflicts []FileChange
		for _, f := range files {
			if f.Status == 'U' {
				conflicts = append(conflicts, f)
			}
		}
		if len(conflicts) > 0 {
			g.group()
			sub := g.submenu(v.text("resolve_conflict"), func(m *ui.Menu) {
				if m.Item(v.text("resolve_ours")).Disabled(!v.CanStage()).Chosen() {
					v.resolveConflicts(conflicts, ConflictOurs)
				}
				if m.Item(v.text("resolve_theirs")).Disabled(!v.CanStage()).Chosen() {
					v.resolveConflicts(conflicts, ConflictTheirs)
				}
			})
			sub.Disabled(!v.CanStage())
		}
		g.group()
		if area == AreaStaged {
			if g.item(v.text("unstage_change")).Shortcut(v.shortcut()|ui.Shift, ui.KeyS).Disabled(disabled).Chosen() {
				v.stageFiles(area, files)
			}
			if one && g.item(v.text("unstage_all")).Disabled(disabled).Chosen() {
				v.stageFiles(area, areaFiles(p, area))
			}
		} else {
			if g.item(v.text("stage_change")).Shortcut(v.shortcut(), ui.KeyS).Disabled(disabled).Chosen() {
				v.stageFiles(area, files)
			}
			if g.item(v.text("discard_changes")).Shortcut(v.shortcut()|ui.Shift, ui.KeyD).Disabled(disabled).Chosen() {
				v.askDiscard(files)
			}
			if one && g.item(v.text("stage_all")).Disabled(disabled).Chosen() {
				v.stageFiles(area, areaFiles(p, area))
			}
		}
		g.group()
		if g.item(v.textf("stash_files", map[string]any{"Count": len(files)})).Disabled(disabled).Chosen() {
			v.openStashFiles(files)
		}
		if len(files) > 0 {
			g.group()
			v.copyFileActions(c, g, p, files)
		}
	})
}

// stagingKeys gives area's list the shortcuts of its rows' menus, acting
// on the chosen rows.
func (v *View) stagingKeys(list ui.Element, p *filePane, area Area) {
	on := func(mods ui.Modifiers, key ui.Key, act func([]FileChange)) {
		list.OnShortcut(mods, key, func() {
			if files := p.chosenFiles(area, ""); len(files) > 0 && v.CanStage() {
				act(files)
			}
		})
	}
	stage := func(files []FileChange) { v.stageFiles(area, files) }
	if area == AreaStaged {
		on(v.shortcut()|ui.Shift, ui.KeyS, stage)
		return
	}
	on(v.shortcut(), ui.KeyS, stage)
	on(v.shortcut()|ui.Shift, ui.KeyD, v.askDiscard)
}

// submitFile stages or unstages the file of the row i of area's list,
// which a double click or Enter activates.
func (v *View) submitFile(p *filePane, area Area, i int) {
	l := &p.lists[area]
	if p != v.current || area == AreaAll || i < 0 || i >= l.state.Rows() {
		return
	}
	if n := p.tree.nodes[l.state.Item(i)]; n != nil && n.children == nil {
		v.stageFiles(area, []FileChange{n.file})
	}
}

// resolveConflicts takes one side of the chosen conflicted files and stages
// them, marking the conflicts resolved.
func (v *View) resolveConflicts(files []FileChange, side ConflictSide) {
	if !v.CanStage() || len(files) == 0 {
		return
	}
	v.syncRepository("resolve", v.text("resolve_failed"), func(git Git, path string) error {
		top, err := git.Top(path)
		if err != nil {
			return err
		}
		for _, f := range files {
			if err := git.ResolveConflict(top, f.Path, side); err != nil {
				return err
			}
		}
		return nil
	})
}

// discarding confirms discarding the worktree's changes to files.
type discarding struct {
	open  bool
	files []FileChange
}

func (v *View) askDiscard(files []FileChange) {
	v.discarding = discarding{open: true, files: files}
}

// discardChange confirms discarding part of a file's unstaged change, such
// as one hunk or selected lines.
type discardChange struct {
	open bool
	path string
	text []byte
	what string
}

func (v *View) askDiscardChange(path string, text []byte, what string) {
	v.discardChange = discardChange{open: true, path: path, text: text, what: what}
	v.invalidate()
}

// discardChangeDialog asks before discarding part of a file, which cannot
// be undone.
func (v *View) discardChangeDialog(c *ui.Context) {
	d := &v.discardChange
	if !d.open {
		return
	}
	title := v.textf("discard_change_title", map[string]any{"What": d.what, "Path": d.path})
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Role(ui.RoleAlertDialog)
		components.Text(c, title).Bold().Wrap().FillWidth()
		components.Text(c, v.text("discard_change_message")).Wrap().FillWidth().TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).AutoFocus().Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("discard"), components.ButtonPrimary).Disabled(!v.CanStage()).Clicked() {
				d.open = false
				file, text := d.path, d.text
				v.syncRepository(opDiscard, v.text("discard_failed"), func(git Git, path string) error {
					top, err := git.Top(path)
					if err != nil {
						return err
					}
					return git.DiscardText(top, file, text)
				})
			}
		})
	})
}

// discardDialog asks before discarding changes, which cannot be undone.
func (v *View) discardDialog(c *ui.Context) {
	d := &v.discarding
	args := map[string]any{"Count": len(d.files)}
	if len(d.files) > 0 {
		args["Name"] = path.Base(d.files[0].Path)
	}
	title := v.textf("discard_title", args)
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Role(ui.RoleAlertDialog)
		components.Text(c, title).Bold().Wrap().FillWidth()
		components.Text(c, v.text("discard_message")).Wrap().FillWidth().TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).AutoFocus().Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("discard"), components.ButtonPrimary).Disabled(!v.CanStage()).Clicked() {
				d.open = false
				files := d.files
				v.syncRepository(opDiscard, v.text("discard_failed"), func(git Git, path string) error {
					top, err := git.Top(path)
					if err != nil {
						return err
					}
					return git.Discard(top, files...)
				})
			}
		})
	})
}

// commitBox takes the message of a commit of the staged changes, which
// ⌘Enter or its button records. It can amend the checked out commit instead,
// or commit and push in one action.
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
			v.scheduleDraftSave()
		}
		amended := v.amend
		components.Checkbox(c, &v.amend, v.text("amend_last_commit"))
		if v.amend != amended && v.amend && strings.TrimSpace(message) == "" {
			v.messages[v.path] = v.headMessage()
			v.scheduleDraftSave()
		}
		ready := (staged > 0 || v.amend) && strings.TrimSpace(message) != "" && v.CanSync()
		box.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEnter || k.Mods != v.shortcut() {
				return false
			}
			if (staged > 0 || v.amend) && strings.TrimSpace(k.Text()) != "" && v.CanSync() {
				v.messages[v.path] = k.Text()
				v.commit(false)
			}
			return true
		})
		label := v.text("commit")
		if v.amend {
			label = v.text("commit_amend")
		}
		ui.Row(c).Gap(t.Space(1)).Children(func() {
			b := components.Button(c, label, components.ButtonPrimary).Grow(1).Disabled(!ready)
			if b.Clicked() && ready {
				v.commit(false)
			}
			push := components.Button(c, v.text("commit_and_push"), components.ButtonSecondary).Shrink(0).Disabled(!ready || v.tracking.Branch == "")
			if push.Clicked() && ready && v.tracking.Branch != "" {
				v.commit(true)
			}
		})
	})
}

// headMessage is the checked out commit's message, which amending offers.
func (v *View) headMessage() string {
	i := commitIndex(v.repo.commits, v.repo.head)
	if i < 0 {
		return ""
	}
	commit := v.repo.commits[i]
	if commit.Body == "" {
		return commit.Subject
	}
	return commit.Subject + "\n\n" + commit.Body
}

// commit records the staged changes, or amends the checked out commit, with
// the worktree's message, which a successful commit clears. push then sends
// the branch to its upstream, or opens the push dialog when it has none.
func (v *View) commit(push bool) {
	path, message, amend := v.path, v.messages[v.path], v.amend
	v.write(opCommit, v.text("commit_failed"), func(git Git, path string) error {
		top, err := git.Top(path)
		if err != nil {
			return err
		}
		if amend {
			return git.AmendCommit(top, message)
		}
		return git.Commit(top, message)
	}, func() {
		v.amend = false
		if v.messages[path] == message {
			delete(v.messages, path)
		}
		v.saveDraft()
		if push {
			v.pushAfterCommit()
		}
	})
}

// pushAfterCommit sends the checked out branch to its upstream, or to the
// default remote when it has none, and opens the push dialog when there is
// no remote to send it to.
func (v *View) pushAfterCommit() {
	branch := v.tracking.Branch
	if branch == "" {
		return
	}
	remote, to := "", branch
	if name := refRemote(v.repo.github, v.tracking.Upstream); name != "" {
		remote, to = name, strings.TrimPrefix(v.tracking.Upstream, name+"/")
	} else if name, target, ok := strings.Cut(v.tracking.Upstream, "/"); ok {
		remote, to = name, target
	}
	if remote == "" {
		remote = v.defaultRemote()
	}
	if remote == "" {
		v.OpenPush()
		return
	}
	o := PushOptions{Branch: branch, Remote: remote, To: to, Tags: v.state.PushTags}
	v.Run(OpPush, v.text("push_failed"), func(git Git, path string) error { return git.PushBranch(path, o) })
}

// defaultRemote names the remote Commit and Push falls back to: origin,
// else the first remote by name.
func (v *View) defaultRemote() string {
	if _, ok := v.repo.remoteURLs["origin"]; ok {
		return "origin"
	}
	names := make([]string, 0, len(v.repo.remoteURLs))
	for name := range v.repo.remoteURLs {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) > 0 {
		return names[0]
	}
	return ""
}
