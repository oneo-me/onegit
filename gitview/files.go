package gitview

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/diff"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	uidiff "github.com/nsgo-dev/nsgo/ui/diff"
)

// maxDiffBytes bounds each side of a compared file; larger files show a
// notice instead of a diff.
const maxDiffBytes = 4 << 20

// filePane lists a set of files as a directory tree beside the chosen one:
// the diff of a changed file, or the contents of a commit's file when the
// pane browses. Git reads run off the engine.
type filePane struct {
	// browse shows files' contents rather than their changes.
	browse bool
	// key names what the files belong to, such as a worktree or commit;
	// another key starts the pane over.
	key string
	// generation discards results of reads that a newer one replaced, and
	// again repeats a read asked for while one ran.
	generation      uint64
	again           func(Git) (Changes, error)
	loading, loaded bool
	err             string
	changes         Changes
	// selected is the tree key of the chosen file (fileKey).
	selected string
	// tree arranges the files by directory, which lists shows by area:
	// AreaAll's lists a commit's or browsed files, and the others a
	// worktree's unstaged and staged ones apart. open holds the open
	// directories, and shown those already seen, which keep the expansion
	// the user chose, while new ones open unless the pane browses.
	tree  changeTree
	lists [3]fileList
	open  map[string]bool
	shown map[string]bool
	// file is the file shown, and before and after its versions the diff
	// compares, which staging its lines picks from; text holds a browsed
	// or conflicted file's contents and preview an image instead; note
	// replaces them for a file that cannot be shown.
	file           FileChange
	before, after  []byte
	text           *string
	compared       bool
	preview        *imagePreview
	path           string
	fileLoading    bool
	note           string
	fileGeneration uint64
	// version counts the versions shown, so the diff follows a reread.
	version int
}

// fileList is a tree of a pane's files. selection holds the rows of an
// area's files that its menus and shortcuts act on while the tree is built.
type fileList struct {
	selection *ui.Selection
	ref       *core.Ref
}

func (v *View) newFilePane(browse bool) *filePane {
	return &filePane{browse: browse, open: map[string]bool{}}
}

// roots are the top rows of area's list.
func (p *filePane) roots(area Area) []string {
	if area == AreaAll {
		return p.tree.roots
	}
	return p.tree.children(areaKeys[area])
}

// rowDir is key's path relative to the worktree when its row is a
// directory, and "" when it is a file.
func (p *filePane) rowDir(key string) string {
	n := p.tree.nodes[key]
	if n == nil || n.children == nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(key, areaKeys[keyArea(key)]), "/")
}

// keyArea is the area whose list shows the row of key.
func keyArea(key string) Area {
	for area, prefix := range areaKeys {
		if strings.HasPrefix(key, prefix) {
			return area
		}
	}
	return AreaAll
}

// clearFiles empties the pane and discards its pending reads; reason, when
// set, explains why it shows no files.
func (v *View) clearFiles(p *filePane, reason string) {
	p.generation++
	p.fileGeneration++
	p.again = nil
	p.loading, p.loaded, p.err, p.changes = false, reason != "", reason, Changes{}
	p.selected, p.text, p.compared, p.preview, p.path, p.fileLoading, p.note = "", nil, false, nil, "", false, ""
	p.file, p.before, p.after = FileChange{}, nil, nil
	p.shown = nil
	p.open = map[string]bool{}
	arrangeFiles(p)
}

// loadFiles reads the pane's files for key, then the selected file. The
// same key rereads in place, keeping the selection; another starts over.
func (v *View) loadFiles(p *filePane, key string, read func(Git) (Changes, error)) {
	defer v.invalidate()
	if p.key != key {
		v.clearFiles(p, "")
		p.key = key
	} else if p.loading {
		p.again = read
		return
	}
	p.again = nil
	p.generation++
	generation := p.generation
	p.loading = true
	git := v.git
	go func() {
		changes, err := read(git)
		v.dispatch(func() {
			if generation != p.generation || v.closed {
				return
			}
			p.loading, p.loaded = false, true
			if err != nil {
				p.err, p.changes = err.Error(), Changes{}
			} else {
				p.err, p.changes = "", changes
			}
			arrangeFiles(p)
			v.loadFile(p)
			if p == v.current && v.seekSection() {
				v.targetChanged()
			}
			if again := p.again; again != nil {
				v.loadFiles(p, key, again)
			}
		})
	}()
}

