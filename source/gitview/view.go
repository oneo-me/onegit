// Package gitview is a reusable Git repository view: a refs pane with the
// worktree's uncommitted changes, the history and the branches, remote
// branches and tags, beside the chosen section, either the changed files or
// the revision graph with the selected commit's details. It also runs the
// Git writes its toolbar and its refs' and commits' menus ask for, with
// their dialogs.
//
// The host owns the window, the project list and the toolbar; the view owns
// everything inside its area. Git runs off the UI thread and results return
// through Options.Dispatch.
package gitview

import (
	"strconv"
	"strings"
	"sync"

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
	// OnWrite reports that a Git write the view ran, named op, finished in
	// the worktree at path, whether or not it failed.
	OnWrite func(op, path string)
	// OpenWorktree shows the worktree at path, as Git lists it, when the
	// user double-clicks the branch another worktree has checked out. Nil
	// leaves such branches alone.
	OpenWorktree func(path string)
	// Overlay builds content floating at the top of the section, such as a
	// command panel, whose rows the history keeps clear of.
	Overlay func(c *ui.Context)
	// SaveFile asks where to save data such as a patch or file under name,
	// writes it there and returns the chosen path, or "" when canceled. It
	// runs off the UI thread. Nil uses MyGo's save dialog.
	SaveFile func(title, name string, data []byte) (string, error)
	// OpenFile opens a file path with its default application, or a directory
	// in the file manager, as a file menu's Open File and Open Folder do; a
	// host whose Git.Host runs elsewhere decides what a path means. Nil
	// leaves those items out.
	OpenFile func(path string) error
	// RevealFile shows a file's or directory's path in the file manager, as a
	// file menu's Show in File Manager does; a host whose Git.Host runs
	// elsewhere decides what a path means. Nil leaves that item out.
	RevealFile func(path string)
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
	// "branches", "remotes", "tags" and "stashes".
	Groups map[string]bool `yaml:"groups,omitempty" json:"groups,omitempty"`
	// DiffWrap wraps diff and file lines.
	DiffWrap bool `yaml:"diffWrap,omitempty" json:"diffWrap,omitempty"`
	// UnstagedShare is the share, from 0 to 100, of a worktree's file
	// lists' height the unstaged files take; zero keeps the default.
	UnstagedShare float32 `yaml:"unstagedShare,omitempty" json:"unstagedShare,omitempty"`
	// FetchBranch, PullRebase, PullStash and PushTags are the fetch, pull
	// and push dialogs' remembered options; FetchBranch fetches the chosen
	// branch instead of every remote.
	FetchBranch bool `yaml:"fetchBranch,omitempty" json:"fetchBranch,omitempty"`
	PullRebase  bool `yaml:"pullRebase,omitempty" json:"pullRebase,omitempty"`
	PullStash   bool `yaml:"pullStash,omitempty" json:"pullStash,omitempty"`
	PushTags    bool `yaml:"pushTags,omitempty" json:"pushTags,omitempty"`
	// RebaseAutostash and RebaseUpdateRefs are the rebase dialog's
	// remembered options, and CherryPickAppend appends the origin of a
	// cherry-picked commit to its message.
	RebaseAutostash  bool `yaml:"rebaseAutostash,omitempty" json:"rebaseAutostash,omitempty"`
	RebaseUpdateRefs bool `yaml:"rebaseUpdateRefs,omitempty" json:"rebaseUpdateRefs,omitempty"`
	CherryPickAppend bool `yaml:"cherryPickAppend,omitempty" json:"cherryPickAppend,omitempty"`
	// StashTrackedOnly leaves untracked files out of a new stash.
	StashTrackedOnly bool `yaml:"stashTrackedOnly,omitempty" json:"stashTrackedOnly,omitempty"`
	// StashKeepApplied keeps a stash after applying it.
	StashKeepApplied bool `yaml:"stashKeepApplied,omitempty" json:"stashKeepApplied,omitempty"`
	// AutoFetch lists the remotes fetched when a worktree opens, by the
	// repository main worktree path.
	AutoFetch map[string][]string `yaml:"autoFetch,omitempty" json:"autoFetch,omitempty"`
	// Drafts holds the commit message typed for a worktree, by its path,
	// until it is committed or cleared.
	Drafts map[string]string `yaml:"drafts,omitempty" json:"drafts,omitempty"`
	// ShowReflog lists lost commits from the reflogs in the history.
	ShowReflog bool `yaml:"showReflog,omitempty" json:"showReflog,omitempty"`
	// MergeChoice remembers how the merge dialog records a merge.
	MergeChoice int `yaml:"mergeChoice,omitempty" json:"mergeChoice,omitempty"`
	// Pinned lists the refs pinned in the refs pane, by repository root, as
	// "branches/name", "remotes/name" or "tags/name".
	Pinned map[string][]string `yaml:"pinned,omitempty" json:"pinned,omitempty"`
}

