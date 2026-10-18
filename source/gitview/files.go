package gitview

import (
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// maxDiffBytes bounds each side of a compared file; larger files show a
// notice instead of a diff.
const maxDiffBytes = 4 << 20

// filePane lists a set of files as a directory tree beside the chosen one:
// the diff of a changed file, or the contents of a commit's file when the
// pane browses. Git reads run off the UI thread.
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
	// tree arranges the files by directory; cursor is its focused row.
	// shown lists the directories already seen, which keep the expansion
	// the user chose, while new ones open unless the pane browses.
	tree      changeTree
	treeState components.TreeVirtualizedState[string]
	cursor    int
	shown     map[string]bool
	// diff compares the selected file once read, view shows a browsed one
	// and preview shows an image instead; note replaces them for a file
	// that cannot be shown. file is the file shown and before and after
	// its versions the diff compares, which staging its lines picks from.
	diff           *editor.DiffState
	file           FileChange
	before, after  []byte
	view           *editor.State
	preview        *imagePreview
	path           string
	fileLoading    bool
	note           string
	fileGeneration uint64
}

func (v *View) newFilePane(browse bool) *filePane {
	p := &filePane{browse: browse}
	p.treeState.List.Selected = &p.cursor
	p.treeState.List.Label = func(i int) string { return p.tree.nodes[p.treeState.Item(i)].name }
	p.treeState.List.OnSubmit = func(i int) { v.submitFile(p, i) }
	p.treeState.List.OnChange = func() {
		if p.cursor >= 0 && p.cursor < p.treeState.Rows() {
			if key := p.treeState.Item(p.cursor); p.tree.children(key) == nil {
				v.selectFile(p, key)
			}
		}
	}
	return p
}

// clearFiles empties the pane and discards its pending reads; reason, when
// set, explains why it shows no files.
func (v *View) clearFiles(p *filePane, reason string) {
	p.generation++
	p.fileGeneration++
	p.again = nil
	p.loading, p.loaded, p.err, p.changes = false, reason != "", reason, Changes{}
	p.selected, p.diff, p.view, p.preview, p.path, p.fileLoading, p.note = "", nil, nil, nil, "", false, ""
	p.file, p.before, p.after = FileChange{}, nil, nil
	p.shown, p.cursor = nil, 0
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
			p.treeState.Open.Add(key)
		}
		shown[key] = true
	}
	p.shown = shown
	selected := keptSelection(p.changes.Files, p.selected)
	if selected == p.selected && p.tree.row(selected, p.treeState.Open.Has) >= 0 {
		// The tree keeps its cursor on the same row by key.
		return
	}
	p.selected = selected
	for _, key := range p.tree.ancestors(selected) {
		p.treeState.Open.Add(key)
	}
	p.cursor = p.tree.row(selected, p.treeState.Open.Has)
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
// already shown keeps its scroll position and expanded lines.
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
		p.diff, p.view, p.preview, p.path, p.note, p.fileLoading = nil, nil, nil, "", "", false
		p.file, p.before, p.after = FileChange{}, nil, nil
		return
	}
	if fileKey(p.file) != fileKey(file) || p.path == "" {
		p.diff, p.view, p.preview, p.path, p.note = nil, nil, nil, file.Path, ""
		p.before, p.after = nil, nil
	}
	p.file = file
	generation, changes, browse := p.fileGeneration, p.changes, p.browse
	p.fileLoading = true
	git := v.git
	go func() {
		before, after, err := git.FileVersions(changes, file)
		var preview *imagePreview
		if err == nil {
			preview, err = imagePreviewFor(file, before, after)
		}
		v.dispatch(func() {
			defer v.invalidate()
			if generation != p.fileGeneration || v.closed {
				return
			}
			p.fileLoading = false
			note := ""
			switch {
			case errors.Is(err, errImageTooLarge):
				note = v.text("file_too_large")
			case err != nil:
				note = err.Error()
			case preview != nil:
				p.diff, p.view, p.preview, p.note = nil, nil, preview, ""
				return
			case len(before) > maxDiffBytes || len(after) > maxDiffBytes:
				note = v.text("file_too_large")
			case isBinary(before) || isBinary(after):
				note = v.text("binary_file")
			}
			p.note, p.preview = note, nil
			if note != "" {
				p.diff, p.view = nil, nil
				return
			}
			if browse {
				if p.view == nil {
					buffer := editor.NewBuffer(string(after))
					buffer.SetLanguage(editor.LanguageFor(file.Path))
					p.view = &editor.State{Buffer: buffer, ReadOnly: true}
				} else if p.view.Buffer.String() != string(after) {
					p.view.Buffer.Reset(string(after))
				}
				return
			}
			p.before, p.after = before, after
			if p.diff == nil {
				lang := editor.LanguageFor(file.Path)
				old, cur := editor.NewBuffer(string(before)), editor.NewBuffer(string(after))
				old.SetLanguage(lang)
				cur.SetLanguage(lang)
				p.diff = &editor.DiffState{Old: old, New: cur, Layout: editor.DiffUnified}
			} else {
				if p.diff.Old.String() != string(before) {
					p.diff.Old.Reset(string(before))
				}
				if p.diff.New.String() != string(after) {
					p.diff.New.Reset(string(after))
				}
			}
		})
	}()
}

// filesView shows the pane's tree beside the selected file. empty explains a
// pane without files.
func (v *View) filesView(c *ui.Context, p *filePane, empty string) {
	split := components.Split(c, &v.state.TreeWidth, func() {
		v.fileTree(c, p, empty)
	}, func() {
		v.fileContent(c, p)
	}, components.SplitOptions{FirstMin: 160, SecondMin: 240, Default: treeWidth})
	split.Fill()
	if split.Changed() {
		v.saveState()
	}
}

