package gitview

import (
	"slices"
	"strconv"
	"strings"
	"time"

	liquid "github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
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
	list             components.ListState
	cursor           int
	// collapsed holds the merges whose commits fold into their row, folded
	// maps each commit they hide to its merge, and rows and graph are the
	// shown commits' indexes and revision graph. toggle is a merge to
	// collapse or expand before the next frame lays out the list.
	toggle    string
	collapsed map[string]bool
	folded    map[string]string
	rows      []int
	graph     []graphRow
	// selected is the selected row's hash, and reveal a commit to select
	// once the history is read.
	selected, reveal string
	// open shows the selected commit's details floating over the list's
	// lower part, and height is the history's height, from which the list
	// keeps its last rows and scroll bar clear of them. panel is the height
	// their tab row was dragged to, zero for the default.
	open          bool
	detail        int
	height, panel float32
	// changes holds the selected commit's changed files and files all of
	// its files.
	changes, files *filePane
}

func (v *View) initHistory() {
	h := &v.history
	h.changes = v.newFilePane(false)
	h.files = v.newFilePane(true)
	h.list.Selected = &h.cursor
	h.list.Key = func(i int) any { return v.repo.commits[h.rows[i]].Hash }
	h.list.Label = func(i int) string { return v.repo.commits[h.rows[i]].Subject }
	h.list.OnChange = func() {
		if h.cursor >= 0 && h.cursor < len(h.rows) {
			v.ref = ""
			v.selectCommit(v.repo.commits[h.rows[h.cursor]].Hash)
		}
	}
	// A double click or Enter opens the chosen commit's details and asks to
	// check it out; an entry's row only opens.
	h.list.OnSubmit = func(i int) {
		if i >= 0 && i < len(h.rows) {
			commit := v.repo.commits[h.rows[i]]
			v.ref = ""
			v.selectCommit(commit.Hash)
			h.open = true
			if _, entry := entryID(commit.Hash); !entry {
				v.openCheckout(commitTarget(commit))
			}
		}
	}
}

// clearHistory forgets the selection, search and details of another
// worktree.
func (v *View) clearHistory() {
	h := &v.history
	h.finding, h.search, h.searched, h.matches, h.match = false, "", "", nil, 0
	h.selected, h.reveal, h.cursor = "", "", 0
	h.collapsed, h.toggle = nil, ""
	v.clearFiles(h.changes, "")
	v.clearFiles(h.files, "")
	h.changes.key, h.files.key = "", ""
}

