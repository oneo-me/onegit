package gitview

import (
	"bytes"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/diff"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
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

// diffSide is a side of a file's diff: its old text or its new one.
type diffSide int

const (
	sideOld diffSide = iota
	sideNew
)

// pickLines returns old with the changes between old and new applied where
// take accepts their lines: a taken old line is deleted and a taken new
// line added, while the others stay as old has them. Lines are split on \n,
// as the diff counts them, so a \r stays with its line.
func pickLines(old, new []byte, take func(side diffSide, line int) bool) []byte {
	a, b := bytes.Split(old, []byte("\n")), bytes.Split(new, []byte("\n"))
	var out [][]byte
	at := 0
	for _, h := range lineHunks(a, b) {
		out = append(out, a[at:h.OldStart]...)
		for i := h.OldStart; i < h.OldEnd; i++ {
			if !take(sideOld, i) {
				out = append(out, a[i])
			}
		}
		for i := h.NewStart; i < h.NewEnd; i++ {
			if take(sideNew, i) {
				out = append(out, b[i])
			}
		}
		at = h.OldEnd
	}
	out = append(out, a[at:]...)
	return bytes.Join(out, []byte("\n"))
}

// inRegion reports whether line of side is one of r's.
func inRegion(r diff.Region, side diffSide, line int) bool {
	if side == sideOld {
		return slices.Contains(r.Old, line)
	}
	return slices.Contains(r.New, line)
}

// stagingText is the index version of the shown file after staging the
// lines take accepts from its unstaged diff, or unstaging them from its
// staged one.
func stagingText(p *filePane, take func(side diffSide, line int) bool) []byte {
	if p.file.Area == AreaStaged {
		// The index keeps the staged lines take leaves.
		return pickLines(p.before, p.after, func(side diffSide, line int) bool { return !take(side, line) })
	}
	return pickLines(p.before, p.after, take)
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
func (v *View) stageLines(p *filePane, take func(side diffSide, line int) bool) {
	if !v.CanStage() || !p.compared || p.file.Area == AreaAll {
		return
	}
	file, text := p.file, stagingText(p, take)
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

// stageRegion stages or unstages the lines of r.
func (v *View) stageRegion(p *filePane, r diff.Region) {
	v.stageLines(p, func(side diffSide, line int) bool { return inRegion(r, side, line) })
}

// discardRegion asks to discard the lines of r from the shown file's
// worktree, keeping the index and the file's other changes.
func (v *View) discardRegion(p *filePane, r diff.Region) {
	if !v.CanStage() || p.file.Area != AreaUnstaged || p.file.Status != 'M' || !p.compared {
		return
	}
	text := pickLines(p.before, p.after, func(side diffSide, line int) bool { return !inRegion(r, side, line) })
	if bytes.Equal(text, p.after) {
		return
	}
	what := v.text("discard_hunk")
	if r.Selected {
		what = v.text("discard_lines")
	}
	v.askDiscardChange(p.file.Path, text, what)
}

// regionActions are the buttons over a change or the selected lines of a
// worktree's file: staging or unstaging them, and discarding an unstaged
// change.
func (v *View) regionActions(cx *core.Scope, p *filePane, r diff.Region) core.Node {
	label := v.text("stage_change")
	if p.file.Area == AreaStaged {
		label = v.text("unstage_change")
	}
	buttons := []core.Node{regionAction(cx, label, v.is(func() bool { return !v.CanStage() }), func() { v.stageRegion(p, r) }, "stage-region")}
	if p.file.Area == AreaUnstaged && p.file.Status == 'M' {
		discard := v.text("discard_hunk")
		if r.Selected {
			discard = v.text("discard_lines")
		}
		buttons = append(buttons, regionAction(cx, discard, v.is(func() bool { return !v.CanStage() }), func() { v.discardRegion(p, r) }, "discard-region"))
	}
	return row(4, buttons...)
}

// regionAction is a button a line tall, solid over the lines under it.
func regionAction(cx *core.Scope, label string, disabled func() bool, press func(), testID string) core.Node {
	colors := ui.ColorsOf(cx)
	return base.Button{OnPress: func(core.PressEvent) { press() }, Label: label, Disabled: disabled, TestID: testID,
		Style: core.Style{Height: core.Pt(20), Padding: core.Symmetric(0, 8), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			CornerRadius: 5, BorderWidth: 1},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Raised(), BorderColor: colors.Border(), Hover: &core.Style{Background: colors.Hover()},
				Active: &core.Style{Background: colors.Pressed()}, Disabled: &core.Style{Opacity: core.Alpha(0.5)}}
		},
		Children: []core.Node{core.Text{Content: label, MaxLines: 1, Style: core.Style{FontSize: 11}, StyleFunc: ui.Colored(colors.Text)}}}
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

// areaList is the list of an area's files under a header naming it with
// their count, whose menu stages or unstages all of them.
func (v *View) areaList(cx *core.Scope, p *filePane, area Area) core.Node {
	colors := ui.ColorsOf(cx)
	label := v.text("unstaged_changes")
	if area == AreaStaged {
		label = v.text("staged_changes")
	}
	title := line(label, 12, ui.Bold, colors.Muted)
	title.Style.FlexGrow = 1
	count := core.Text{ContentFunc: func() string { v.track(); return strconv.Itoa(len(areaFiles(p, area))) }, MaxLines: 1,
		Style: core.Style{FontSize: 11.5, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Muted)}
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Padding: core.Edges{Top: 8, Right: 18, Bottom: 4, Left: 18}, FlexShrink: 0},
		TestID: "area-" + strconv.Itoa(int(area)),
		OnContextMenu: v.menuOn(cx, func() []core.MenuItem {
			files := areaFiles(p, area)
			g := &menuGroups{}
			disabled := !v.CanStage() || len(files) == 0
			if area == AreaStaged {
				g.item(v.text("unstage_all"), disabled, func() { v.stageFiles(area, files) })
			} else {
				g.item(v.text("stage_all"), disabled, func() { v.stageFiles(area, files) })
				g.item(v.text("discard_all"), disabled, func() { v.askDiscard(files) })
			}
			return g.items
		}),
		Children: []core.Node{title, count}}
	return core.View{Style: ui.FillRest, Children: []core.Node{header, v.fileList(cx, p, area, "")}}
}