// fileTree lists the pane's files, with a commit's message under the
// worktree's staged and unstaged ones.
func (v *View) fileTree(c *ui.Context, p *filePane, empty string) {
	staging := p == v.current
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		v.fileList(c, p, empty)
		if staging {
			v.commitBox(c, p)
		}
	})
}

func (v *View) fileList(c *ui.Context, p *filePane, empty string) {
	t := c.Theme()
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
		message := ""
		switch {
		case !p.loaded:
			message = v.text("loading_changes")
		case p.err != "":
			message = p.err
		case len(p.changes.Files) == 0:
			message = empty
		}
		if message != "" {
			components.Text(c, message).Wrap().FillWidth().TextColor(t.TextMuted).Padding(t.Space(2), 18)
			return
		}
		// The list pads its scrolling content, so the scroll view reaches the
		// pane's edges.
		components.TreeVirtualized(c, &p.treeState, p.tree.roots, p.tree.children, func(key string) {
			n := p.tree.nodes[key]
			if n.area != AreaAll {
				v.areaRow(c, p, n.area)
				return
			}
			if p.browse && n.children == nil {
				ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(6).Children(func() {
					fileIcon(c, n.name, 14)
					components.Text(c, n.name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13)
				})
				return
			}
			row := changeTreeRow(c, n.name, n.file.Status, n.file.Added, n.file.Deleted)
			if n.file.Status == 'R' {
				components.Tooltip(c, row, n.file.OldPath+" → "+n.file.Path)
			}
			if n.children == nil && n.file.Area != AreaAll {
				v.stageMenu(row, n.file)
			}
		}).Fill().Padding(t.Space(1), 8, 8, 8)
	})
}

// fileContent names the selected file above its diff or contents, or says
// why it cannot show one.
func (v *View) fileContent(c *ui.Context, p *filePane) {
	t := c.Theme()
	ui.Column(c).Fill().MinHeight(0).MinWidth(0).Children(func() {
		if p.path != "" {
			v.fileHeader(c, p)
		}
		switch {
		case p.diff != nil:
			// Code sits a step below body text, as in Onevra's Diff viewer
			// example, since the editor's default reads larger.
			p.diff.Font = ui.Font{Size: t.FontSize - 1}
			p.diff.Wrap = v.state.DiffWrap
			v.stagingDiff(p)
			editor.DiffView(c, p.diff).Grow(1).MinHeight(0).FillWidth()
			return
		case p.view != nil:
			p.view.Font = ui.Font{Size: t.FontSize - 1}
			p.view.SoftWrap = v.state.DiffWrap
			editor.View(c, p.view).Grow(1).MinHeight(0).FillWidth()
			return
		case p.preview != nil:
			v.imageComparison(c, p.preview, !p.browse)
			return
		}
		message := p.note
		switch {
		case message != "":
		case p.fileLoading || p.loading && !p.loaded:
			ui.Column(c).Fill().Center().Children(func() { components.Spinner(c, components.SpinnerRing) })
			return
		case p.loaded && p.err == "" && len(p.changes.Files) > 0:
			message = v.text("choose_changed_file")
			if p.browse {
				message = v.text("choose_file")
			}
		default:
			return
		}
		ui.Column(c).Fill().Center().Padding(t.Space(4)).Children(func() {
			components.Text(c, message).Wrap().TextColor(t.TextMuted)
		})
	})
}

// fileHeader names the selected file, whose path can be selected and
// copied, and toggles line wrapping; the tree already counts its changed
// lines.
func (v *View) fileHeader(c *ui.Context, p *filePane) {
	t := c.Theme()
	size := max(28, t.Rem(1.75))
	ui.Row(c).FillWidth().Shrink(0).Height(size+t.Space(2)).AlignItems(ui.Center).Gap(8).Padding(0, t.Space(2), 0, t.Space(3)).
		Children(func() {
			components.Text(c, p.path).Bold().FontSize(13).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Selectable()
			v.stagingActions(c, p, size)
			if p.diff != nil || p.view != nil {
				if iconButton(c, v.text("wrap_lines"), lucide.TextWrap, size, v.state.DiffWrap).Clicked() {
					v.state.DiffWrap = !v.state.DiffWrap
					v.saveState()
				}
			}
		})
	ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
}

// imageComparison shows an image's versions side by side, each scaled down
// to fit its half, or the one version an addition or deletion has; labels
// names each version.
func (v *View) imageComparison(c *ui.Context, p *imagePreview, labels bool) {
	t := c.Theme()
	ui.Row(c).Fill().MinHeight(0).MinWidth(0).Gap(t.Space(3)).Padding(t.Space(3)).Children(func() {
		for _, side := range []struct {
			label string
			image previewImage
		}{{v.text("image_before"), p.before}, {v.text("image_after"), p.after}} {
			if side.image.source == nil {
				continue
			}
			ui.Column(c).Grow(1).Basis(0).FillHeight().MinWidth(0).MinHeight(0).Gap(t.Space(1)).Children(func() {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					if labels {
						components.Text(c, side.label).Bold().FontSize(12).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					} else {
						ui.Spacer(c)
					}
					components.Text(c, fmt.Sprintf("%d × %d", side.image.width, side.image.height)).
						Shrink(0).FontSize(12).TextColor(t.TextMuted)
				})
				ui.Column(c).FillWidth().Grow(1).MinHeight(0).Center().Padding(t.Space(2)).Radius(8).
					Background(t.Text.Alpha(.04)).Children(func() {
					ui.Image(c, side.image.source).Fill().MinWidth(0).MinHeight(0).Fit(ui.ScaleDown).Label(side.label)
				})
			})
		}
	})
}
