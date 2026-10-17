// Package gitview is a reusable Git repository view: a refs pane with the
// worktree's uncommitted changes, the history and the branches, remote
// branches and tags, beside the chosen section, either the changed files or
// the revision graph with the selected commit's details. It also runs the
// fetch, pull, push and ref deletions its host's toolbar and menus ask for,
// with their dialogs.
//
// The host owns the window, the project list and the toolbar; the view owns
// everything inside its area. Git runs off the UI thread and results return
// through Options.Dispatch.
package gitview

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Default widths of the refs pane and the file trees, and their limits; the
// section keeps at least sectionMinWidth.
const (
	refsWidth, refsMinWidth, refsMaxWidth, sectionMinWidth = 220, 160, 400, 320
	treeWidth                                              = 260
)

// Options configure a View. Dispatch is required; the rest is optional.
type Options struct {
	// Dispatch runs fn on the UI thread before the next frame.
	Dispatch func(fn func())
	// Invalidate redraws the host window.
	Invalidate func()
	// Git runs the git executable, such as with a login environment.
	Git Git
	// Language picks the built-in strings: a Chinese tag such as "zh-Hans"
	// picks Chinese, anything else English. Text, when set, answers first,
	// with an empty string falling back to the built-in one.
	Language string
	Text     func(key string, args map[string]any) string
	// Shortcut is the modifier of the view's shortcuts, such as ⌘F for the
	// history's find bar; zero means Super.
	Shortcut ui.Modifiers
	// AvatarDir caches commit authors' GitHub or Gravatar pictures; empty
	// draws initials only and never downloads.
	AvatarDir string
	// RefFilter keeps the refs it accepts, hiding the others from the refs
	// pane and the history, along with the commits only they reach. Nil
	// keeps every ref.
	RefFilter func(kind RefKind, name string) bool
	// Entries lists rows the host inserts into the history, read with each
	// repository read.
	Entries func() []Entry
	// Busy reports host work, such as a landing, that the view's Git writes
	// must not race; it disables them.
	Busy func() bool
	// Watch rereads the view when the repository or the shown worktrees
	// change on disk. Without it the view reads only when asked.
	Watch bool
	// State restores the remembered layout and choices; OnState receives
	// them whenever the user changes them.
	State   State
	OnState func(State)
	// OnSelect reports that the shown section or selected history row
	// changed; Selected reads the row.
	OnSelect func()
	// OnWrite reports that a Git write the view ran, named op, finished,
	// whether or not it failed.
	OnWrite func(op string)
	// Overlay builds content floating at the top of the section, such as a
	// command panel, whose rows the history keeps clear of.
	Overlay func(c *ui.Context)
	// DialogInset keeps the view's dialogs below a host title bar.
	DialogInset func(c *ui.Context) float32
}

// State is the view's remembered layout and choices.
type State struct {
	// RefsWidth is the refs pane's width and TreeWidth that of the file
	// trees; zero keeps the default.
	RefsWidth float32 `yaml:"refsWidth,omitempty" json:"refsWidth,omitempty"`
	TreeWidth float32 `yaml:"treeWidth,omitempty" json:"treeWidth,omitempty"`
	// Groups holds the disclosure of the refs pane's groups by key:
	// "branches", "remotes" and "tags".
	Groups map[string]bool `yaml:"groups,omitempty" json:"groups,omitempty"`
	// DiffWrap wraps diff and file lines.
	DiffWrap bool `yaml:"diffWrap,omitempty" json:"diffWrap,omitempty"`
	// FetchBranch, PullRebase, PullStash and PushTags are the fetch, pull
	// and push dialogs' remembered options; FetchBranch fetches the chosen
	// branch instead of every remote.
	FetchBranch bool `yaml:"fetchBranch,omitempty" json:"fetchBranch,omitempty"`
	PullRebase  bool `yaml:"pullRebase,omitempty" json:"pullRebase,omitempty"`
	PullStash   bool `yaml:"pullStash,omitempty" json:"pullStash,omitempty"`
	PushTags    bool `yaml:"pushTags,omitempty" json:"pushTags,omitempty"`
}