// arrangeFiles rebuilds the tree from the files and keeps the selected one,
// or chooses the first, revealing it in the tree.
func arrangeFiles(p *filePane) {
	p.tree = newChangeTree(p.changes.Files)
	shown := map[string]bool{}
	for _, key := range p.tree.directories() {
		if !p.shown[key] && !p.browse {
			p.open[key] = true
		}
		shown[key] = true
	}
	p.shown = shown
	for i := range p.lists {
		if s := p.lists[i].selection; s != nil {
			var kept []any
			for _, key := range s.Keys() {
				if p.tree.nodes[key.(string)] != nil {
					kept = append(kept, key)
				}
			}
			if len(kept) != s.Len() {
				s.Set(kept...)
			}
		}
	}
	selected := keptSelection(p.changes.Files, p.selected)
	if selected == p.selected && p.tree.nodes[selected] != nil {
		return
	}
	p.selected = selected
	for _, key := range p.tree.ancestors(selected) {
		p.open[key] = true
	}
	area := keyArea(selected)
	for i := range p.lists {
		if s := p.lists[i].selection; s != nil {
			if Area(i) == area && selected != "" {
				s.Set(selected)
			} else {
				s.Clear()
			}
		}
	}
}

// keptSelection keeps the selected file's key while it is still listed, or
// follows the file to its other area, as after staging it whole, and
// otherwise chooses the first file.
func keptSelection(files []FileChange, selected string) string {
	moved := ""
	for _, f := range files {
		key := fileKey(f)
		if key == selected {
			return selected
		}
		if moved == "" && f.Area != AreaAll && strings.TrimPrefix(selected, areaKeys[otherArea(f.Area)]) == f.Path {
			moved = key
		}
	}
	if moved != "" {
		return moved
	}
	if len(files) > 0 {
		return fileKey(files[0])
	}
	return ""
}

func otherArea(a Area) Area {
	if a == AreaStaged {
		return AreaUnstaged
	}
	return AreaStaged
}

func (v *View) selectFile(p *filePane, path string) {
	if p.selected == path {
		return
	}
	p.selected = path
	v.loadFile(p)
}

// loadFile reads the selected file's versions. Rereading the file that is
// already shown keeps its place.
func (v *View) loadFile(p *filePane) {
	defer v.invalidate()
	p.fileGeneration++
	var file FileChange
	found := false
	for _, f := range p.changes.Files {
		if fileKey(f) == p.selected {
			file, found = f, true
			break
		}
	}
	if !found {
		p.text, p.compared, p.preview, p.path, p.note, p.fileLoading = nil, false, nil, "", "", false
		p.file, p.before, p.after = FileChange{}, nil, nil
		return
	}
	if fileKey(p.file) != fileKey(file) || p.path == "" {
		p.text, p.compared, p.preview, p.path, p.note = nil, false, nil, file.Path, ""
		p.before, p.after = nil, nil
	}
	p.file = file
	generation, changes, browse := p.fileGeneration, p.changes, p.browse
	p.fileLoading = true
	git := v.git
	go func() {
		var before, after []byte
		var preview *imagePreview
		var err error
		if file.Status == 'U' {
			// A conflicted file has no single indexed version, so show its
			// worktree text with Git's conflict markers.
			after, err = git.WorktreeFile(changes.Top, file.Path)
		} else {
			before, after, err = git.FileVersions(changes, file)
			if err == nil {
				preview, err = imagePreviewFor(file, before, after)
			}
		}
		v.dispatch(func() {
			defer v.invalidate()
			if generation != p.fileGeneration || v.closed {
				return
			}
			p.fileLoading = false
			p.version++
			if file.Status == 'U' {
				p.preview, p.compared, p.before = nil, false, nil
				p.after = after
				if err != nil {
					p.note, p.text = err.Error(), nil
					return
				}
				p.note = ""
				text := string(after)
				p.text = &text
				return
			}
			note := ""
			switch {
			case errors.Is(err, errImageTooLarge):
				note = v.text("file_too_large")
			case err != nil:
				note = err.Error()
			case preview != nil:
				p.text, p.compared, p.preview, p.note = nil, false, preview, ""
				return
			case len(before) > maxDiffBytes || len(after) > maxDiffBytes:
				note = v.text("file_too_large")
			case isBinary(before) || isBinary(after):
				note = v.text("binary_file")
			}
			p.note, p.preview = note, nil
			if note != "" {
				p.text, p.compared = nil, false
				return
			}
			if browse {
				text := string(after)
				p.text, p.compared = &text, false
				return
			}
			p.before, p.after, p.text, p.compared = before, after, nil, true
		})
	}()
}