// filesMenu opens, reveals, stages, unstages, discards or stashes from a
// row's menu the files of the chosen rows, or of the row alone when it is
// not chosen, and copies their paths. A directory row opens its folder.
func (v *View) filesMenu(cx *core.Scope, p *filePane, area Area, key string) []core.MenuItem {
	files, dir := p.chosenFiles(area, key), p.rowDir(key)
	one := len(files) == 1 && dir == ""
	disabled := !v.CanStage() || len(files) == 0
	g := &menuGroups{}
	v.fileActions(g, p, files, dir)
	var conflicts []FileChange
	for _, f := range files {
		if f.Status == 'U' {
			conflicts = append(conflicts, f)
		}
	}
	if len(conflicts) > 0 {
		g.group()
		g.submenu(v.text("resolve_conflict"), !v.CanStage(), []core.MenuItem{
			{Label: v.text("resolve_ours"), Disabled: !v.CanStage(), OnSelect: func() { v.resolveConflicts(conflicts, ConflictOurs) }},
			{Label: v.text("resolve_theirs"), Disabled: !v.CanStage(), OnSelect: func() { v.resolveConflicts(conflicts, ConflictTheirs) }},
		})
	}
	g.group()
	if area == AreaStaged {
		g.shortcut(v.text("unstage_change"), "mod+shift+s", disabled, func() { v.stageFiles(area, files) })
		if one {
			g.item(v.text("unstage_all"), disabled, func() { v.stageFiles(area, areaFiles(p, area)) })
		}
	} else {
		g.shortcut(v.text("stage_change"), "mod+s", disabled, func() { v.stageFiles(area, files) })
		g.shortcut(v.text("discard_changes"), "mod+shift+d", disabled, func() { v.askDiscard(files) })
		if one {
			g.item(v.text("stage_all"), disabled, func() { v.stageFiles(area, areaFiles(p, area)) })
		}
	}
	g.group()
	g.item(v.textf("stash_files", map[string]any{"Count": len(files)}), disabled, func() { v.openStashFiles(files) })
	if len(files) > 0 {
		g.group()
		v.copyFileActions(cx, g, p, files)
	}
	return g.items
}

// stagingKeys gives area's list the shortcuts of its rows' menus, acting
// on the chosen rows.
func (v *View) stagingKeys(cx *core.Scope, p *filePane, area Area) {
	on := func(key string, act func([]FileChange)) {
		cx.OnKeyWith(key, core.KeyOptions{When: core.KeyNotEditing}, func() {
			if s := p.lists[area].selection; s == nil || s.Len() == 0 {
				return
			}
			if files := p.chosenFiles(area, ""); len(files) > 0 && v.CanStage() {
				act(files)
			}
		})
	}
	stage := func(files []FileChange) { v.stageFiles(area, files) }
	if area == AreaStaged {
		on("mod+shift+s", stage)
		return
	}
	on("mod+s", stage)
	on("mod+shift+d", v.askDiscard)
}