// Entry is a row the host inserts into the history right above the commit
// Head names, which the row descends from in the graph. Its own commits,
// from Head down, join the history even when no shown ref reaches them.
type Entry struct {
	// ID tells entries apart; Title names the row and Icon leads it.
	ID, Title string
	Icon      func() *ui.SVG
	// Head is a revision such as a branch or commit.
	Head string
	// Author and Date fill those columns, which stay blank when nil.
	Author, Date func(c *ui.Context)
	// Changes and Files read what the details' Changes and Files tabs show
	// for the row; a nil one shows nothing to compare.
	Changes, Files func(git Git) (Changes, error)
	// Watch is a directory to watch while the row is selected.
	Watch string
}

// entryPrefix starts the hash of an entry's row, which no commit hash can
// begin with; the entry's ID follows it.
const entryPrefix = "entry:"

func entryID(hash string) (string, bool) { return strings.CutPrefix(hash, entryPrefix) }

// section is what the view shows beside its refs pane.
type section int

const (
	// sectionChanges lists the worktree's uncommitted files.
	sectionChanges section = iota
	// sectionCommits shows the repository's history.
	sectionCommits
)

// View is the repository view of one worktree at a time.
type View struct {
	opts    Options
	git     Git
	avatars *avatarCache
	state   State
	// path is the worktree shown.
	path    string
	section section
	// overlayHeight is the overlay's height, which the history's list keeps
	// its first rows clear of.
	overlayHeight float32
	// ref is the refs pane's row of the branch, remote branch or tag last
	// chosen in sectionCommits.
	ref string
	// groups holds the disclosure of the refs pane's groups and ref
	// folders, and refFilter the text that narrows their refs.
	groups    map[string]*bool
	refFilter string
	// seekChanges shows the worktree's changes once read when there are
	// some, after the view opened or moved to another worktree.
	seekChanges bool

	repo repositoryState
	// current holds the worktree's uncommitted changes.
	current *filePane
	history historyState
	// tracking compares the worktree's branch with its upstream; syncing
	// names the Git write running, and toast reports the last one that
	// failed; dialog chooses a fetch's, pull's or push's branches.
	tracking           Tracking
	trackingGeneration uint64
	syncing            string
	dialog             syncDialog
	toast              *ui.Toast
	// deletion confirms deleting a ref from the refs pane.
	deletion refDeletion
	// watches follow the repository's Git directory and shown worktrees.
	watches map[string]*gitWatch
	closed  bool
}

// repositoryState is the repository's history and refs, and its worktrees'
// checked out commits.
type repositoryState struct {
	// generation discards results of reads that a newer one replaced, and
	// again repeats a read asked for while one ran.
	generation             uint64
	loading, loaded, again bool
	err                    string
	// signature describes what the commits were read for, so a reread that
	// finds it unchanged keeps the history.
	signature string
	// commits holds the history with the entries' rows, of which count
	// are commits.
	commits []Commit
	count   int
	graph   []graphRow
	// head is the commit the worktree has checked out.
	head      string
	refTrees  refTrees
	checkouts []Checkout
	// github maps each remote to whether it is hosted on GitHub.
	github map[string]bool
	// entries are the host's rows by ID.
	entries map[string]Entry
}

// New creates a view showing nothing until SetPath.
func New(opts Options) *View {
	v := &View{opts: opts, git: opts.Git, state: opts.State, section: sectionCommits, groups: map[string]*bool{}}
	if v.state.RefsWidth <= 0 {
		v.state.RefsWidth = refsWidth
	}
	v.state.RefsWidth = min(v.state.RefsWidth, refsMaxWidth)
	if v.state.TreeWidth <= 0 {
		v.state.TreeWidth = treeWidth
	}
	if opts.AvatarDir != "" {
		v.avatars = newAvatarCache(opts.AvatarDir)
	}
	v.current = v.newFilePane(false)
	v.initHistory()
	return v
}

func (v *View) dispatch(fn func()) { v.opts.Dispatch(fn) }