// filesView shows the pane's tree beside the selected file. empty explains a
// pane without files.
func (v *View) filesView(cx *core.Scope, p *filePane, empty string) core.Node {
	return ui.ResizablePanelGroup{Axis: base.Horizontal, Style: ui.FillRest,
		Panels: []base.ResizablePanel{
			{Size: func() core.Points { return core.Points(v.state.TreeWidth) }, MinSize: 160,
				OnResize: func(size core.Points) {
					v.state.TreeWidth = float32(size)
					v.saveState()
				},
				Style: core.Style{MinHeight: core.Pt(0)}, Children: []core.Node{v.fileTree(cx, p, empty)}},
			{MinSize: 240, Style: core.Style{MinHeight: core.Pt(0), MinWidth: core.Pt(0)}, Children: []core.Node{v.fileContent(cx, p)}},
		}}
}

// fileTree lists the pane's files. A worktree's unstaged files show above
// its staged ones, which a commit's message follows.
func (v *View) fileTree(cx *core.Scope, p *filePane, empty string) core.Node {
	split := core.NewMemo(cx, func() bool { v.track(); return p == v.current && p.loaded && p.err == "" })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		areas := split.Get()
		var node core.Node
		cx.Untrack(func() {
			if !areas {
				node = v.fileList(cx, p, AreaAll, empty)
				return
			}
			lists := ui.ResizablePanelGroup{Axis: base.Vertical, Style: ui.FillRest,
				OnResize: func(sizes []core.Points) {
					if len(sizes) == 2 && sizes[0]+sizes[1] > 0 {
						v.state.UnstagedShare = float32(100 * sizes[0] / (sizes[0] + sizes[1]))
						v.saveState()
					}
				},
				Panels: []base.ResizablePanel{
					{Size: func() core.Points { return core.Points(v.state.UnstagedShare) * 10 }, MinSize: areaMinHeight,
						Style: core.Style{MinWidth: core.Pt(0)}, Children: []core.Node{v.areaList(cx, p, AreaUnstaged)}},
					{Size: func() core.Points { return core.Points(100-v.state.UnstagedShare) * 10 }, MinSize: areaMinHeight,
						Style: core.Style{MinWidth: core.Pt(0)}, Children: []core.Node{v.areaList(cx, p, AreaStaged)}},
				}}
			node = core.View{Style: ui.FillRest, Children: []core.Node{lists, hairline(cx), v.commitBox(cx, p)}}
		})
		return node
	})
}

