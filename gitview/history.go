package gitview

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base/commitgraph"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Tabs of the selected commit's details.
const (
	detailMessage = iota
	detailChanges
	detailFiles
)

// The open details float historyPanelMargin from the view's edges,
// historyPanelHeight tall until their tab row is dragged, keeping at least
// historyPanelMinHeight of themselves and historyListMinHeight of the list.
const (
	historyPanelHeight    = 460
	historyPanelMinHeight = 120
	historyListMinHeight  = 120
	historyPanelMargin    = 8
)

// historyState is the view's history: the commit list, its find bar and the
// selected commit's details.
type historyState struct {
	// finding shows the find bar for search; searched is the search matches
	// was built for, holding the matching commits' indexes, and match the
	// current one among them.
	finding          bool
	search, searched string
	matches          []int
	match            int
	// selected is the selected row's hash, and reveal a commit to select
	// once the history is read.
	selected, reveal string
	// open shows the selected commit's details floating over the list's
	// lower part, and height is the history's height, from which the list
	// keeps its last rows and scroll bar clear of them. panel is the height
	// their tab row was dragged to, zero for the default.
	open          bool
	detail        int
	height, panel core.Points
	// changes holds the selected commit's changed files and files all of
	// its files.
	changes, files *filePane
	// selection holds the chosen rows' hashes, graph the shown rows with
	// their merges collapsed or expanded, and list scrolls them, while the
	// history is built.
	selection *ui.Selection
	graph     *commitgraph.History
	list      *core.Ref
}

func (v *View) initHistory() {
	h := &v.history
	h.changes = v.newFilePane(false)
	h.files = v.newFilePane(true)
}

// clearHistory forgets the selection, search and details of another
// worktree.
func (v *View) clearHistory() {
	h := &v.history
	h.finding, h.search, h.searched, h.matches, h.match = false, "", "", nil, 0
	h.selected, h.reveal = "", ""
	if h.selection != nil {
		h.selection.Clear()
	}
	v.clearFiles(h.changes, "")
	v.clearFiles(h.files, "")
	h.changes.key, h.files.key = "", ""
}

// applyHistory searches newly read commits and keeps the selected row, or
// selects the one waiting to be revealed, else the newest commit.
func (v *View) applyHistory() {
	h := &v.history
	h.matches, h.searched = searchCommits(v.repo.commits, h.search), h.search
	h.match = max(0, slices.Index(h.matches, commitIndex(v.repo.commits, h.selected)))
	target := h.selected
	if h.reveal != "" {
		target, h.reveal = h.reveal, ""
	}
	if commitIndex(v.repo.commits, target) < 0 {
		target = ""
		if len(v.repo.commits) > 0 {
			target = v.repo.commits[0].Hash
		}
	}
	v.showCommit(target)
}

func commitIndex(commits []Commit, hash string) int {
	if hash == "" {
		return -1
	}
	for i, c := range commits {
		if c.Hash == hash {
			return i
		}
	}
	return -1
}

// graphCommits are the history's commits as the graph knows them.
func graphCommits(commits []Commit) []commitgraph.Commit {
	out := make([]commitgraph.Commit, len(commits))
	for i, c := range commits {
		out[i] = commitgraph.Commit{Hash: c.Hash, Parents: c.Parents}
	}
	return out
}

// graphColumn returns the graph column of the commit at index in the
// history, or -1 when it has no shown row.
func (v *View) graphColumn(index int) int {
	h := &v.history
	if h.graph == nil {
		return -1
	}
	for i := range core.Untracked(v.cx, h.graph.Len) {
		if item := core.Untracked(v.cx, func() commitgraph.Item { return h.graph.At(i) }); item.Index == index {
			return item.Row.Column
		}
	}
	return -1
}

// shownRow is the row of hash in the shown history, -1 when it has none.
func (v *View) shownRow(hash string) int {
	h := &v.history
	if h.graph == nil {
		return commitIndex(v.repo.commits, hash)
	}
	for i := range core.Untracked(v.cx, h.graph.Len) {
		if core.Untracked(v.cx, func() commitgraph.Item { return h.graph.At(i) }).Hash == hash {
			return i
		}
	}
	return -1
}