func (v *View) invalidate() {
	if v.opts.Invalidate != nil {
		v.opts.Invalidate()
	}
}

func (v *View) saveState() {
	if v.opts.OnState != nil {
		v.opts.OnState(v.state)
	}
}

func (v *View) shortcut() ui.Modifiers {
	if v.opts.Shortcut != 0 {
		return v.opts.Shortcut
	}
	return ui.Super
}

// Path is the worktree the view shows.
func (v *View) Path() string { return v.path }

// SetPath shows the worktree at path, discarding another worktree's changes,
// history and pending reads; an empty path shows nothing. Its changes
// replace the section once read when there are some.
func (v *View) SetPath(path string) {
	if v.path == path {
		return
	}
	v.path = path
	v.repo.generation++
	v.repo = repositoryState{generation: v.repo.generation}
	v.clearFiles(v.current, "")
	v.current.key = ""
	v.seekChanges = true
	v.clearHistory()
	v.ref = ""
	v.tracking = Tracking{}
	v.Reload()
}

// Reload rereads the repository, the worktree's changes and the selected
// row's files.
func (v *View) Reload() {
	if v.closed {
		return
	}
	v.loadRepository()
	v.loadChanges()
	v.targetChanged()
}

// Resume restarts the watches that ended, such as on an unmounted volume,
// rereading the view first since it may have missed changes; a host calls
// it when its window gains focus.
func (v *View) Resume() {
	for _, watch := range v.watches {
		if watch.ended {
			v.Reload()
			break
		}
	}
	v.followWatches(true)
}

// Close stops the view's watches and drops its pending results.
func (v *View) Close() {
	v.closed = true
	v.stopWatches()
}

// Selected names the selected history row while the history shows: an
// entry's ID, or else a commit's hash; both are empty otherwise.
func (v *View) Selected() (entry, hash string) {
	if v.section != sectionCommits {
		return "", ""
	}
	if id, ok := entryID(v.history.selected); ok {
		return id, ""
	}
	return "", v.history.selected
}

// RevealEntry shows the history with the entry's row selected and
// scrolled into view, once read.
func (v *View) RevealEntry(id string) {
	v.seekChanges, v.ref = false, ""
	v.revealCommit(entryPrefix + id)
}

// Loading reports whether the worktree's repository or changes are being
// read.
func (v *View) Loading() bool {
	return v.path != "" && (!v.repo.loaded || v.repo.loading || v.current.loading)
}

// available reports whether the worktree has a repository to show.
func (v *View) available() bool {
	return v.path != "" && v.repo.loaded && v.repo.err == ""
}

// targetChanged rereads the branch's tracking and moves the watches after
// the shown section, row or worktree changed.
func (v *View) targetChanged() {
	v.loadTracking()
	v.followWatches(false)
	if v.opts.OnSelect != nil {
		v.opts.OnSelect()
	}
}

// hasCurrentChanges reports whether the worktree has uncommitted changes, or
// reading them failed, so the refs pane offers them.
func (v *View) hasCurrentChanges() bool {
	p := v.current
	return v.path != "" && p.loaded && (p.err != "" || len(p.changes.Files) > 0)
}

// showFoundChanges shows the worktree's changes once read when there are
// some after the view opened or moved, reporting whether it did.
func (v *View) showFoundChanges() bool {
	if !v.seekChanges {
		return false
	}
	v.seekChanges = false
	if v.section == sectionChanges || v.current.err != "" || len(v.current.changes.Files) == 0 {
		return false
	}
	v.section, v.ref = sectionChanges, ""
	return true
}

// leaveEmptyChanges shows the history instead of the worktree's changes
// when there are none, reporting whether it did.
func (v *View) leaveEmptyChanges() bool {
	if v.section != sectionChanges || v.hasCurrentChanges() {
		return false
	}
	v.section, v.ref = sectionCommits, ""
	return true
}