// applyHistory searches newly read commits and keeps the selected row, or
// selects the one waiting to be revealed, else the newest commit, passing
// over the entries' rows.
func (v *View) applyHistory() {
	h := &v.history
	h.fold(v.repo)
	h.matches, h.searched = searchCommits(v.repo.commits, h.search), h.search
	h.match = max(0, slices.Index(h.matches, commitIndex(v.repo.commits, h.selected)))
	target := h.selected
	if h.reveal != "" {
		target, h.reveal = h.reveal, ""
	}
	if commitIndex(v.repo.commits, target) < 0 {
		target = ""
		for _, c := range v.repo.commits {
			if _, ok := entryID(c.Hash); !ok {
				target = c.Hash
				break
			}
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

// fold lays out the shown commits of repo's history, hiding those the
// collapsed merges fold.
func (h *historyState) fold(repo repositoryState) {
	h.folded = foldCommits(repo.commits, h.collapsed)
	h.rows = h.rows[:0]
	if len(h.folded) == 0 {
		for i := range repo.commits {
			h.rows = append(h.rows, i)
		}
		h.graph = repo.graph
		return
	}
	shown := make([]Commit, 0, len(repo.commits)-len(h.folded))
	for i, commit := range repo.commits {
		if h.folded[commit.Hash] != "" {
			continue
		}
		parents := commit.Parents[:0:0]
		for _, parent := range commit.Parents {
			if h.folded[parent] == "" {
				parents = append(parents, parent)
			}
		}
		commit.Parents = parents
		shown = append(shown, commit)
		h.rows = append(h.rows, i)
	}
	h.graph = buildGraph(shown)
}

// toggleMerge collapses or expands a merge's commits, keeping the selected
// commit shown: one the merge folds selects the merge instead.
func (v *View) toggleMerge(hash string) {
	h := &v.history
	if h.collapsed[hash] {
		delete(h.collapsed, hash)
	} else {
		if h.collapsed == nil {
			h.collapsed = map[string]bool{}
		}
		h.collapsed[hash] = true
	}
	h.fold(v.repo)
	selected := h.selected
	if h.folded[selected] != "" {
		v.ref, selected = "", hash
	}
	v.showCommit(selected)
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

// searchCommits returns the indexes of commits whose subject, message,
// author, co-author, hash or refs contain query, ignoring case, or nil for
// an empty query. The entries' rows are not commits, so they never match.
func searchCommits(commits []Commit, query string) []int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	matches := []int{}
	for i, c := range commits {
		if _, ok := entryID(c.Hash); ok {
			continue
		}
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

// showCommit selects a commit and moves the list's cursor to it, expanding
// the merges that fold it.
func (v *View) showCommit(hash string) {
	h := &v.history
	if h.folded[hash] != "" {
		for h.folded[hash] != "" {
			delete(h.collapsed, h.folded[hash])
			h.folded = foldCommits(v.repo.commits, h.collapsed)
		}
		h.fold(v.repo)
	}
	if h.cursor = slices.Index(h.rows, commitIndex(v.repo.commits, hash)); h.cursor >= 0 {
		h.list.ScrollIntoView(h.cursor)
	}
	v.selectCommit(hash)
}

// selectCommit shows a row's details, reading the files its tab needs, and
// opens them when another row is chosen. An entry's row rereads its files
// with the host's readers.
func (v *View) selectCommit(hash string) {
	h := &v.history
	changed := hash != h.selected
	if changed {
		h.open = hash != ""
	}
	h.selected = hash
	defer v.invalidate()
	defer func() {
		v.followWatches(false)
		if changed && v.opts.OnSelect != nil {
			v.opts.OnSelect()
		}
	}()
	if id, ok := entryID(hash); ok {
		v.loadEntry(id)
		return
	}
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

// loadEntry reads the files the selected entry row's tab shows, again in
// place when they were read before.
func (v *View) loadEntry(id string) {
	h := &v.history
	e, ok := v.repo.entries[id]
	pane, read := h.changes, e.Changes
	if h.detail == detailFiles {
		pane, read = h.files, e.Files
	}
	if !ok || read == nil {
		v.clearFiles(pane, v.text("entry_unavailable"))
		pane.key = ""
		return
	}
	v.loadFiles(pane, entryPrefix+id, read)
}

// selectedWatch is the directory of the selected entry row while the
// history shows, or empty.
func (v *View) selectedWatch() string {
	if entry, _ := v.Selected(); entry != "" {
		return v.repo.entries[entry].Watch
	}
	return ""
}

// panelHeight is the open details' height, within the history's limits
// once its height is known.
func (h *historyState) panelHeight() float32 {
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
func (v *View) historyView(c *ui.Context) {
	t := c.Theme()
	h := &v.history
	message := ""
	switch {
	case v.repo.err != "":
		message = v.repo.err
	case !v.repo.loaded:
		ui.Column(c).Fill().Center().Children(func() { components.Spinner(c, components.SpinnerRing) })
		return
	case len(v.repo.commits) == 0:
		message = v.text("no_commits")
	}
	if message != "" {
		ui.Column(c).Fill().Center().Padding(t.Space(4)).Children(func() {
			components.Text(c, message).Wrap().TextColor(t.TextMuted)
		})
		return
	}
	if h.toggle != "" {
		v.toggleMerge(h.toggle)
		h.toggle = ""
	}
	if c.Shortcut(v.shortcut(), ui.KeyF) {
		h.finding = true
	}
	// Escape hides the details unless the find bar took it.
	if h.open && c.Shortcut(0, ui.KeyEscape) {
		h.open = false
	}
	list := func() {
		ui.Column(c).Fill().MinHeight(0).Children(func() {
			// The overlay covers the find bar's place or else the list's
			// first rows.
			top := v.overlayHeight
			if h.finding {
				v.findCommits()
			}
			// The bar's field is inset as far from its sides as from its top.
			bar := ui.FindBar(c, &h.finding, &h.search, len(h.matches), &h.match).FillWidth().Padding(t.Space(1.5))
			if h.finding {
				bar.Margin(top, 0, 0, 0)
				top = 0
			}
			if bar.Changed() {
				v.showMatch()
			}
			// The list reaches the edges, insetting only its rows; its rows and
			// scroll bar start clear of the overlay and end clear of the open
			// details.
			var covered float32
			if h.open {
				covered = h.panelHeight() + historyPanelMargin
			}
			components.List(c, &h.list, len(h.rows), func(i int) {
				v.commitRow(c, v.repo.commits[h.rows[i]], h.graph[i])
			}).Fill().Padding(top+t.Space(1.5), t.Space(2), covered+t.Space(2), t.Space(2)).
				ScrollbarInsets(top+min(top, historyPanelMargin), 0, covered+min(covered, historyPanelMargin), 0)
		})
	}
	// The list keeps its place in the tree as the details open and close,
	// so its rows keep their state.
	ui.Column(c).Fill().MinHeight(0).Draw(func(_ *ui.Painter, r ui.Rect) {
		if h.height != r.H {
			h.height = r.H
			c.Invalidate()
		}
	}).Children(func() {
		list()
		if !h.open {
			return
		}
		// A faint lift separates the panel from the rows it covers.
		shadow := ui.RGBA(0, 0, 0, .08)
		if t.Dark {
			shadow = ui.RGBA(0, 0, 0, .24)
		}
		ui.Column(c).Key("commit-details").Absolute().Height(h.panelHeight()).
			Right(historyPanelMargin).Bottom(historyPanelMargin).Left(historyPanelMargin).MinHeight(0).
			Radius(t.Radius*1.5).Border(1, t.Border).Clip().
			Material(liquid.Glass{Style: liquid.Regular, Tint: t.Surface.Alpha(.7)}).
			Shadow(0, 2, 8, 0, shadow).Children(func() { v.commitDetails(c) })
	})
}

// commitRowHeight is a commit row's height, which the graph's lines span.
const commitRowHeight = 26

// commitRow shows a commit's graph, refs, subject, author, hash and date,
// with the checked out commit's subject bold. A merge's node collapses or
// expands the commits it brings in, and a faint return mark after the
// subject tells that the message goes on.
func (v *View) commitRow(c *ui.Context, commit Commit, row graphRow) {
	t := c.Theme()
	if id, ok := entryID(commit.Hash); ok {
		v.entryRow(c, v.repo.entries[id], row)
		return
	}
	var merge *mergeToggle
	if len(commit.Parents) > 1 {
		collapsed := v.history.collapsed[commit.Hash]
		label := v.text("collapse_merge")
		if collapsed {
			label = v.text("expand_merge")
		}
		merge = &mergeToggle{collapsed: collapsed, label: label, toggle: func() { v.history.toggle = commit.Hash }}
	}
	ui.Row(c).FillWidth().MinWidth(0).Height(commitRowHeight).AlignItems(ui.Center).Padding(0, t.Space(2)).ContextMenu(func(m *ui.Menu) {
		v.refMenu(c, m, commitTarget(commit), nil)
	}).Children(func() {
		graphCell(c, row, commitRowHeight, merge)
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Padding(0, 0, 0, t.Space(1)).Children(func() {
			if len(commit.Refs) > 0 {
				// Refs give way to the subject rather than hiding it.
				ui.Row(c).Shrink(1).MinWidth(0).MaxWidth(320).Gap(t.Space(1)).Clip().Children(func() { refPills(c, commit.Refs, v.tracking.Upstream, v.repo.github) })
			}
			ui.Row(c).Grow(1).MinWidth(120).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
				subject := components.Text(c, commit.Subject).FontSize(13).MinWidth(0).SingleLine().Ellipsis("…")
				if commit.Hash == v.repo.head {
					subject.Bold()
				}
				if commit.Body != "" {
					components.Icon(c, lucide.CornerDownLeft).Size(12, 12).Shrink(0).TextColor(t.TextMuted.Alpha(.6))
				}
			})
			ui.Row(c).Width(140).Shrink(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
				v.avatarStack(c, commit, 16)
				components.Text(c, commit.Author).FontSize(12).Grow(1).MinWidth(0).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			})
			components.Text(c, commit.Short).FontSize(12).Width(64).Shrink(0).TextColor(t.TextMuted).SingleLine()
			components.Text(c, commit.AuthorDate.Local().Format("2006-01-02 15:04")).FontSize(12).Width(112).Shrink(0).
				TextColor(t.TextMuted).SingleLine().TextAlign(ui.End)
		})
	})
}

// entryRow shows a host entry's row in the graph above the commit it
// names: its icon or the host's leading mark, its bold title, and the
// author and date columns the host fills.
func (v *View) entryRow(c *ui.Context, e Entry, row graphRow) {
	t := c.Theme()
	ui.Row(c).FillWidth().MinWidth(0).Height(commitRowHeight).AlignItems(ui.Center).Padding(0, t.Space(2)).Children(func() {
		graphCell(c, row, commitRowHeight, nil)
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Padding(0, 0, 0, t.Space(1)).Children(func() {
			ui.Row(c).Grow(1).MinWidth(120).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
				if e.Lead != nil {
					e.Lead(c)
				} else {
					icon := e.Icon
					if icon == nil {
						icon = lucide.FileDiff
					}
					components.Icon(c, icon).Size(13, 13).Shrink(0).TextColor(t.Accent)
				}
				components.Text(c, e.Title).FontSize(13).Bold().MinWidth(0).SingleLine().Ellipsis("…")
			})
			ui.Row(c).Width(140).Shrink(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
				if e.Author != nil {
					e.Author(c)
				}
			})
			ui.Box(c).Width(64).Shrink(0)
			ui.Row(c).Width(112).Shrink(0).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				if e.Date != nil {
					e.Date(c)
				}
			})
		})
	})
}