// expandTo expands the collapsed merges that hide hash.
func (v *View) expandTo(hash string) {
	h := &v.history
	if h.graph == nil {
		return
	}
	commits := graphCommits(v.repo.commits)
	for range len(commits) {
		collapsed := map[string]bool{}
		for _, c := range commits {
			if len(c.Parents) > 1 && core.Untracked(v.cx, func() bool { return h.graph.Collapsed(c.Hash) }) {
				collapsed[c.Hash] = true
			}
		}
		merge := commitgraph.Fold(commits, collapsed)[hash]
		if merge == "" {
			return
		}
		h.graph.Toggle(merge)
	}
}

// findCommits searches the history again after the search changed and
// selects the first match from the selected commit on, wrapping around.
func (v *View) findCommits() {
	h := &v.history
	if h.searched == h.search {
		return
	}
	h.matches, h.searched, h.match = searchCommits(v.repo.commits, h.search), h.search, 0
	if len(h.matches) == 0 {
		v.invalidate()
		return
	}
	from := max(0, commitIndex(v.repo.commits, h.selected))
	for k, i := range h.matches {
		if i >= from {
			h.match = k
			break
		}
	}
	v.showMatch()
}

// showMatch selects the current match.
func (v *View) showMatch() {
	h := &v.history
	if h.match >= 0 && h.match < len(h.matches) {
		v.ref = ""
		v.showCommit(v.repo.commits[h.matches[h.match]].Hash)
	}
}

// stepMatch moves to the next match, or the previous one when back is set,
// wrapping around.
func (v *View) stepMatch(back bool) {
	h := &v.history
	v.findCommits()
	if len(h.matches) == 0 {
		return
	}
	if back {
		h.match = (h.match + len(h.matches) - 1) % len(h.matches)
	} else {
		h.match = (h.match + 1) % len(h.matches)
	}
	v.showMatch()
}

// searchCommits returns the indexes of commits whose subject, message,
// author, co-author, hash or refs contain query, ignoring case, or nil for
// an empty query.
func searchCommits(commits []Commit, query string) []int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	matches := []int{}
	for i, c := range commits {
		if strings.HasPrefix(c.Hash, query) || strings.Contains(strings.ToLower(c.Subject), query) ||
			strings.Contains(strings.ToLower(c.Body), query) || strings.Contains(strings.ToLower(c.Author), query) ||
			strings.Contains(strings.ToLower(c.AuthorEmail), query) ||
			slices.ContainsFunc(c.CoAuthors, func(p Person) bool {
				return strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(strings.ToLower(p.Email), query)
			}) ||
			strings.Contains(strings.ToLower(strings.Join(c.Refs, "\n")), query) {
			matches = append(matches, i)
		}
	}
	return matches
}

// revealCommit shows the history with the commit selected and scrolled into
// view. A commit outside the history read so far is selected once it is
// read again.
func (v *View) revealCommit(hash string) {
	h := &v.history
	if v.section != sectionCommits {
		v.section = sectionCommits
		v.targetChanged()
	}
	if commitIndex(v.repo.commits, hash) < 0 {
		h.reveal = hash
		v.loadRepository()
		return
	}
	v.showCommit(hash)
}

// showCommit selects a commit and scrolls the list to it, expanding the
// merges that fold it.
func (v *View) showCommit(hash string) {
	h := &v.history
	v.expandTo(hash)
	if h.selection != nil {
		if hash == "" {
			h.selection.Clear()
		} else {
			h.selection.Set(hash)
		}
	}
	if i := v.shownRow(hash); i >= 0 && h.list != nil {
		h.list.ScrollToIndexAt(i, 0.5, 0, false)
	}
	v.selectCommit(hash)
}

// selectCommit shows a row's details, reading the files its tab needs, and
// opens them when another row is chosen.
func (v *View) selectCommit(hash string) {
	h := &v.history
	if hash != h.selected {
		h.open = hash != ""
	}
	h.selected = hash
	defer v.invalidate()
	if hash == "" {
		v.clearFiles(h.changes, "")
		v.clearFiles(h.files, "")
		h.changes.key, h.files.key = "", ""
		return
	}
	path := v.path
	switch h.detail {
	case detailChanges:
		if h.changes.key != hash {
			v.loadFiles(h.changes, hash, func(git Git) (Changes, error) { return git.CommitChanges(path, hash) })
		}
	case detailFiles:
		if h.files.key != hash {
			v.loadFiles(h.files, hash, func(git Git) (Changes, error) { return git.CommitFiles(path, hash) })
		}
	}
}