// loadChanges rereads the worktree's uncommitted files. A worktree without
// commits has none to compare.
func (v *View) loadChanges() {
	path := v.path
	if path == "" {
		v.clearFiles(v.current, "")
		v.current.key = ""
		return
	}
	v.loadFiles(v.current, path, func(git Git) (Changes, error) {
		if _, err := git.Top(path); err == nil && git.Unborn(path) {
			return Changes{}, nil
		}
		return git.WorktreeChanges(path, "")
	})
}

// loadRepository rereads the refs and worktrees, and the history when they
// or the host's entries changed.
func (v *View) loadRepository() {
	defer v.invalidate()
	if v.path == "" {
		v.repo.generation++
		v.repo = repositoryState{generation: v.repo.generation}
		v.applyHistory()
		return
	}
	if v.repo.loading {
		v.repo.again = true
		return
	}
	v.repo.again = false
	v.repo.generation++
	generation, known, path, git, keep := v.repo.generation, v.repo.signature, v.path, v.git, v.opts.RefFilter
	var entries []Entry
	if v.opts.Entries != nil {
		entries = v.opts.Entries()
	}
	noHistory := v.text("no_history")
	v.repo.loading = true
	go func() {
		var refs, hidden Refs
		var checkouts []Checkout
		var github map[string]bool
		_, err := git.Top(path)
		if err == nil {
			if refs, err = git.Refs(path); err == nil {
				refs, hidden = refs.split(keep)
				checkouts, err = git.Checkouts(path)
			}
		} else {
			err = errorText(noHistory)
		}
		if err == nil {
			// Without them, remotes just lose their GitHub mark.
			github, _ = git.GitHubRemotes(path)
		}
		// An entry whose head no longer resolves has no row.
		var shown []Entry
		var heads []string
		for _, e := range entries {
			if err != nil {
				break
			}
			if hash, err := git.Run(path, "rev-parse", "--verify", "--quiet", e.Head+"^{commit}"); err == nil && hash != "" {
				e.Head = hash
				shown, heads = append(shown, e), append(heads, hash)
			}
		}
		signature, trees := refsSignature(refs, hidden, checkouts, shown), newRefTrees(refs)
		var commits []Commit
		var graph []graphRow
		var head string
		count := 0
		reread := err == nil && signature != known
		if reread {
			if commits, err = git.Commits(path, hidden, heads...); err == nil {
				count, head = len(commits), headCommit(commits)
				commits = withEntries(commits, shown)
				graph = buildGraph(commits)
			}
		}
		v.dispatch(func() {
			defer v.invalidate()
			if generation != v.repo.generation || v.closed {
				return
			}
			v.repo.loading, v.repo.loaded = false, true
			if v.repo.again {
				defer v.loadRepository()
			}
			if err != nil {
				v.repo.err = err.Error()
				v.repo.signature = ""
				return
			}
			v.repo.err, v.repo.refTrees, v.repo.checkouts, v.repo.github = "", trees, checkouts, github
			v.repo.entries = map[string]Entry{}
			for _, e := range shown {
				v.repo.entries[e.ID] = e
			}
			if reread {
				v.repo.signature, v.repo.commits, v.repo.count, v.repo.graph, v.repo.head = signature, commits, count, graph, head
				v.applyHistory()
			}
			v.followWatches(false)
		})
	}()
}

type errorText string

func (e errorText) Error() string { return string(e) }

// refsSignature describes every shown and hidden ref, checked out commit
// and entry, which together decide the history.
func refsSignature(refs, hidden Refs, checkouts []Checkout, entries []Entry) string {
	var b strings.Builder
	for _, list := range [][]Ref{refs.Branches, refs.Remotes, refs.Tags, hidden.Branches, hidden.Remotes, hidden.Tags} {
		for _, r := range list {
			b.WriteString(r.Name + " " + r.Hash + "\n")
		}
		b.WriteString("\n")
	}
	for _, c := range checkouts {
		b.WriteString(c.Head + "\n")
	}
	b.WriteString("\n")
	for _, e := range entries {
		b.WriteString(e.ID + " " + e.Title + " " + e.Head + "\n")
	}
	return b.String()
}