// refPills labels the branches, remote branches and tags at a commit,
// leaving out remotes' HEAD aliases. The checkout's refs are bold: its
// branch or detached HEAD, the branch's upstream, and tags at its commit.
// A remote branch carries the mark of its remote's host from github, and a
// tag is cyan with an outline of the same hue.
func refPills(c *ui.Context, refs []string, upstream string, github map[string]bool) {
	t := c.Theme()
	head := slices.ContainsFunc(refs, isHeadRef)
	cyan := ui.RGB(8, 145, 178)
	if t.Dark {
		cyan = ui.RGB(34, 211, 238)
	}
	for _, ref := range refs {
		icon, name, color, current, outline := lucide.GitBranch, ref, t.Accent, ref == upstream, false
		switch {
		case strings.HasPrefix(ref, "tag: "):
			icon, name, color, current, outline = lucide.Tag, strings.TrimPrefix(ref, "tag: "), cyan, head, true
		case isHeadRef(ref):
			icon, name, current = lucide.CircleDot, strings.TrimPrefix(ref, "HEAD -> "), true
		case strings.HasSuffix(ref, "/HEAD"):
			continue
		default:
			if remote := refRemote(github, ref); remote != "" {
				icon = remoteIcon(github, remote)
			}
		}
		pill := ui.Row(c).Shrink(0).Height(18).Radius(5).Background(color.Alpha(.14)).Padding(0, 6).Gap(4).AlignItems(ui.Center)
		if outline {
			pill.Border(1, color.Alpha(.5))
		}
		pill.Children(func() {
			components.Icon(c, icon).Size(11, 11).Shrink(0).TextColor(color)
			label := components.Text(c, name).FontSize(11).TextColor(color).SingleLine()
			if current {
				label.Bold()
			}
		})
	}
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
func (v *View) commitDetails(c *ui.Context) {
	t := c.Theme()
	h := &v.history
	i := commitIndex(v.repo.commits, h.selected)
	// An entry's row has no message, so it shows its changes in its place.
	_, entry := entryID(h.selected)
	detail := h.detail
	if entry {
		detail = max(detail, detailChanges)
	}
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		bar := ui.Row(c).FillWidth().Shrink(0).Padding(t.Space(1.5)).AlignItems(ui.Center).Cursor(ui.CursorResizeNS)
		bar.Children(func() {
			var tabs ui.TabsParts
			if entry {
				tab := detail - detailChanges
				tabs = components.Tabs(c, &tab, components.TabsHighlight, v.text("commit_changes"), v.text("commit_files"))
				if tabs.List.Changed() {
					h.detail = tab + detailChanges
				}
			} else {
				tabs = components.Tabs(c, &h.detail, components.TabsHighlight,
					v.text("commit_message"), v.text("commit_changes"), v.text("commit_files"))
			}
			if tabs.List.Changed() {
				v.selectCommit(h.selected)
			}
			ui.Box(c).Grow(1)
			if iconButton(c, v.text("close_details"), lucide.X, max(28, t.Rem(1.75)), false).Clicked() {
				h.open = false
				v.invalidate()
			}
		})
		if _, dy, held := bar.Dragged(); held && dy != 0 {
			h.panel = h.panelHeight() - dy
			h.panel = h.panelHeight()
			c.Invalidate()
		}
		if bar.DoubleClicked() {
			h.panel = 0
			c.Invalidate()
		}
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
		if i < 0 {
			ui.Column(c).Fill().Center().Padding(t.Space(4)).Children(func() {
				components.Text(c, v.text("choose_commit")).Wrap().TextColor(t.TextMuted)
			})
			return
		}
		switch detail {
		case detailChanges:
			v.filesView(c, h.changes, v.text("no_changes"))
		case detailFiles:
			v.filesView(c, h.files, v.text("no_files"))
		default:
			v.commitMessage(c, v.repo.commits[i])
		}
	})
}