// activateCommit opens the commit's details and asks to check it out, as a
// double click or Return on its row does.
func (v *View) activateCommit(commit Commit) {
	v.ref = ""
	v.selectCommit(commit.Hash)
	v.history.open = true
	v.openCheckout(commitTarget(commit))
}

// panelHeight is the open details' height, within the history's limits
// once its height is known.
func (h *historyState) panelHeight() core.Points {
	height := h.panel
	if height <= 0 {
		height = historyPanelHeight
	}
	if h.height > 0 {
		height = min(height, h.height-historyPanelMargin-historyListMinHeight)
	}
	return max(historyPanelMinHeight, height)
}

// historyView shows the commit list, with the find bar above it while
// open, under the selected commit's details floating in a glass panel.
func (v *View) historyView(cx *core.Scope) core.Node {
	h := &v.history
	h.selection = ui.NewSelection(cx, true)
	h.list = &core.Ref{}
	h.graph = commitgraph.NewHistory(cx, func() []commitgraph.Commit {
		v.track()
		return graphCommits(v.repo.commits)
	}, h.selection)
	cx.Cleanup(func() { h.selection, h.graph, h.list = nil, nil, nil })
	if h.selected != "" {
		h.selection.Set(h.selected)
	}
	cx.OnKey("mod+f", func() {
		h.finding = true
		v.invalidate()
	})
	// Escape hides the details unless the find bar took it.
	cx.OnKey("escape", func() {
		if h.open {
			h.open = false
			v.invalidate()
		}
	})
	state := core.NewMemo(cx, func() string {
		v.track()
		switch {
		case v.repo.err != "":
			return "error:" + v.repo.err
		case !v.repo.loaded:
			return "loading"
		case len(v.repo.commits) == 0:
			return "empty"
		}
		return "list"
	})
	return core.Dynamic(func(cx *core.Scope) core.Node {
		s := state.Get()
		var node core.Node
		cx.Untrack(func() {
			switch {
			case s == "loading":
				node = centered(spinner(24))
			case s == "empty":
				node = centered(muted(cx, v.text("no_commits"), 13))
			case strings.HasPrefix(s, "error:"):
				node = centered(muted(cx, v.repo.err, 13))
			default:
				node = v.historyList(cx)
			}
		})
		return node
	})
}