// withEntries inserts each entry's row right above the commit it names,
// which the row descends from in the graph.
func withEntries(commits []Commit, entries []Entry) []Commit {
	if len(entries) == 0 {
		return commits
	}
	above := map[string][]Commit{}
	for _, e := range entries {
		above[e.Head] = append(above[e.Head], Commit{Hash: entryPrefix + e.ID, Parents: []string{e.Head}, Subject: e.Title})
	}
	all := make([]Commit, 0, len(commits)+len(entries))
	for _, c := range commits {
		all = append(all, above[c.Hash]...)
		all = append(all, c)
	}
	return all
}

// Layout builds the view: the refs pane beside the chosen section, and the
// view's dialogs. It fills the space it is given.
func (v *View) Layout(c *ui.Context) {
	if v.toast != nil {
		c.AddToast(*v.toast)
		v.toast = nil
	}
	if v.path == "" {
		return
	}
	refs := func() {
		ui.Column(c).Fill().MinHeight(0).Children(func() { v.refsPane(c) })
	}
	section := func() {
		ui.Column(c).Fill().MinHeight(0).MinWidth(0).Children(func() {
			switch v.section {
			case sectionCommits:
				v.historyView(c)
			default:
				v.filesView(c, v.current, v.text("no_changes"))
			}
			v.overlay(c)
		})
	}
	split := components.Split(c, &v.state.RefsWidth, refs, section, components.SplitOptions{
		FirstMin: refsMinWidth, SecondMin: sectionMinWidth, Default: refsWidth,
	})
	split.Fill().MinHeight(0).MinWidth(0)
	if split.Changed() {
		v.state.RefsWidth = min(v.state.RefsWidth, refsMaxWidth)
		v.saveState()
	}
	v.syncDialogView(c)
	v.refDeletionDialog(c)
}

// overlay floats the host's overlay at the top of the section, measuring
// it so the history keeps its first rows clear.
func (v *View) overlay(c *ui.Context) {
	if v.opts.Overlay == nil {
		return
	}
	ui.Column(c).Key("overlay").Absolute().Attach(ui.AnchorTop, ui.AnchorTop).FillWidth().MinWidth(0).
		Draw(func(_ *ui.Painter, r ui.Rect) {
			if v.overlayHeight != r.H {
				v.overlayHeight = r.H
				c.Invalidate()
			}
		}).Children(func() { v.opts.Overlay(c) })
}

func (v *View) dialogInset(c *ui.Context) float32 {
	if v.opts.DialogInset != nil {
		return v.opts.DialogInset(c)
	}
	return 0
}

// refsPane leads to the worktree's changes and the history, and lists the
// branches, remote branches and tags in compact rows above their filter.
func (v *View) refsPane(c *ui.Context) {
	t := c.Theme()
	components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Gap(1).Padding(t.Space(1.5), t.Space(2)).Children(func() {
			if v.hasCurrentChanges() {
				if sidebarItem(c, v.text("current_changes"), lucide.FileDiff(), v.section == sectionChanges, lineCounts(v.current.changes.Files)).Clicked() {
					v.section = sectionChanges
					v.loadChanges()
					v.targetChanged()
				}
			}
			var count func(*ui.Context, float32)
			if n := v.repo.count; n > 0 {
				count = func(c *ui.Context, _ float32) {
					components.Text(c, strconv.Itoa(n)).FontSize(11.5).Shrink(0).TextColor(t.TextMuted).SingleLine()
				}
			}
			if sidebarItem(c, v.text("all_commits"), lucide.GitGraph(), v.section == sectionCommits && v.ref == "", count).Clicked() {
				v.section, v.ref = sectionCommits, ""
				v.targetChanged()
			}
			trees, query := v.repo.refTrees, strings.TrimSpace(v.refFilter)
			groups := []struct {
				key, label string
				icon       *ui.SVG
				open       bool
				nodes      []*refNode
			}{
				{"branches", v.text("branches"), lucide.GitBranch(), true, filterRefs(trees.branches, query)},
				{"remotes", v.text("remote_branches"), lucide.Cloud(), false, filterRefs(trees.remotes, query)},
				{"tags", v.text("tags"), lucide.Tag(), false, filterRefs(trees.tags, query)},
			}
			found := false
			for _, g := range groups {
				// A filter hides the groups it empties.
				if query == "" || len(g.nodes) > 0 {
					found = true
					v.refGroup(c, g.key, g.label, g.icon, g.open, query != "", g.nodes)
				}
			}
			if !found {
				components.Text(c, v.text("no_results")).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted).Padding(t.Space(1), t.Space(2))
			}
		})
	})
	// The ref filter stays at the bottom while the refs scroll.
	ui.Column(c).Key("ref-filter").FillWidth().Shrink(0).Padding(t.Space(2)).Children(func() {
		FilterField(c, &v.refFilter, v.text("filter_refs"), v.text("filter"))
	})
}