// fileList is the tree of area's files. empty explains a list of a
// commit's or browsed files without any.
func (v *View) fileList(cx *core.Scope, p *filePane, area Area, empty string) core.Node {
	l := &p.lists[area]
	l.selection = ui.NewSelection(cx, area != AreaAll)
	l.ref = &core.Ref{}
	cx.Cleanup(func() { l.selection, l.ref = nil, nil })
	if p.selected != "" && keyArea(p.selected) == area {
		l.selection.Set(p.selected)
	}
	if area != AreaAll {
		v.stagingKeys(cx, p, area)
	}
	message := core.NewMemo(cx, func() string {
		v.track()
		switch {
		case area != AreaAll:
		case !p.loaded:
			return v.text("loading_changes")
		case p.err != "":
			return p.err
		case len(p.changes.Files) == 0:
			return empty
		}
		return ""
	})
	tree := ui.Tree{Label: v.text("files"), Selection: l.selection, Ref: l.ref, TestID: fmt.Sprint("files-", area), EstimatedSize: 26,
		Roots: func() []any {
			v.track()
			var keys []any
			for _, key := range p.roots(area) {
				keys = append(keys, key)
			}
			return keys
		},
		Children: func(key any) []any {
			v.track()
			var keys []any
			for _, k := range p.tree.children(key.(string)) {
				keys = append(keys, k)
			}
			return keys
		},
		Expanded: func(key any) bool { v.track(); return p.open[key.(string)] },
		OnExpand: func(key any, open bool) {
			p.open[key.(string)] = open
			v.invalidate()
		},
		RowLabel: func(key any) string {
			v.track()
			if n := p.tree.nodes[key.(string)]; n != nil {
				return n.name
			}
			return ""
		},
		OnActivate: func(key any) { v.submitFile(p, area, key.(string)) },
		Row: func(cx *core.Scope, item ui.TreeItem) core.Node {
			return v.fileRow(cx, p, area, func() string { k, _ := item.Key().(string); return k })
		},
		Style: ui.FillRest}
	// Choosing a file shows it; the other area's list lets go of its rows.
	cx.Effect(func() {
		key, ok := l.selection.Cursor().(string)
		if !ok || !l.selection.Has(key) {
			return
		}
		cx.Untrack(func() {
			for j := range p.lists {
				if Area(j) != area && p.lists[j].selection != nil {
					p.lists[j].selection.Clear()
				}
			}
			if p.tree.children(key) == nil {
				v.selectFile(p, key)
			}
		})
	})
	return core.Dynamic(func(cx *core.Scope) core.Node {
		m := message.Get()
		if m != "" {
			return core.View{Style: core.Style{Padding: core.Symmetric(8, 18)}, Children: []core.Node{muted(cx, m, 13)}}
		}
		return tree
	})
}

// fileRow is a file's or directory's row in a pane's tree: a changed file
// with its status and changed lines, a browsed one with its icon.
func (v *View) fileRow(cx *core.Scope, p *filePane, area Area, key func() string) core.Node {
	colors := ui.ColorsOf(cx)
	node := func() *changeNode {
		v.track()
		if n := p.tree.nodes[key()]; n != nil {
			return n
		}
		return &changeNode{}
	}
	look := core.NewMemo(cx, func() string {
		n := node()
		return fmt.Sprint(n.name, "\x00", n.children != nil, "\x00", n.file.Status, n.file.Added, ",", n.file.Deleted, "\x00", n.file.OldPath)
	})
	content := core.Dynamic(func(cx *core.Scope) core.Node {
		look.Get()
		var out core.Node
		cx.Untrack(func() {
			n := node()
			if p.browse && n.children == nil {
				out = row(6, fileIcon(cx, n.name, 14), line(n.name, 13, ui.Regular, colors.Text))
				return
			}
			r := changeTreeRow(cx, n.name, n.file.Status, n.file.Added, n.file.Deleted, n.children != nil)
			if n.file.Status == 'R' {
				out = ui.Tooltip{Text: n.file.OldPath + " → " + n.file.Path, Trigger: r}
				return
			}
			out = r
		})
		return out
	})
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
		OnContextMenu: func(e core.ContextMenuEvent) {
			k := key()
			if area != AreaAll {
				v.popup(cx, e.PagePosition, v.filesMenu(cx, p, area, k))
			} else {
				v.popup(cx, e.PagePosition, v.allFilesMenu(cx, p, area, k))
			}
		},
		Children: []core.Node{content}}
}