// Entry is a row the host inserts into the history right above the commit
// Head names, which the row descends from in the graph. Its own commits,
// from Head down, join the history even when no shown ref reaches them.
type Entry struct {
	// ID tells entries apart; Title names the row and Icon leads it, unless
	// Lead draws the leading mark itself, such as an animated one.
	ID, Title string
	Icon      func() *ui.SVG
	Lead      func(c *ui.Context)
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
	// sectionStash lists the files a stash saved.
	sectionStash
	// sectionCompare lists the files two commits or a commit and the
	// worktree differ in.
	sectionCompare
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
	// operation is the Git operation that stopped in the worktree, such as
	// a conflicted merge, until the user completes or aborts it. amend
	// records that the commit box will replace the checked out commit, and
	// draftGeneration discards a draft save a newer edit replaced.
	operation       Operation
	amend           bool
	draftGeneration uint64
	// abortingOperation shows the dialog confirming the operation's abort.
	abortingOperation bool
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
	// some, and otherwise the history, after the view opened or moved to
	// another worktree, unless the user chose a section first.
	seekChanges bool

	repo repositoryState
	// current holds the worktree's staged and unstaged changes, and
	// messages the commit message typed for each worktree.
	current  *filePane
	messages map[string]string
	history  historyState
	// stash is the hash of the stash chosen in sectionStash, whose files
	// stashFiles lists.
	stash       string
	stashFiles  *filePane
	compare     compareState
	fileHistory fileHistoryDialog
	blame       blameDialog
	search      historySearchDialog
	interactive interactiveRebaseDialog
	bulk        bulkDialog
	// tracking compares the worktree's branch with its upstream; syncing
	// names the Git write running in each worktree, and toast reports the
	// last one that failed; dialog chooses a fetch's, pull's or push's branches.
	tracking           Tracking
	trackingGeneration uint64
	syncing            map[string]string
	dialog             syncDialog
	stashDialog        stashDialog
	applyStash         applyStash
	toast              *ui.Toast
	// deletion and discarding confirm deleting a ref from the refs pane and
	// discarding a worktree's changes; discardingChange discards part of a
	// file, and restoring resets a file to a commit; action asks for what
	// another write of a ref's or commit's menu needs.
	deletion   refDeletion
	discarding discarding
	// discardChange asks before discarding a hunk or selected lines.
	discardChange discardChange
	restoring     restoring
	action        refAction
	// rebase, reset, cherryPick and revert are the history's dialogs, and
	// pushTag the tag menu's; stashRename and stashDeletion change a stash.
	rebase        rebaseDialog
	reset         resetDialog
	cherryPick    cherryPickDialog
	revert        revertDialog
	pushTag       pushTagDialog
	stashRename   stashRenameDialog
	stashDeletion stashDeletionDialog
	// remoteDialog adds or edits a remote, remoteDeletion confirms deleting
	// one, and stale chooses stale local branches to delete.
	remoteDialog   remoteDialog
	remoteDeletion remoteDeletion
	stale          staleBranches
	// autoFetchPending fetches the remotes chosen for automatic fetching
	// once the repository read after SetPath finishes.
	autoFetchPending bool
	// watches follow the repository's Git directory and shown worktrees.
	watches map[string]*gitWatch
	closed  bool
	// tempMu guards tempDirs, the temporary directories holding versions of
	// files opened from commits or stashes.
	tempMu   sync.Mutex
	tempDirs []string
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
	// head is the commit the worktree has checked out, and root the
	// repository main worktree path, shared by all of its worktrees.
	head      string
	root      string
	refTrees  refTrees
	stashes   []Stash
	checkouts []Checkout
	// remoteURLs maps each remote to its fetch URL, and github to whether it
	// is hosted on GitHub.
	remoteURLs map[string]string
	github     map[string]bool
	operation  Operation
	// entries are the host's rows by ID.
	entries map[string]Entry
}