// historyList is the find bar over the list and the details floating over
// its lower part. The list keeps its place as the details open and close.
func (v *View) historyList(cx *core.Scope) core.Node {
	h := &v.history
	colors := ui.ColorsOf(cx)
	list := ui.List{Ref: h.list, Label: v.text("all_commits"), TestID: "history", Selection: h.selection, EstimatedSize: commitgraph.RowHeight,
		Count: h.graph.Len,
		Key:   func(i int) any { return h.graph.At(i).Hash },
		RowLabel: func(i int) string {
			v.track()
			if c, ok := v.commitAt(h.graph.At(i).Index); ok {
				return c.Subject
			}
			return ""
		},
		OnSelectionChange: func(s *ui.Selection) {
			if hash, ok := s.Cursor().(string); ok && s.Has(hash) {
				v.ref = ""
				v.selectCommit(hash)
			}
		},
		OnActivate: func(i int) {
			if c, ok := v.commitAt(core.Untracked(cx, func() int { return h.graph.At(i).Index })); ok {
				v.activateCommit(c)
			}
		},
		RowStyle: core.Style{Height: core.Pt(commitgraph.RowHeight), MinHeight: core.Pt(commitgraph.RowHeight), Padding: core.Symmetric(0, 4), Gap: 0},
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			return v.commitRow(cx, func() commitgraph.Item { return h.graph.At(row.Index()) })
		},
		Style: ui.FillRest,
		ListStyleFunc: func() core.Style {
			v.track()
			bottom := core.Points(8)
			if h.open {
				bottom += h.panelHeight() + historyPanelMargin
			}
			return core.Style{Padding: core.Edges{Top: 6, Right: 8, Bottom: bottom, Left: 8}}
		}}
	details := core.Show{When: func() bool { v.track(); return h.open }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "commit-details", Style: core.Style{Position: core.Absolute, Left: core.Pt(historyPanelMargin), Right: core.Pt(historyPanelMargin),
			Bottom: core.Pt(historyPanelMargin), MinHeight: core.Pt(0), CornerRadius: 12, BorderWidth: 1, Overflow: core.Hidden},
			StyleFunc: func() core.Style {
				v.track()
				shadow := ui.Black(0.08)
				if cx.Dark() {
					shadow = ui.Black(0.24)
				}
				// Liquid Glass where the system has it, else a frosted blur,
				// tinted with the surface.
				return core.Style{Height: core.Pt(h.panelHeight()), BorderColor: colors.Border(), Background: ui.Alpha(colors.Surface(), 0.7),
					GlassEffect: core.GlassRegular, BackdropFilters: []core.Filter{core.Blur(24)}, Shadows: []core.BoxShadow{ui.Shadow(0, 2, 8, 0, shadow)}}
			},
			Children: []core.Node{v.commitDetails(cx)}}
	}}
	return core.View{Style: ui.FillRest, Children: []core.Node{
		v.findBar(cx),
		core.View{Style: ui.FillRest, OnLayout: func(b core.Bounds) {
			if h.height != b.Size.Height {
				h.height = b.Size.Height
				v.invalidate()
			}
		}, Children: []core.Node{list, details}},
	}}
}

// commitAt is the history's commit at index, as the view holds it now.
func (v *View) commitAt(index int) (Commit, bool) {
	if index < 0 || index >= len(v.repo.commits) {
		return Commit{}, false
	}
	return v.repo.commits[index], true
}

// findBar finds commits in the history by their subject, message, author,
// hash or refs while it shows, Return and its arrows moving through the
// matches.
func (v *View) findBar(cx *core.Scope) core.Node {
	h := &v.history
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { v.track(); return h.finding }, Then: func(cx *core.Scope) core.Node {
		count := func() string {
			v.track()
			if strings.TrimSpace(h.search) == "" {
				return ""
			}
			if len(h.matches) == 0 {
				return "0/0"
			}
			return strconv.Itoa(h.match+1) + "/" + strconv.Itoa(len(h.matches))
		}
		closeBar := func() {
			h.finding = false
			v.invalidate()
		}
		cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, closeBar)
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Padding: core.All(6), FlexShrink: 0}, Children: []core.Node{
			ui.Input{Value: func() string { v.track(); return h.search }, AutoFocus: true, Label: v.text("search_history"), Placeholder: v.text("search_history_placeholder"),
				TestID: "find", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
				OnChange: func(s string) {
					h.search = s
					v.findCommits()
				},
				OnSubmit: func(string) { v.stepMatch(false) }},
			core.Text{ContentFunc: count, MaxLines: 1, Style: core.Style{FontSize: 12, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Muted)},
			v.iconButton(v.text("find_previous"), lucide.ChevronUp, 24, nil, v.is(func() bool { return len(h.matches) == 0 }), func() { v.stepMatch(true) }, "find-previous"),
			v.iconButton(v.text("find_next"), lucide.ChevronDown, 24, nil, v.is(func() bool { return len(h.matches) == 0 }), func() { v.stepMatch(false) }, "find-next"),
			v.iconButton(v.text("close_details"), lucide.X, 24, nil, nil, closeBar, "find-close"),
		}}
	}}
}