// fileContent names the selected file above its diff or contents, or says
// why it cannot show one.
func (v *View) fileContent(cx *core.Scope, p *filePane) core.Node {
	state := core.NewMemo(cx, func() string {
		v.track()
		switch {
		case p.compared:
			return fmt.Sprint("diff:", fileKey(p.file))
		case p.text != nil:
			return fmt.Sprint("text:", fileKey(p.file), p.file.Status == 'U')
		case p.preview != nil:
			return fmt.Sprint("preview:", fileKey(p.file), p.version)
		case p.note != "":
			return "note:" + p.note
		case p.fileLoading || p.loading && !p.loaded:
			return "loading"
		case p.loaded && p.err == "" && len(p.changes.Files) > 0:
			return "choose"
		}
		return ""
	})
	header := core.Show{When: func() bool { v.track(); return p.path != "" }, Then: func(cx *core.Scope) core.Node { return v.fileHeader(cx, p) }}
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		s := state.Get()
		var node core.Node
		cx.Untrack(func() {
			switch {
			case strings.HasPrefix(s, "diff:"):
				node = v.diffView(cx, p)
			case strings.HasPrefix(s, "text:"):
				node = v.textView(cx, p)
			case strings.HasPrefix(s, "preview:"):
				node = v.imageComparison(cx, p.preview, !p.browse)
			case strings.HasPrefix(s, "note:"):
				node = centered(muted(cx, p.note, 13))
			case s == "loading":
				node = centered(spinner(24))
			case s == "choose":
				message := v.text("choose_changed_file")
				if p.browse {
					message = v.text("choose_file")
				}
				node = centered(muted(cx, message, 13))
			}
		})
		return node
	})
	return core.View{Style: ui.FillRest, Children: []core.Node{header, body}}
}

// diffView compares the shown file's versions, its changes and selected
// lines staged, unstaged or discarded when it is a worktree's.
func (v *View) diffView(cx *core.Scope, p *filePane) core.Node {
	ref := &diff.Ref{}
	view := uidiff.View{Ref: ref, Unified: true, Selectable: true, TestID: "diff", Language: p.file.Path,
		OldValue: func() string { v.track(); return string(p.before) },
		NewValue: func() string { v.track(); return string(p.after) },
		Style:    ui.FillRest}
	if v.state.DiffWrap {
		view.SoftWrap = true
	}
	if p.file.Area != AreaAll {
		view.RenderRegionActions = func(cx *core.Scope, r diff.Region) core.Node { return v.regionActions(cx, p, r) }
	}
	wrap := core.NewMemo(cx, func() bool { v.track(); return v.state.DiffWrap })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		view.SoftWrap = wrap.Get()
		return view
	})
}

// textView shows a browsed file's contents, or a conflicted file's with
// Git's markers under a hint.
func (v *View) textView(cx *core.Scope, p *filePane) core.Node {
	colors := ui.ColorsOf(cx)
	text := ""
	if p.text != nil {
		text = *p.text
	}
	view := uidiff.View{OldValue: func() string { return text }, NewValue: func() string { return text }, ExpandAll: true, Unified: true,
		Selectable: true, SoftWrap: v.state.DiffWrap, Language: p.file.Path, TestID: "file-text", Style: ui.FillRest}
	if p.file.Status != 'U' {
		return view
	}
	hint := core.View{Style: core.Style{Padding: core.Symmetric(4, 8), FlexShrink: 0},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Warn(), 0.08)} },
		Children:  []core.Node{muted(cx, v.text("conflict_hint"), 12)}}
	return core.View{Style: ui.FillRest, Children: []core.Node{hint, view}}
}