// submitFile stages or unstages the file of key's row of area's list,
// which a double click or Return activates.
func (v *View) submitFile(p *filePane, area Area, key string) {
	if p != v.current || area == AreaAll {
		return
	}
	if n := p.tree.nodes[key]; n != nil && n.children == nil {
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
	v.invalidate()
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
func (v *View) discardChangeDialog(cx *core.Scope) core.Node {
	d := &v.discardChange
	run := func() {
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
	return v.dialogNode(cx, dialogSpec{open: &d.open, alert: true, testID: "discard-change-dialog",
		title:       func() string { return v.textf("discard_change_title", map[string]any{"What": d.what, "Path": d.path}) },
		description: func() string { return v.text("discard_change_message") },
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("discard"), v.CanStage, run, "confirm")}
		}})
}

// discardDialog asks before discarding changes, which cannot be undone.
func (v *View) discardDialog(cx *core.Scope) core.Node {
	d := &v.discarding
	run := func() {
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
	return v.dialogNode(cx, dialogSpec{open: &d.open, alert: true, testID: "discard-dialog",
		title: func() string {
			args := map[string]any{"Count": len(d.files)}
			if len(d.files) > 0 {
				args["Name"] = path.Base(d.files[0].Path)
			}
			return v.textf("discard_title", args)
		},
		description: func() string { return v.text("discard_message") },
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{v.cancelButton(&d.open), v.confirmButton(v.text("discard"), v.CanStage, run, "confirm")}
		}})
}

// commitBox takes the message of a commit of the staged changes, which
// ⌘Return or its button records. It can amend the checked out commit
// instead, or commit and push in one action.
func (v *View) commitBox(cx *core.Scope, p *filePane) core.Node {
	staged := func() int { return len(areaFiles(p, AreaStaged)) }
	ready := func() bool {
		return (staged() > 0 || v.amend) && strings.TrimSpace(v.messages[v.path]) != "" && v.CanSync()
	}
	message := ui.Input{Multiline: true, Label: v.text("commit_message_label"), Placeholder: v.text("commit_placeholder"), TestID: "commit-message",
		Value: func() string { v.watch(); return v.messages[v.path] },
		OnChange: func(s string) {
			v.messages[v.path] = s
			v.scheduleDraftSave()
			v.edited()
		},
		Style: core.Style{FontSize: 13, MinHeight: core.Pt(48), MaxHeight: core.Pt(160)}}
	cx.OnKeyWith("mod+enter", core.KeyOptions{When: core.KeyAlways}, func() {
		if ready() {
			v.commit(false)
		}
	})
	amend := ui.Checkbox{Text: v.text("amend_last_commit"), TestID: "amend", Checked: func() bool { v.track(); return v.amend }, OnChange: func(on bool) {
		v.amend = on
		if on && strings.TrimSpace(v.messages[v.path]) == "" {
			v.messages[v.path] = v.headMessage()
			v.scheduleDraftSave()
		}
		v.invalidate()
		v.edited()
	}}
	commit := ui.Button{Variant: ui.ButtonPrimary, Flex: 1, TestID: "commit", Disabled: v.is(func() bool { return !ready() }),
		Text: v.text("commit"), OnPress: func() {
			if ready() {
				v.commit(false)
			}
		}}
	label := core.NewMemo(cx, func() bool { v.track(); return v.amend })
	push := ui.Button{Variant: ui.ButtonSecondary, Text: v.text("commit_and_push"), TestID: "commit-and-push",
		Disabled: v.is(func() bool { return !ready() || v.tracking.Branch == "" }), OnPress: func() {
			if ready() && v.tracking.Branch != "" {
				v.commit(true)
			}
		}}
	buttons := core.Dynamic(func(cx *core.Scope) core.Node {
		b := commit
		if label.Get() {
			b.Text = v.text("commit_amend")
		}
		return row(4, b, push)
	})
	return core.View{Style: core.Style{Gap: 8, Padding: core.All(8), FlexShrink: 0}, Children: []core.Node{message, amend, buttons}}
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
		v.edited()
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