// commitRow shows a commit's graph, refs, subject, author, hash and date,
// with the checked out commit's subject bold. A merge's node collapses or
// expands the commits it brings in, and a faint return mark after the
// subject tells that the message goes on.
func (v *View) commitRow(cx *core.Scope, item func() commitgraph.Item) core.Node {
	h := &v.history
	colors := ui.ColorsOf(cx)
	type look struct {
		hash, upstream string
		refs           string
		column         int
	}
	at := func() Commit {
		v.track()
		c, _ := v.commitAt(item().Index)
		return c
	}
	pills := core.NewMemo(cx, func() look {
		c := at()
		return look{hash: c.Hash, upstream: v.tracking.Upstream, refs: strings.Join(c.Refs, "\n"), column: item().Row.Column}
	})
	subject := core.Text{ContentFunc: func() string { return at().Subject }, MaxLines: 1, Style: core.Style{FontSize: 13, FlexShrink: 1, MinWidth: core.Pt(0)},
		StyleFunc: func() core.Style {
			s := core.Style{Color: colors.Text()}
			if c := at(); c.Hash != "" && c.Hash == v.repo.head {
				s.FontWeight = ui.Bold
			}
			return s
		}}
	mutedText := func(fn func() string, width core.Points, style core.Style) core.Node {
		return core.Text{ContentFunc: fn, MaxLines: 1, StyleFunc: ui.Colored(colors.Muted),
			Style: core.Style{FontSize: 12, Width: core.Pt(width), FlexShrink: 0}.Merge(style)}
	}
	author := core.NewMemo(cx, func() string {
		c := at()
		var b strings.Builder
		b.WriteString(c.Author + "\x00" + c.AuthorEmail)
		for _, p := range c.CoAuthors {
			b.WriteString("\x00" + p.Name + "\x00" + p.Email)
		}
		return b.String()
	})
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pt(commitgraph.RowHeight)},
		OnContextMenu: func(e core.ContextMenuEvent) {
			if c := core.Untracked(cx, at); c.Hash != "" {
				v.popup(cx, e.PagePosition, v.commitMenu(cx, c))
			}
		},
		Children: []core.Node{
			commitgraph.Graph{History: h.graph, Item: item, Color: graphColor,
				MergeStyleFunc: func() core.Style {
					return core.Style{Hover: &core.Style{Background: ui.Alpha(graphColor(item().Row.Line), 0.22)}}
				}},
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0),
				Gap: 6, Padding: core.Edges{Left: 4}}, Children: []core.Node{
				core.Dynamic(func(cx *core.Scope) core.Node {
					l := pills.Get()
					if l.refs == "" {
						return nil
					}
					// Refs give way to the subject rather than hiding it.
					var node core.Node
					cx.Untrack(func() {
						node = core.View{Style: core.Style{Direction: core.Row, FlexShrink: 1, MinWidth: core.Pt(0), MaxWidth: core.Pt(320), Gap: 4, Overflow: core.Hidden},
							Children: v.refPills(cx, strings.Split(l.refs, "\n"), l.upstream, l.column)}
					})
					return node
				}),
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(120), Gap: 4},
					Children: []core.Node{subject, core.Show{When: func() bool { return at().Body != "" }, Then: func(cx *core.Scope) core.Node {
						return ui.Icon(lucide.CornerDownLeft, 12, colors.Faint)
					}}}},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Width: core.Pt(140), FlexShrink: 0, Gap: 6}, Children: []core.Node{
					core.Dynamic(func(cx *core.Scope) core.Node {
						author.Get()
						var node core.Node
						cx.Untrack(func() { node = v.avatarStack(cx, at(), 16) })
						return node
					}),
					core.Text{ContentFunc: func() string { return at().Author }, MaxLines: 1, StyleFunc: ui.Colored(colors.Muted),
						Style: core.Style{FontSize: 12, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}},
				}},
				mutedText(func() string { return at().Short }, 64, core.Style{FontFamily: ui.Mono}),
				mutedText(func() string { return at().AuthorDate.Local().Format("2006-01-02 15:04") }, 112,
					core.Style{TextAlign: core.TextAlignEnd, FontVariant: core.FontTabularNums}),
			}},
		}}
}