// lineCounts shows the lines files add and delete in total, or is nil when
// they change none.
func lineCounts(files []FileChange) func(*ui.Context, float32) {
	added, deleted := 0, 0
	for _, f := range files {
		added += f.Added
		deleted += f.Deleted
	}
	if added == 0 && deleted == 0 {
		return nil
	}
	return func(c *ui.Context, _ float32) {
		t := c.Theme()
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
			if added > 0 {
				components.Text(c, "+"+strconv.Itoa(added)).FontSize(11.5).TextColor(t.Success).SingleLine()
			}
			if deleted > 0 {
				components.Text(c, "−"+strconv.Itoa(deleted)).FontSize(11.5).TextColor(t.Danger).SingleLine()
			}
		})
	}
}

// FilterField is a search field of a Git pane, exported so a host's project
// list can match the refs pane's filter.
func FilterField(c *ui.Context, value *string, label, placeholder string) {
	components.TextBox(c, value, components.TextBoxOptions{LeadingIcon: lucide.Search()}).
		FillWidth().Height(c.Theme().Space(7)).FontSize(12).
		Label(label).Placeholder(placeholder)
}

// treeRow is the compact height of the refs pane's tree rows.
func treeRow(c *ui.Context) float32 { return c.Theme().Space(6) }

// sidebarItem is a refs pane entry as compact as the tree rows below it,
// followed by trailing, such as a count, unless it is nil. Its icon lines
// up with the ref groups' icons, which follow their disclosure arrow.
func sidebarItem(c *ui.Context, label string, icon *ui.SVG, selected bool, trailing func(*ui.Context, float32)) ui.Element {
	t := c.Theme()
	return components.SidebarItemWithSlots(c, label, selected, func(c *ui.Context, _ float32) {
		components.Icon(c, icon).Size(t.Space(3.5), t.Space(3.5)).Shrink(0).TextColor(t.Accent)
	}, trailing).Height(treeRow(c)).Padding(0, t.Space(2), 0, t.Space(2)+arrowSpan(c)).Gap(t.Space(1.5)).FontSize(13)
}

// arrowSpan is the width a ref group's disclosure arrow and its gap add
// before the group's icon.
func arrowSpan(c *ui.Context) float32 { return c.Theme().Space(4) + c.Theme().Space(1.5) }

// refFolder is a collapsible refs pane row, led by icon in its text color
// unless it is nil: clicking anywhere on it opens or closes it.
func refFolder(c *ui.Context, key, label string, icon func() *ui.SVG, open *bool, rows func()) ui.Element {
	var leading func(*ui.Context, float32)
	if icon != nil {
		leading = func(c *ui.Context, _ float32) {
			size := c.Theme().Space(3)
			components.Icon(c, icon).Size(size, size)
		}
	}
	var row ui.Element
	ui.Column(c).Key("group-" + key).FillWidth().Shrink(0).Children(func() {
		row = components.TreeItemWithLeading(c, label, open, false, leading, rows).Height(treeRow(c))
	})
	// The row toggles itself on a double click.
	if row.Clicked() && !row.DoubleClicked() {
		*open = !*open
		c.Invalidate()
	}
	return row
}