// commitMessage shows a commit's author, its committer when another person
// committed it and the co-authors its message names, above its message,
// hash, parents and refs; choosing a parent selects it.
func (v *View) commitMessage(c *ui.Context, commit Commit) {
	t := c.Theme()
	field := func(label string, value func()) {
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
			components.Text(c, label).FontSize(12).Width(88).Shrink(0).TextColor(t.TextMuted).SingleLine()
			ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(value)
		})
	}
	person := func(role, name, email string, date time.Time) {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(2.5)).Children(func() {
			v.avatarView(c, name, email, 36)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Selectable().Children(func() {
				ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
					components.Text(c, name).FontSize(13).Bold().MinWidth(0).SingleLine().Ellipsis("…")
					components.Text(c, role).FontSize(11).Shrink(0).TextColor(t.TextMuted).SingleLine()
				})
				components.Text(c, email).FontSize(12).TextColor(t.TextMuted).MinWidth(0).SingleLine().Ellipsis("…")
				if !date.IsZero() {
					components.Text(c, date.Local().Format("2006-01-02 15:04:05 -0700")).FontSize(12).TextColor(t.TextMuted).SingleLine()
				}
			})
		})
	}
	people := []func(){func() { person(v.text("commit_author"), commit.Author, commit.AuthorEmail, commit.AuthorDate) }}
	if commit.Committer != commit.Author || commit.CommitterEmail != commit.AuthorEmail {
		people = append(people, func() {
			person(v.text("commit_committer"), commit.Committer, commit.CommitterEmail, commit.CommitDate)
		})
	}
	for _, co := range commit.CoAuthors {
		people = append(people, func() { person(v.text("commit_co_author"), co.Name, co.Email, time.Time{}) })
	}
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Gap(t.Space(2)).Padding(t.Space(3), t.Space(4)).Children(func() {
			// People sit two to a row, keeping equal widths in a last row of one
			// when there are more than two.
			for i := 0; i < len(people); i += 2 {
				ui.Row(c).Key("people-" + strconv.Itoa(i)).FillWidth().MinWidth(0).Gap(t.Space(4)).Children(func() {
					people[i]()
					if i+1 < len(people) {
						people[i+1]()
					} else if len(people) > 2 {
						ui.Box(c).Grow(1).Basis(0)
					}
				})
			}
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border).Margin(t.Space(1), 0)
			// The message can be selected and copied.
			ui.Column(c).FillWidth().Shrink(0).Gap(t.Space(2)).Selectable().Children(func() {
				components.Text(c, commit.Subject).Bold().FontSize(15).Wrap().FillWidth()
				if commit.Body != "" {
					components.Text(c, commit.Body).FontSize(13).Wrap().FillWidth()
				}
			})
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border).Margin(t.Space(1), 0)
			field(v.text("commit_hash"), func() {
				components.Text(c, commit.Hash).FontSize(13).SingleLine().Ellipsis("…").MinWidth(0)
			})
			if len(commit.Parents) > 0 {
				field(v.text("commit_parents"), func() {
					for _, parent := range commit.Parents {
						short := parent[:min(len(parent), 10)]
						ui.Column(c).Key("parent-" + parent).Shrink(0).Children(func() {
							if components.Button(c, short, components.ButtonLink).Clicked() {
								v.ref = ""
								v.revealCommit(parent)
							}
						})
					}
				})
			}
			if len(commit.Refs) > 0 {
				field(v.text("commit_refs"), func() { refPills(c, commit.Refs, v.tracking.Upstream, v.repo.github) })
			}
		})
	})
}