// fileHeader names the selected file, whose path can be selected and
// copied, and toggles line wrapping; the tree already counts its changed
// lines.
func (v *View) fileHeader(cx *core.Scope, p *filePane) core.Node {
	colors := ui.ColorsOf(cx)
	name := core.Text{ContentFunc: func() string { v.track(); return p.path }, MaxLines: 1, Selectable: core.SelectableOn,
		Style: core.Style{FontSize: 13, FontWeight: ui.Bold, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)}
	wrap := core.Show{When: func() bool { v.track(); return p.compared || p.text != nil }, Then: func(cx *core.Scope) core.Node {
		return v.iconButton(v.text("wrap_lines"), lucide.TextWrap, 28, func() bool { v.track(); return v.state.DiffWrap }, nil, func() {
			v.state.DiffWrap = !v.state.DiffWrap
			v.saveState()
			v.invalidate()
		}, "wrap-lines")
	}}
	bar := row(8, name, wrap)
	bar.Style.Height, bar.Style.FlexShrink, bar.Style.Padding = core.Pt(36), 0, core.Edges{Left: 12, Right: 8}
	return core.Group{bar, hairline(cx)}
}

// imageComparison shows an image's versions side by side, each scaled down
// to fit its half, or the one version an addition or deletion has; labels
// names each version.
func (v *View) imageComparison(cx *core.Scope, p *imagePreview, labels bool) core.Node {
	colors := ui.ColorsOf(cx)
	var sides []core.Node
	for _, side := range []struct {
		label string
		image previewImage
	}{{v.text("image_before"), p.before}, {v.text("image_after"), p.after}} {
		if side.image.data == nil && side.image.svg == nil {
			continue
		}
		var title core.Node = ui.Spacer{}
		if labels {
			t := line(side.label, 12, ui.Bold, colors.Text)
			t.Style.FlexGrow = 1
			title = t
		}
		size := line(fmt.Sprintf("%d × %d", side.image.width, side.image.height), 12, ui.Regular, colors.Muted)
		size.Style.FlexShrink = 0
		var picture core.Node
		if side.image.svg != nil {
			picture = core.Icon{SVG: side.image.svg, Label: side.label, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
		} else {
			picture = core.Image{Source: core.ImageSource{Data: side.image.data}, ResizeMode: core.Contain, Label: side.label,
				Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), MaxWidth: core.Pt(core.Points(side.image.width)), MaxHeight: core.Pt(core.Points(side.image.height))}}
		}
		frame := core.View{Style: ui.FillRest.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(8), CornerRadius: 8}),
			StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.04)} },
			Children:  []core.Node{picture}}
		sides = append(sides, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), MinHeight: core.Pt(0), Gap: 4},
			Children: []core.Node{row(8, title, size), frame}})
	}
	return core.View{Style: ui.FillRest.Merge(core.Style{Direction: core.Row, Gap: 12, Padding: core.All(12)}), Children: sides}
}

// chosenKeys lists the keys of the rows of area's list that a menu or
// shortcut acts on: the chosen rows when key is one of them or empty, and
// otherwise key's row.
func (p *filePane) chosenKeys(area Area, key string) []string {
	s := p.lists[area].selection
	if key != "" && (s == nil || !s.Has(key)) {
		return []string{key}
	}
	var keys []string
	if s != nil {
		for _, k := range s.Keys() {
			keys = append(keys, k.(string))
		}
	}
	if len(keys) == 0 && key != "" {
		keys = []string{key}
	}
	return keys
}

// chosenFiles lists the files of the rows of area's list that a menu or
// shortcut acts on, a directory's with it.
func (p *filePane) chosenFiles(area Area, key string) []FileChange {
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
	for _, key := range p.chosenKeys(area, key) {
		add(key)
	}
	slices.SortFunc(files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	return files
}