// disclosure is the refs pane row key's disclosure, open at first when open
// is set.
func (v *View) disclosure(key string, open bool) *bool {
	state := v.groups[key]
	if state == nil {
		state = &open
		v.groups[key] = state
	}
	return state
}

// refGroup lists refs as a tree of their name's folders under a collapsible
// heading, led by icon, whose disclosure is remembered; choosing a ref
// selects its commit in the history. A filter shows the group and its
// folders open.
func (v *View) refGroup(c *ui.Context, key, label string, icon *ui.SVG, open, filtering bool, nodes []*refNode) {
	t := c.Theme()
	if saved, ok := v.state.Groups[key]; ok {
		open = saved
	}
	state := v.disclosure(key, open)
	if filtering {
		state = new(bool)
		*state = true
	}
	ui.Column(c).Key("group-" + key).FillWidth().Shrink(0).Children(func() {
		parts := components.Collapsible(c, label, state, components.CollapsibleOptions{ArrowPosition: components.ArrowLeft, Leading: func() {
			components.Icon(c, icon).Size(t.Space(3.5), t.Space(3.5)).Shrink(0).TextColor(t.Accent)
		}}, func() {
			components.Tree(c, func() { v.refRows(c, key, filtering, nodes) })
		})
		parts.Root.FillWidth()
		// The heading's icon, after its arrow, lines up with the entries'
		// icons above it.
		parts.Trigger.Height(treeRow(c)).Padding(0, t.Space(2)).Gap(t.Space(1.5)).
			FontSize(t.Rem(.8)).Bold().TextColor(t.TextMuted)
	})
	if !filtering && *state != open {
		groups := map[string]bool{}
		for k, open := range v.state.Groups {
			groups[k] = open
		}
		groups[key] = *state
		v.state.Groups = groups
		v.saveState()
	}
}

// currentRef reports whether the refs pane's ref in group is the
// worktree's: its branch, that branch's upstream or a tag at its commit.
func (v *View) currentRef(group string, n *refNode) bool {
	switch group {
	case "branches":
		return n.path == v.tracking.Branch
	case "remotes":
		return n.path == v.tracking.Upstream
	case "tags":
		return n.hash == v.repo.head
	}
	return false
}

func (v *View) refRows(c *ui.Context, group string, filtering bool, nodes []*refNode) {
	for _, n := range nodes {
		id := group + "/" + n.path
		if n.children == nil {
			ui.Column(c).Key("ref-" + id).FillWidth().Shrink(0).Children(func() {
				row := components.TreeItem(c, n.name, nil, v.section == sectionCommits && v.ref == id, nil).Height(treeRow(c))
				if v.currentRef(group, n) {
					row.Bold()
				}
				if n.name != n.path {
					components.Tooltip(c, row, n.path)
				}
				row.ContextMenu(func(m *ui.Menu) { v.refDeleteItem(m, group, n) })
				if row.Clicked() {
					v.revealCommit(n.hash)
					v.ref = id
				}
			})
			continue
		}
		// A folder's key ends in a slash, so it never meets a ref's.
		state := v.disclosure(id+"/", true)
		if filtering {
			state = new(bool)
			*state = true
		}
		// A remote's folder carries the mark of its host.
		var icon func() *ui.SVG
		if _, remote := v.repo.github[n.path]; remote && group == "remotes" {
			icon = remoteIcon(v.repo.github, n.path)
		}
		refFolder(c, id+"/", n.name, icon, state, func() { v.refRows(c, group, filtering, n.children) })
	}
}

// iconButton is a small square ghost button with a tooltip; active ones
// show their option is on.
func iconButton(c *ui.Context, label string, icon func() *ui.SVG, size float32, active bool) ui.Element {
	variant := components.ButtonGhost
	if active {
		variant = components.ButtonSecondary
	}
	b := components.Button(c, "", variant).Label(label).Size(size, size).Padding(0).Shrink(0).
		Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		components.Icon(c, icon).Size(16, 16).Shrink(0)
	})
	components.Tooltip(c, b, label)
	return b
}