// refPills labels the branches, remote branches and tags at a commit in the
// graph's column, leaving out remotes' HEAD aliases. A branch takes that
// column's color for its border and tint, as Fork colors its labels, and a
// tag takes Fork's tag hue. The checkout's refs are bold: its branch or
// detached HEAD, the branch's upstream, and tags at its commit. A remote
// branch carries the mark of its remote's host.
func (v *View) refPills(cx *core.Scope, refs []string, upstream string, column int) []core.Node {
	colors := ui.ColorsOf(cx)
	head := slices.ContainsFunc(refs, isHeadRef)
	var pills []core.Node
	for _, ref := range refs {
		icon, name, current, tag := lucide.GitBranch, ref, ref == upstream, false
		switch {
		case strings.HasPrefix(ref, "tag: "):
			icon, name, current, tag = lucide.Tag, strings.TrimPrefix(ref, "tag: "), head, true
		case isHeadRef(ref):
			icon, name, current = lucide.CircleDot, strings.TrimPrefix(ref, "HEAD -> "), true
		case strings.HasSuffix(ref, "/HEAD"):
			continue
		default:
			if remote := refRemote(v.repo.github, ref); remote != "" {
				icon = remoteIcon(v.repo.github, remote)
			}
		}
		weight := ui.Regular
		if current {
			weight = ui.Bold
		}
		label := ui.Text(name, 11, weight, colors.Text)
		label.MaxLines = 1
		pills = append(pills, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(18), FlexShrink: 0, Gap: 4,
			Padding: core.Symmetric(0, 6), CornerRadius: 5, BorderWidth: 1},
			StyleFunc: func() core.Style {
				border, background := refColors(column, cx.Dark())
				if tag {
					border, background = tagColors(cx.Dark())
				}
				return core.Style{BorderColor: border, Background: background}
			},
			Children: []core.Node{ui.Icon(icon, 11, colors.Text), label}})
	}
	return pills
}

// isHeadRef reports whether a commit's decoration marks the checkout's HEAD,
// on a branch or detached.
func isHeadRef(ref string) bool { return ref == "HEAD" || strings.HasPrefix(ref, "HEAD -> ") }

// headCommit is the commit the checkout's HEAD decorates, or empty.
func headCommit(commits []Commit) string {
	for _, c := range commits {
		if slices.ContainsFunc(c.Refs, isHeadRef) {
			return c.Hash
		}
	}
	return ""
}

// commitDetails shows the selected commit's message, changes or files
// below their tab row, which ends with a button closing them. Dragging the
// row resizes the details, and a double click restores their height.
func (v *View) commitDetails(cx *core.Scope) core.Node {
	h := &v.history
	tabs := ui.Tabs[int]{Variant: ui.TabsSegmented, Label: v.text("commit_details"), TestID: "detail-tabs",
		Options: []ui.Option[int]{{Value: detailMessage, Label: v.text("commit_message")}, {Value: detailChanges, Label: v.text("commit_changes")},
			{Value: detailFiles, Label: v.text("commit_files")}},
		Value: func() int { v.track(); return h.detail },
		OnChange: func(d int) {
			h.detail = d
			v.selectCommit(h.selected)
		}}
	var start core.Points
	bar := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.All(6), FlexShrink: 0, Cursor: core.CursorRowResize},
		OnPan: func(e core.PanEvent) {
			if e.Phase == core.GestureBegan {
				start = h.panelHeight()
			}
			h.panel = start - e.Translation.Y
			h.panel = h.panelHeight()
			v.invalidate()
		},
		OnPress: func(e core.PressEvent) {
			if e.Clicks >= 2 {
				h.panel = 0
				v.invalidate()
			}
		},
		Children: []core.Node{tabs, ui.Spacer{}, v.iconButton(v.text("close_details"), lucide.X, 28, nil, nil, func() {
			h.open = false
			v.invalidate()
		}, "close-details")}}
	shown := core.NewMemo(cx, func() string {
		v.track()
		if commitIndex(v.repo.commits, h.selected) < 0 {
			return ""
		}
		return h.selected + "\x00" + strconv.Itoa(h.detail)
	})
	content := core.Dynamic(func(cx *core.Scope) core.Node {
		key := shown.Get()
		var node core.Node
		cx.Untrack(func() {
			if key == "" {
				node = centered(muted(cx, v.text("choose_commit"), 13))
				return
			}
			switch h.detail {
			case detailChanges:
				node = v.filesView(cx, h.changes, v.text("no_changes"))
			case detailFiles:
				node = v.filesView(cx, h.files, v.text("no_files"))
			default:
				i := commitIndex(v.repo.commits, h.selected)
				node = v.commitMessage(cx, v.repo.commits[i], v.graphColumn(i))
			}
		})
		return node
	})
	return core.View{Style: ui.FillRest, Children: []core.Node{bar, hairline(cx), content}}
}