// New creates a view showing nothing until SetPath.
func New(opts Options) *View {
	v := &View{opts: opts, git: opts.Git, state: opts.State, section: sectionCommits, groups: map[string]*bool{}, messages: map[string]string{}, syncing: map[string]string{}}
	for path, message := range opts.State.Drafts {
		v.messages[path] = message
	}
	if v.state.RefsWidth <= 0 {
		v.state.RefsWidth = refsWidth
	}
	v.state.RefsWidth = min(v.state.RefsWidth, refsMaxWidth)
	if v.state.TreeWidth <= 0 {
		v.state.TreeWidth = treeWidth
	}
	if v.state.UnstagedShare <= 0 {
		v.state.UnstagedShare = unstagedShare
	}
	if opts.AvatarDir != "" {
		v.avatars = newAvatarCache(opts.AvatarDir)
	}
	v.current = v.newFilePane(false)
	v.stashFiles = v.newFilePane(false)
	v.compare.files = v.newFilePane(false)
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
	v.saveDraft()
	v.path = path
	v.repo.generation++
	v.repo = repositoryState{generation: v.repo.generation}
	v.autoFetchPending = path != ""
	v.clearFiles(v.current, "")
	v.current.key = ""
	v.seekChanges = true
	v.clearHistory()
	v.ref = ""
	v.clearStash()
	v.clearCompare()
	v.tracking = Tracking{}
	v.amend, v.abortingOperation = false, false
	v.fileHistory.open, v.blame.open, v.search.open = false, false, false
	v.interactive.open, v.bulk.open = false, false
	v.discardChange = discardChange{}
	v.remoteDialog.open, v.remoteDeletion.open = false, false
	v.stale = staleBranches{generation: v.stale.generation + 1}
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
	if v.section == sectionCompare {
		v.loadCompare()
	}
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
	v.saveDraft()
	v.closed = true
	v.stopWatches()
	v.removeTempDirs()
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
// reading them failed.
func (v *View) hasCurrentChanges() bool {
	p := v.current
	return v.path != "" && p.loaded && (p.err != "" || len(p.changes.Files) > 0)
}

// seekSection shows the worktree's changes once first read after the view
// opened or moved when it found some, and otherwise the history,
// reporting whether the section changed. Later reads leave the section to
// the user, so emptied changes stay shown.
func (v *View) seekSection() bool {
	if !v.seekChanges {
		return false
	}
	v.seekChanges = false
	want := sectionCommits
	if v.current.err == "" && len(v.current.changes.Files) > 0 {
		want = sectionChanges
	}
	if v.section == want {
		return false
	}
	v.section, v.ref = want, ""
	return true
}

// loadChanges rereads the worktree's staged and unstaged files.
func (v *View) loadChanges() {
	path := v.path
	if path == "" {
		v.clearFiles(v.current, "")
		v.current.key = ""
		return
	}
	v.loadFiles(v.current, path, func(git Git) (Changes, error) { return git.WorktreeStatus(path) })
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
	reflog := v.state.ShowReflog
	var entries []Entry
	if v.opts.Entries != nil {
		entries = v.opts.Entries()
	}
	noHistory := v.text("no_history")
	v.repo.loading = true
	go func() {
		var refs, hidden Refs
		var checkouts []Checkout
		var stashes []Stash
		var remoteURLs map[string]string
		var operation Operation
		github := map[string]bool{}
		_, err := git.Top(path)
		if err == nil {
			// A stopped operation changes the meaning of the worktree even
			// when its refs did not change.
			operation, _ = git.Operation(path)
			if refs, err = git.Refs(path); err == nil {
				refs, hidden = refs.split(keep)
				checkouts, err = git.Checkouts(path)
			}
			if err == nil {
				stashes, err = git.Stashes(path)
			}
		} else {
			err = errorText(noHistory)
		}
		if err == nil {
			// Without them, remotes just lose their GitHub mark.
			remoteURLs, _ = git.RemoteURLs(path)
			for name, url := range remoteURLs {
				github[name] = onGitHub(url)
			}
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
		signature, trees := refsSignature(refs, hidden, checkouts, shown, reflog), newRefTrees(refs)
		for remote := range remoteURLs {
			trees.remotes = addRefFolder(trees.remotes, remote)
		}
		var commits []Commit
		var graph []graphRow
		var head string
		count := 0
		reread := err == nil && signature != known
		if reread {
			if reflog {
				if commits, err = git.Reflog(path); err == nil {
					count, head = len(commits), headCommit(commits)
					graph = buildGraph(commits)
				}
			} else if commits, err = git.Commits(path, hidden, heads...); err == nil {
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
			if len(checkouts) > 0 {
				v.repo.root = checkouts[0].Path
			}
			if v.autoFetchPending {
				v.autoFetchPending = false
				defer v.fetchAutomatic()
			}
			if err != nil {
				v.repo.err = err.Error()
				v.repo.signature = ""
				return
			}
			v.repo.err, v.repo.refTrees, v.repo.checkouts, v.repo.stashes = "", trees, checkouts, stashes
			v.repo.remoteURLs, v.repo.github = remoteURLs, github
			v.repo.operation = operation
			v.keepStash()
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
func refsSignature(refs, hidden Refs, checkouts []Checkout, entries []Entry, reflog bool) string {
	var b strings.Builder
	if reflog {
		b.WriteString("reflog\n")
	}
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
	if c.Shortcut(v.shortcut()|ui.Shift, ui.KeyF) {
		v.openHistorySearch()
	}
	refs := func() {
		ui.Column(c).Fill().MinHeight(0).Children(func() { v.refsPane(c) })
	}
	section := func() {
		ui.Column(c).Fill().MinHeight(0).MinWidth(0).Children(func() {
			v.operationBanner(c)
			switch v.section {
			case sectionCommits:
				v.historyView(c)
			case sectionStash:
				v.filesView(c, v.stashFiles, v.text("no_changes"))
			case sectionCompare:
				v.compareView(c)
			default:
				v.filesView(c, v.current, "")
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
	v.stashDialogView(c)
	v.applyStashDialog(c)
	v.stashRenameDialogView(c)
	v.stashDeletionDialogView(c)
	v.refDeletionDialog(c)
	v.discardDialog(c)
	v.discardChangeDialog(c)
	v.restoreFileDialog(c)
	v.abortOperationDialog(c)
	v.refActionDialog(c)
	v.remoteDialogView(c)
	v.remoteDeletionDialog(c)
	v.staleBranchesDialogView(c)
	v.rebaseDialogView(c)
	v.resetDialogView(c)
	v.cherryPickDialogView(c)
	v.revertDialogView(c)
	v.pushTagDialogView(c)
	v.fileHistoryDialogView(c)
	v.blameDialogView(c)
	v.historySearchDialogView(c)
	v.interactiveRebaseDialogView(c)
	v.bulkDialogView(c)
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
			if sidebarItem(c, v.text("current_changes"), lucide.FileDiff(), v.section == sectionChanges, lineCounts(v.current.changes.Files)).Clicked() {
				v.section, v.seekChanges = sectionChanges, false
				v.loadChanges()
				v.targetChanged()
			}
			var count func(*ui.Context, float32)
			if n := v.repo.count; n > 0 {
				count = func(c *ui.Context, _ float32) {
					components.Text(c, strconv.Itoa(n)).FontSize(11.5).Shrink(0).TextColor(t.TextMuted).SingleLine()
				}
			}
			label := v.text("all_commits")
			if v.state.ShowReflog {
				label = v.text("all_commits_reflog")
			}
			all := sidebarItem(c, label, lucide.GitGraph(), v.section == sectionCommits && v.ref == "", count)
			all.ContextMenu(func(m *ui.Menu) {
				if m.Item(v.text("search_history")).Shortcut(v.shortcut()|ui.Shift, ui.KeyF).Chosen() {
					v.openHistorySearch()
				}
				if m.Item(v.text("show_reflog")).Checked(v.state.ShowReflog).Chosen() {
					v.state.ShowReflog = !v.state.ShowReflog
					v.saveState()
					v.loadRepository()
					v.invalidate()
				}
			})
			if all.Clicked() {
				v.section, v.ref, v.seekChanges = sectionCommits, "", false
				v.targetChanged()
			}
			if v.compare.from != "" {
				row := sidebarItem(c, v.text("compare_refs"), lucide.GitCompare(), v.section == sectionCompare, nil)
				components.Tooltip(c, row, v.compareLabel())
				if row.Clicked() {
					v.section, v.ref, v.seekChanges = sectionCompare, "", false
					v.targetChanged()
				}
			}
			trees, query := v.repo.refTrees, strings.TrimSpace(v.refFilter)
			found := false
			if pinned := v.pinnedRefs(); len(pinned) > 0 {
				visible := false
				if query == "" {
					visible = true
				} else {
					for _, ref := range pinned {
						if strings.Contains(strings.ToLower(ref.name), strings.ToLower(query)) {
							visible = true
							break
						}
					}
				}
				if visible {
					found = true
					v.refGroup(c, "pinned", v.text("pinned_refs"), lucide.Pin(), true, query != "", nil, func() { v.pinnedRows(c, query) })
				}
			}
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
			for _, g := range groups {
				// A filter hides the groups it empties.
				if query == "" || len(g.nodes) > 0 {
					found = true
					var menu func(*ui.Menu)
					switch g.key {
					case "branches":
						menu = v.branchGroupMenu
					case "remotes":
						menu = v.remoteGroupMenu
					case "tags":
						menu = v.tagGroupMenu
					}
					v.refGroup(c, g.key, g.label, g.icon, g.open, query != "", menu, func() { v.refRows(c, g.key, query != "", g.nodes) })
				}
			}
			// Stashes show only when there are some.
			if stashes := filterStashes(v.repo.stashes, query); len(stashes) > 0 {
				found = true
				v.refGroup(c, "stashes", v.text("stashes"), lucide.Archive(), true, query != "", nil, func() { v.stashRows(c, stashes) })
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
// unless it is nil: clicking anywhere on it opens or closes it, and menu
// fills its context menu unless it is nil.
func refFolder(c *ui.Context, key, label string, icon func() *ui.SVG, open *bool, menu func(*ui.Menu), rows func()) ui.Element {
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
	if menu != nil {
		row.ContextMenu(menu)
	}
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

// refGroup lists rows under a collapsible heading, led by icon, whose
// disclosure is remembered. A filter shows the group open, and menu fills
// the heading's context menu unless it is nil.
func (v *View) refGroup(c *ui.Context, key, label string, icon *ui.SVG, open, filtering bool, menu func(*ui.Menu), rows func()) {
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
			components.Tree(c, rows)
		})
		parts.Root.FillWidth()
		// The heading's icon, after its arrow, lines up with the entries'
		// icons above it.
		parts.Trigger.Height(treeRow(c)).Padding(0, t.Space(2)).Gap(t.Space(1.5)).
			FontSize(t.Rem(.8)).Bold().TextColor(t.TextMuted)
		if menu != nil {
			parts.Trigger.ContextMenu(menu)
		}
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

// refRows lists refs as a tree of their name's folders; choosing a ref
// selects its commit in the history. A filter shows the folders open.
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
				row.ContextMenu(func(m *ui.Menu) { v.refMenu(c, m, nodeTarget(group, n), n) })
				if row.Clicked() {
					v.revealCommit(n.hash)
					v.ref = id
				}
				if row.DoubleClicked() {
					v.openCheckout(nodeTarget(group, n))
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
		// A remote's folder carries the mark of its host and its menu.
		var icon func() *ui.SVG
		var menu func(*ui.Menu)
		if _, remote := v.repo.remoteURLs[n.path]; remote && group == "remotes" {
			icon = remoteIcon(v.repo.github, n.path)
			menu = func(m *ui.Menu) { v.remoteMenu(c, m, n.path) }
		}
		refFolder(c, id+"/", n.name, icon, state, menu, func() { v.refRows(c, group, filtering, n.children) })
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