// commitMessage shows a commit's author, its committer when another person
// committed it and the co-authors its message names, above its message,
// hash, parents and refs in its graph column; choosing a parent selects it.
func (v *View) commitMessage(cx *core.Scope, commit Commit, column int) core.Node {
	colors := ui.ColorsOf(cx)
	field := func(label string, value ...core.Node) core.Node {
		l := line(label, 12, ui.Regular, colors.Muted)
		l.Style.Width, l.Style.FlexShrink = core.Pt(88), 0
		values := row(6, value...)
		values.Style.FlexGrow, values.Style.FlexShrink, values.Style.MinWidth = 1, 1, core.Pt(0)
		return row(12, l, values)
	}
	person := func(role, name, email string, date time.Time) core.Node {
		lines := []core.Node{
			row(6, line(name, 13, ui.Bold, colors.Text), line(role, 11, ui.Regular, colors.Muted)),
			line(email, 12, ui.Regular, colors.Muted),
		}
		if !date.IsZero() {
			lines = append(lines, line(date.Local().Format("2006-01-02 15:04:05 -0700"), 12, ui.Regular, colors.Muted))
		}
		text := core.View{Selectable: true, Style: core.Style{Gap: 2, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: lines}
		r := row(10, v.avatarView(cx, name, email, 36), text)
		r.Style.FlexGrow, r.Style.FlexShrink, r.Style.FlexBasis, r.Style.MinWidth = 1, 1, core.Pt(0), core.Pt(0)
		return r
	}
	people := []core.Node{person(v.text("commit_author"), commit.Author, commit.AuthorEmail, commit.AuthorDate)}
	if commit.Committer != commit.Author || commit.CommitterEmail != commit.AuthorEmail {
		people = append(people, person(v.text("commit_committer"), commit.Committer, commit.CommitterEmail, commit.CommitDate))
	}
	for _, co := range commit.CoAuthors {
		people = append(people, person(v.text("commit_co_author"), co.Name, co.Email, time.Time{}))
	}
	var nodes []core.Node
	// People sit two to a row, keeping equal widths in a last row of one
	// when there are more than two.
	for i := 0; i < len(people); i += 2 {
		pair := []core.Node{people[i]}
		if i+1 < len(people) {
			pair = append(pair, people[i+1])
		} else if len(people) > 2 {
			pair = append(pair, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)}})
		}
		nodes = append(nodes, row(16, pair...))
	}
	nodes = append(nodes, hairline(cx))
	// The message can be selected and copied.
	message := []core.Node{core.Text{Content: commit.Subject, Style: core.Style{FontSize: 15, FontWeight: ui.Bold}}}
	if commit.Body != "" {
		message = append(message, core.Text{Content: commit.Body, Style: core.Style{FontSize: 13}})
	}
	nodes = append(nodes, core.View{Selectable: true, Style: core.Style{Gap: 8}, Children: message}, hairline(cx))
	hash := line(commit.Hash, 13, ui.Regular, colors.Text)
	hash.Selectable = core.SelectableOn
	nodes = append(nodes, field(v.text("commit_hash"), hash))
	if len(commit.Parents) > 0 {
		var links []core.Node
		for _, parent := range commit.Parents {
			links = append(links, ui.Link{Text: parent[:min(len(parent), 10)], TestID: "parent-" + parent, OnPress: func() {
				v.ref = ""
				v.revealCommit(parent)
			}})
		}
		nodes = append(nodes, field(v.text("commit_parents"), links...))
	}
	if len(commit.Refs) > 0 {
		nodes = append(nodes, field(v.text("commit_refs"), v.refPills(cx, commit.Refs, v.tracking.Upstream, column)...))
	}
	return ui.ScrollView{Style: ui.FillRest, ContentStyle: core.Style{Gap: 8, Padding: core.Symmetric(12, 16)}, Children: nodes}
}
