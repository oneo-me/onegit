// Package gitview is a Git repository view: a refs pane with the
// worktree's uncommitted changes, the history and the branches, remote
// branches and tags, beside the chosen section, either the changed files or
// the revision graph with the selected commit's details. It also runs the
// Git writes its toolbar and its refs' and commits' menus ask for, with
// their dialogs.
//
// The host owns the window, the project list and the toolbar's place; the
// view owns everything inside its area. Git runs off the engine and results
// return through Options.Dispatch.
package gitview

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Default widths of the refs pane and the file trees, and their limits; the
// section keeps at least sectionMinWidth.
const (
	refsWidth, refsMinWidth, refsMaxWidth, sectionMinWidth = 220, 160, 400, 320
	treeWidth                                              = 260
)

// Options configure a View. Dispatch is required; the rest is optional.
type Options struct {
	// Dispatch runs fn on the engine's goroutine, as Scope.Post does.
	Dispatch func(fn func())
	// Invalidate hears every change the view shows.
	Invalidate func()
	// Git runs the git executable, such as with a login environment.
	Git Git
	// Language picks the built-in strings: a Chinese tag such as "zh-Hans"
	// picks Chinese, anything else English.
	Language string
	// AvatarDir caches commit authors' GitHub or Gravatar pictures; empty
	// draws initials only and never downloads.
	AvatarDir string
	// RefFilter keeps the refs it accepts, hiding the others from the refs
	// pane and the history, along with the commits only they reach. Nil
	// keeps every ref.
	RefFilter func(kind RefKind, name string) bool
	// State restores the remembered layout and choices; OnState receives
	// them whenever the user changes them.
	State   State
	OnState func(State)
	// OnWrite reports that a Git write the view ran, named op, finished in
	// the worktree at path, whether or not it failed.
	OnWrite func(op, path string)
	// OpenWorktree shows the worktree at path, as Git lists it, when the
	// user double-clicks the branch another worktree has checked out. Nil
	// leaves such branches alone.
	OpenWorktree func(path string)
	// Worktrees lists the repository's worktrees below the refs pane's
	// entries, above its refs, when it has more than one, the shown one in
	// bold; double-clicking another goes to OpenWorktree.
	Worktrees bool
	// SaveFile asks where to save data such as a patch or file under name,
	// writes it there and returns the chosen path, or "" when canceled. It
	// runs off the engine. Nil uses the system's save dialog.
	SaveFile func(title, name string, data []byte) (string, error)
	// OpenFile opens a file path with its default application, or a
	// directory in the file manager, as a file menu's Open File and Open
	// Folder do. It runs off the engine; nil leaves those items out.
	OpenFile func(path string) error
	// RevealFile shows a file's or directory's path in the file manager, as
	// a file menu's Show in File Manager does. It runs off the engine; nil
	// leaves that item out.
	RevealFile func(path string)
	// Toaster shows the view's failures as toasts.
	Toaster *ui.Toaster
	// DialogInset keeps the view's dialogs below a host title bar.
	DialogInset core.Points
}

// State is the view's remembered layout and choices.
type State struct {
	// RefsWidth is the refs pane's width and TreeWidth that of the file
	// trees; zero keeps the default.
	RefsWidth float32 `yaml:"refsWidth,omitempty" json:"refsWidth,omitempty"`
	TreeWidth float32 `yaml:"treeWidth,omitempty" json:"treeWidth,omitempty"`
	// Groups holds the disclosure of the refs pane's groups by key:
	// "pinned", "branches", "remotes", "tags" and "stashes", and of their
	// folders by group and path ending in a slash, such as
	// "branches/feature/". It keeps only what differs from the default.
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
	// rev counts the changes the view shows and edits the changes its
	// dialogs' fields make, while the view is built; cx is where it is.
	rev, edits *core.Signal[int]
	cx         *core.Scope
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
	// ref is the refs pane's row of the branch, remote branch or tag last
	// chosen in sectionCommits.
	ref string
	// refFilter is the text that narrows the refs pane's refs.
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
	// names the Git write running in each worktree, and lastToast is the
	// last failure shown; dialog chooses a fetch's, pull's or push's
	// branches.
	tracking           Tracking
	trackingGeneration uint64
	syncing            map[string]string
	dialog             syncDialog
	stashDialog        stashDialog
	applyStash         applyStash
	lastToast          toastContent
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
	closed           bool
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
	// commits holds the history.
	commits []Commit
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
}

// New creates a view showing nothing until SetPath.
func New(opts Options) *View {
	v := &View{opts: opts, git: opts.Git, state: opts.State, section: sectionCommits, messages: map[string]string{}, syncing: map[string]string{}}
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

func (v *View) saveState() {
	if v.opts.OnState != nil {
		v.opts.OnState(v.state)
	}
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

// Close drops the view's pending results and its temporary files.
func (v *View) Close() {
	v.saveDraft()
	v.closed = true
	v.removeTempDirs()
}

// Selected names the selected commit while the history shows, or is empty.
func (v *View) Selected() string {
	if v.section != sectionCommits {
		return ""
	}
	return v.history.selected
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

// targetChanged rereads the branch's tracking after the shown section, row
// or worktree changed.
func (v *View) targetChanged() {
	v.loadTracking()
	v.invalidate()
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
// changed.
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
		signature, trees := refsSignature(refs, hidden, checkouts, reflog), newRefTrees(refs)
		for remote := range remoteURLs {
			trees.remotes = addRefFolder(trees.remotes, remote)
		}
		var commits []Commit
		var head string
		reread := err == nil && signature != known
		if reread {
			if reflog {
				commits, err = git.Reflog(path)
			} else {
				commits, err = git.Commits(path, hidden)
			}
			head = headCommit(commits)
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
			if reread {
				v.repo.signature, v.repo.commits, v.repo.head = signature, commits, head
				v.applyHistory()
			}
		})
	}()
}

type errorText string

func (e errorText) Error() string { return string(e) }

// refsSignature describes every shown and hidden ref and checked out
// commit, which together decide the history.
func refsSignature(refs, hidden Refs, checkouts []Checkout, reflog bool) string {
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
	return b.String()
}

// Bind gives the view the scope its state lives in, which outlives every
// place it shows, such as the app's; Node and Toolbar bind to theirs
// otherwise.
func (v *View) Bind(cx *core.Scope) {
	if v.rev != nil {
		return
	}
	v.cx = cx
	v.rev, v.edits = core.NewSignal(cx, 0), core.NewSignal(cx, 0)
	cx.Cleanup(func() {
		if v.cx == cx {
			v.rev, v.edits, v.cx = nil, nil, nil
		}
	})
}

// Node is the view: the refs pane beside the chosen section, and the view's
// dialogs. It fills the space it is given. Build it once; it follows the
// view's changes.
func (v *View) Node() core.Node { return core.Func(v.render) }

func (v *View) render(cx *core.Scope) core.Node {
	v.Bind(cx)
	cx.OnKey("mod+shift+f", v.openHistorySearch)
	language := core.NewMemo(cx, func() string { v.track(); return v.opts.Language })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		// The words change with the language, which builds the view again.
		language.Get()
		var node core.Node
		cx.Untrack(func() {
			node = core.Show{When: func() bool { v.track(); return v.path != "" }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: ui.FillRest, Children: []core.Node{v.split(cx), v.dialogs(cx)}}
			}}
		})
		return node
	})
}

// split puts the refs pane beside the section, the user dragging the line
// between them.
func (v *View) split(cx *core.Scope) core.Node {
	return ui.ResizablePanelGroup{Axis: base.Horizontal, Style: ui.FillRest,
		Panels: []base.ResizablePanel{
			{Size: func() core.Points { return core.Points(v.state.RefsWidth) }, MinSize: refsMinWidth, MaxSize: refsMaxWidth,
				OnResize: func(size core.Points) {
					v.state.RefsWidth = min(float32(size), refsMaxWidth)
					v.saveState()
				},
				Style: core.Style{MinHeight: core.Pt(0)}, Children: []core.Node{v.refsPane(cx)}},
			{MinSize: sectionMinWidth, Style: core.Style{MinHeight: core.Pt(0), MinWidth: core.Pt(0)}, Children: []core.Node{v.sectionView(cx)}},
		}}
}

// sectionView shows the stopped operation above the chosen section.
func (v *View) sectionView(cx *core.Scope) core.Node {
	shown := core.NewMemo(cx, func() section { v.track(); return v.section })
	return core.View{Style: ui.FillRest, Children: []core.Node{
		v.operationBanner(cx),
		core.Dynamic(func(cx *core.Scope) core.Node {
			s := shown.Get()
			var node core.Node
			cx.Untrack(func() {
				switch s {
				case sectionCommits:
					node = v.historyView(cx)
				case sectionStash:
					node = v.filesView(cx, v.stashFiles, v.text("no_changes"))
				case sectionCompare:
					node = v.compareView(cx)
				default:
					node = v.filesView(cx, v.current, "")
				}
			})
			return node
		}),
	}}
}

// dialogs are the view's dialogs, each shown while open.
func (v *View) dialogs(cx *core.Scope) core.Node {
	return core.Group{
		v.syncDialogView(cx), v.stashDialogView(cx), v.applyStashDialog(cx), v.stashRenameDialogView(cx),
		v.stashDeletionDialogView(cx), v.refDeletionDialog(cx), v.discardDialog(cx), v.discardChangeDialog(cx),
		v.restoreFileDialog(cx), v.abortOperationDialog(cx), v.refActionDialog(cx), v.remoteDialogView(cx),
		v.remoteDeletionDialog(cx), v.staleBranchesDialogView(cx), v.rebaseDialogView(cx), v.resetDialogView(cx),
		v.cherryPickDialogView(cx), v.revertDialogView(cx), v.pushTagDialogView(cx), v.fileHistoryDialogView(cx),
		v.blameDialogView(cx), v.historySearchDialogView(cx), v.interactiveRebaseDialogView(cx), v.bulkDialogView(cx),
	}
}

// refsKey says what the refs pane's rows show, so it is built again only
// when they change; selection and bold rows follow on their own.
func (v *View) refsKey() string {
	v.track()
	var b strings.Builder
	fmt.Fprint(&b, v.opts.Language, v.path, "\x00", v.state.ShowReflog, len(v.repo.commits), v.compare.from != "", "\x00", v.refFilter, "\x00")
	added, deleted := lineTotals(v.current.changes.Files)
	fmt.Fprint(&b, added, ",", deleted, "\x00")
	for _, list := range [][]*refNode{v.repo.refTrees.branches, v.repo.refTrees.remotes, v.repo.refTrees.tags} {
		for _, n := range refLeaves(list) {
			fmt.Fprint(&b, n.path, " ", n.hash, " ", n.upstream, n.ahead, ",", n.behind, "\n")
		}
		b.WriteString("\x00")
	}
	for _, s := range v.repo.stashes {
		b.WriteString(s.Hash + s.Message + "\n")
	}
	for _, c := range v.repo.checkouts {
		b.WriteString(c.Path + c.Branch + c.Head + "\n")
	}
	for name := range v.repo.github {
		b.WriteString(name + "\n")
	}
	fmt.Fprint(&b, v.state.Pinned[v.pinnedRoot()])
	return b.String()
}

// refsPane leads to the worktree's changes and the history, and lists the
// branches, remote branches and tags in compact rows above their filter.
func (v *View) refsPane(cx *core.Scope) core.Node {
	rows := region(cx, v.refsKey, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 1, Padding: core.Symmetric(6, 8), FlexShrink: 0}, Children: v.refRowsAll(cx)}
	})
	filter := FilterField{Value: func() string { v.track(); return v.refFilter }, OnChange: func(s string) {
		v.refFilter = s
		v.invalidate()
	}, Label: v.text("filter_refs"), Placeholder: v.text("filter"), TestID: "ref-filter"}
	return core.View{Style: ui.FillRest, Children: []core.Node{
		ui.ScrollView{Style: ui.FillRest, Children: []core.Node{rows}},
		// The ref filter stays at the bottom while the refs scroll.
		core.View{Style: core.Style{Padding: core.All(8), FlexShrink: 0}, Children: []core.Node{filter}},
	}}
}

// refRowsAll lists the refs pane's rows as the view holds them now.
func (v *View) refRowsAll(cx *core.Scope) []core.Node {
	colors := ui.ColorsOf(cx)
	trees, query := v.repo.refTrees, strings.TrimSpace(v.refFilter)
	var nodes []core.Node
	found := false
	nodes = append(nodes, v.sidebarItem(cx, v.text("current_changes"), lucide.FileDiff,
		func() bool { return v.section == sectionChanges }, lineCounts(cx, v.current.changes.Files), func(core.PressEvent) {
			v.section, v.seekChanges = sectionChanges, false
			v.loadChanges()
			v.targetChanged()
		}, nil, "current-changes"))
	var count core.Node
	if n := len(v.repo.commits); n > 0 {
		count = line(strconv.Itoa(n), 11.5, ui.Regular, colors.Muted)
	}
	label := v.text("all_commits")
	if v.state.ShowReflog {
		label = v.text("all_commits_reflog")
	}
	nodes = append(nodes, v.sidebarItem(cx, label, lucide.GitGraph, func() bool { return v.section == sectionCommits && v.ref == "" }, count,
		func(core.PressEvent) {
			v.section, v.ref, v.seekChanges = sectionCommits, "", false
			v.targetChanged()
		}, func() []core.MenuItem {
			g := &menuGroups{}
			g.shortcut(v.text("search_history"), "mod+shift+f", false, v.openHistorySearch)
			g.check(v.text("show_reflog"), v.state.ShowReflog, false, func() {
				v.state.ShowReflog = !v.state.ShowReflog
				v.saveState()
				v.loadRepository()
			})
			return g.items
		}, "all-commits"))
	if v.compare.from != "" {
		nodes = append(nodes, ui.Tooltip{Text: v.compareLabel(), Trigger: v.sidebarItem(cx, v.text("compare_refs"), lucide.GitCompare,
			func() bool { return v.section == sectionCompare }, nil, func(core.PressEvent) {
				v.section, v.ref, v.seekChanges = sectionCompare, "", false
				v.targetChanged()
			}, nil, "compare")})
	}
	// A repository's only worktree needs no list.
	if v.opts.Worktrees && len(v.repo.checkouts) > 1 {
		if checkouts := filterCheckouts(v.repo.checkouts, query); len(checkouts) > 0 {
			found = true
			nodes = append(nodes, v.refGroup(cx, "worktrees", v.text("worktrees"), lucide.FolderGit2, true, query != "", nil, v.worktreeRows(cx, checkouts)))
		}
	}
	if pinned := v.pinnedRefs(); len(pinned) > 0 {
		visible := query == ""
		for _, ref := range pinned {
			visible = visible || strings.Contains(strings.ToLower(ref.name), strings.ToLower(query))
		}
		if visible {
			found = true
			nodes = append(nodes, v.refGroup(cx, "pinned", v.text("pinned_refs"), lucide.Pin, true, query != "", nil, v.pinnedRows(cx, query)))
		}
	}
	groups := []struct {
		key, label string
		icon       func() *core.SVG
		open       bool
		nodes      []*refNode
		menu       func() []core.MenuItem
	}{
		{"branches", v.text("branches"), lucide.GitBranch, true, filterRefs(trees.branches, query), v.branchGroupMenu},
		{"remotes", v.text("remote_branches"), lucide.Cloud, false, filterRefs(trees.remotes, query), v.remoteGroupMenu},
		{"tags", v.text("tags"), lucide.Tag, false, filterRefs(trees.tags, query), v.tagGroupMenu},
	}
	for _, g := range groups {
		// A filter hides the groups it empties.
		if query == "" || len(g.nodes) > 0 {
			found = true
			nodes = append(nodes, v.refGroup(cx, g.key, g.label, g.icon, g.open, query != "", g.menu, v.refRows(cx, g.key, query != "", g.nodes, 0)))
		}
	}
	// Stashes show only when there are some.
	if stashes := filterStashes(v.repo.stashes, query); len(stashes) > 0 {
		found = true
		nodes = append(nodes, v.refGroup(cx, "stashes", v.text("stashes"), lucide.Archive, true, query != "", nil, v.stashRows(cx, stashes)))
	}
	if !found {
		nodes = append(nodes, core.View{Style: core.Style{Padding: core.Symmetric(4, 8)}, Children: []core.Node{muted(cx, v.text("no_results"), 12)}})
	}
	return nodes
}

// lineTotals counts the lines files add and delete in total.
func lineTotals(files []FileChange) (added, deleted int) {
	for _, f := range files {
		added += f.Added
		deleted += f.Deleted
	}
	return added, deleted
}

// lineCounts shows the lines files add and delete in total, or is nil when
// they change none.
func lineCounts(cx *core.Scope, files []FileChange) core.Node {
	added, deleted := lineTotals(files)
	if added == 0 && deleted == 0 {
		return nil
	}
	colors := ui.ColorsOf(cx)
	var nodes []core.Node
	if added > 0 {
		nodes = append(nodes, line("+"+strconv.Itoa(added), 11.5, ui.Regular, colors.Up))
	}
	if deleted > 0 {
		nodes = append(nodes, line("−"+strconv.Itoa(deleted), 11.5, ui.Regular, colors.Danger))
	}
	r := row(4, nodes...)
	r.Style.FlexShrink = 0
	return r
}

// FilterField is a search field of a Git pane, exported so a host's project
// list can match the refs pane's filter.
type FilterField struct {
	Value              func() string
	OnChange           func(string)
	Label, Placeholder string
	TestID             string
}

// Render implements core.Node.
func (f FilterField) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter}, Children: []core.Node{
		ui.Input{Value: f.Value, OnChange: f.OnChange, Label: f.Label, Placeholder: f.Placeholder, TestID: f.TestID,
			Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 12, Padding: core.Edges{Left: 26, Right: 8}}},
		core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(8)}, PointerEvents: core.PointerEventsNone,
			Children: []core.Node{ui.Icon(lucide.Search, 14, colors.Muted)}},
	}}
}

// treeRow is the compact height of the refs pane's rows.
const treeRow = 24

// The refs pane's tree: a group's rows start indentSpan in,
// each level of a folder indents levelSpan more, and every row keeps an
// arrowSlot for a folder's arrow, its label gap after it.
const (
	indentSpan, levelSpan, arrowSlot = 22, 16, 16
	// arrowSpan is the width a ref group's arrow and its gap add before
	// the group's icon, which the entries' icons line up with.
	arrowSpan = 22
	// headingSize is the groups' headings', 0.8 rem of 13 point text.
	headingSize = 10.4
)

// rowStyle is a refs pane row's look: compact, rounded, and filled while
// selected, with indent before its content.
func rowStyle(cx *core.Scope, indent core.Points, gap core.Points) (core.Style, func() core.Style) {
	colors := ui.ColorsOf(cx)
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: gap, Height: core.Pt(treeRow), FlexShrink: 0,
			Padding: core.Edges{Left: indent, Right: 8}, CornerRadius: 8, Cursor: core.CursorDefault},
		func() core.Style { return core.Style{Selected: &core.Style{Background: ui.Alpha(colors.Text(), 0.1)}} }
}

// sidebarItem is a refs pane entry as compact as the tree rows below it,
// followed by trailing, such as a count, unless it is nil. Its icon lines
// up with the ref groups' icons, which follow their disclosure arrow.
func (v *View) sidebarItem(cx *core.Scope, label string, icon func() *core.SVG, selected func() bool, trailing core.Node,
	press func(core.PressEvent), menu func() []core.MenuItem, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	style, styleFunc := rowStyle(cx, 8+arrowSpan, 6)
	text := line(label, 13, ui.Regular, colors.Text)
	text.Style.FlexGrow = 1
	children := []core.Node{ui.Icon(icon, 14, colors.Accent), text}
	if trailing != nil {
		children = append(children, trailing)
	}
	r := core.View{Role: core.RoleButton, Label: label, TestID: testID, Selected: v.is(selected), NoFocus: true,
		Style: style, StyleFunc: styleFunc, OnPress: press, Children: children}
	if menu != nil {
		r.OnContextMenu = v.menuOn(cx, menu)
	}
	return r
}

// treeIndent is where a tree row's arrow slot starts at depth.
func treeIndent(depth int) core.Points { return 4 + levelSpan*core.Points(depth) }

// treeItem is a ref's row in the refs pane, indented by depth, bold while
// bold says, which a press chooses and a double click activates.
func (v *View) treeItem(cx *core.Scope, label, tip string, depth int, selected, bold func() bool, press, activate func(), menu func() []core.MenuItem, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	style, styleFunc := rowStyle(cx, treeIndent(depth), 2)
	text := line(label, 13, ui.Regular, colors.Text)
	text.StyleFunc = func() core.Style {
		v.track()
		s := core.Style{Color: colors.Text()}
		if bold != nil && bold() {
			s.FontWeight = ui.Bold
		}
		return s
	}
	slot := core.View{Style: core.Style{Width: core.Pt(arrowSlot), FlexShrink: 0}}
	r := core.View{Role: core.RoleButton, Label: label, TestID: testID, Selected: v.is(selected), NoFocus: true,
		Style: style, StyleFunc: styleFunc, Children: []core.Node{slot, text},
		OnPress: func(e core.PressEvent) {
			if e.Clicks >= 2 && activate != nil {
				activate()
				return
			}
			if press != nil {
				press()
			}
		}}
	if menu != nil {
		r.OnContextMenu = v.menuOn(cx, menu)
	}
	if tip == "" {
		return r
	}
	return ui.Tooltip{Text: tip, Trigger: r}
}

// disclosure is the refs pane row key's disclosure, restored from state and
// otherwise open when open is set.
func (v *View) disclosure(key string, open bool) bool {
	if saved, ok := v.state.Groups[key]; ok {
		return saved
	}
	return open
}

// rememberDisclosure saves the refs pane row key's disclosure, dropping it
// once it is back to the default open.
func (v *View) rememberDisclosure(key string, state, open bool) {
	groups := map[string]bool{}
	for k, open := range v.state.Groups {
		groups[k] = open
	}
	if state == open {
		delete(groups, key)
	} else {
		groups[key] = state
	}
	v.state.Groups = groups
	v.saveState()
	v.invalidate()
}

// shownFunc reads the disclosure of the refs pane row key as the view
// changes; a filter shows it open.
func (v *View) shownFunc(key string, open, filtering bool) func() bool {
	return func() bool {
		v.track()
		return filtering || v.disclosure(key, open)
	}
}

// refGroup lists rows under a collapsible heading, led by icon, whose
// disclosure is remembered and springs open and closed. A filter shows the
// group open, and menu fills the heading's context menu unless it is nil.
func (v *View) refGroup(cx *core.Scope, key, label string, icon func() *core.SVG, open, filtering bool, menu func() []core.MenuItem, rows []core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	shown := v.shownFunc(key, open, filtering)
	title := line(label, headingSize, ui.Bold, colors.Muted)
	title.Style.FlexGrow = 1
	heading := core.View{Role: core.RoleButton, Label: label, Expanded: shown, NoFocus: true, TestID: "group-" + key,
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pt(treeRow), Padding: core.Symmetric(0, 8),
			CornerRadius: 8, Cursor: core.CursorDefault},
		OnPress: func(core.PressEvent) {
			if !filtering {
				v.rememberDisclosure(key, !v.disclosure(key, open), open)
			}
		},
		Children: []core.Node{disclosureArrow(cx, shown), ui.Icon(icon, 14, colors.Accent), title}}
	if menu != nil {
		heading.OnContextMenu = v.menuOn(cx, menu)
	}
	return base.Collapsible{Open: shown, Reveal: func() bool { return true }, Style: core.Style{FlexShrink: 0},
		Children: []core.Node{heading},
		Content:  core.View{Style: core.Style{Gap: 1, Padding: core.Edges{Top: 2, Bottom: 2, Left: indentSpan}}, Children: rows}}
}

// disclosureArrow is a disclosure's arrow, pointing along the line and
// turning down as open says it opens.
func disclosureArrow(cx *core.Scope, open func() bool) core.Node {
	style := core.Style{FlexShrink: 0, Width: core.Pt(arrowSlot), Height: core.Pt(arrowSlot), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
	if !ui.ReduceMotion(cx) {
		style.Transition = core.Transition{Duration: 200 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionTransform}
	}
	return core.View{Style: style, StyleFunc: func() core.Style {
		var degrees float32
		if open() {
			degrees = 90
		}
		return core.Style{Transform: core.Transform{Rotate: degrees}}
	}, Children: []core.Node{ui.Icon(lucide.ChevronRight, 12, ui.ColorsOf(cx).Muted)}}
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

// refRows lists refs as a tree of their name's folders, whose disclosure is
// remembered; choosing a ref selects its commit in the history. A filter
// shows the folders open.
func (v *View) refRows(cx *core.Scope, group string, filtering bool, nodes []*refNode, depth int) []core.Node {
	colors := ui.ColorsOf(cx)
	var rows []core.Node
	for _, n := range nodes {
		id := group + "/" + n.path
		if n.children == nil {
			tip := ""
			if n.name != n.path {
				tip = n.path
			}
			rows = append(rows, v.treeItem(cx, n.name, tip, depth,
				func() bool { return v.section == sectionCommits && v.ref == id },
				func() bool { return v.currentRef(group, n) },
				func() { v.revealCommit(n.hash); v.ref = id; v.invalidate() },
				func() { v.openCheckout(nodeTarget(group, n)) },
				func() []core.MenuItem { return v.refMenu(cx, nodeTarget(group, n), n) }, "ref-"+id))
			continue
		}
		// A folder's key ends in a slash, so it never meets a ref's.
		key := id + "/"
		open := v.shownFunc(key, true, filtering)
		children := []core.Node{disclosureArrow(cx, open)}
		// A remote's folder carries the mark of its host, nearer its label
		// than the arrow is, and its menu.
		var menu func() []core.MenuItem
		if _, remote := v.repo.remoteURLs[n.path]; remote && group == "remotes" {
			children = append(children, core.View{Style: core.Style{FlexShrink: 0, Margin: core.Edges{Left: 2, Right: 4}},
				Children: []core.Node{ui.Icon(remoteIcon(v.repo.github, n.path), 14, colors.Text)}})
			menu = func() []core.MenuItem { return v.remoteMenu(cx, n.path) }
		}
		children = append(children, line(n.name, 13, ui.Regular, colors.Text))
		style, _ := rowStyle(cx, treeIndent(depth), 2)
		folder := core.View{Role: core.RoleButton, Label: n.name, Expanded: open, NoFocus: true, TestID: "folder-" + id,
			Style: style, Children: children, OnPress: func(core.PressEvent) {
				if !filtering {
					v.rememberDisclosure(key, !v.disclosure(key, true), true)
				}
			}}
		if menu != nil {
			folder.OnContextMenu = v.menuOn(cx, menu)
		}
		rows = append(rows, base.Collapsible{Open: open, Reveal: func() bool { return true }, Style: core.Style{FlexShrink: 0},
			Children: []core.Node{folder},
			Content:  core.View{Style: core.Style{Gap: 1, Padding: core.Edges{Top: 1}}, Children: v.refRows(cx, group, filtering, n.children, depth+1)}})
	}
	return rows
}

// worktreeRows lists worktrees by their directory's name, the shown one in
// bold. Choosing one selects its checked out commit, and double-clicking
// another goes to OpenWorktree.
func (v *View) worktreeRows(cx *core.Scope, checkouts []Checkout) []core.Node {
	var rows []core.Node
	for _, w := range checkouts {
		id := "worktrees/" + w.Path
		shown := func() bool { return path.Clean(filepath.ToSlash(w.Path)) == path.Clean(filepath.ToSlash(v.path)) }
		tip := w.Path
		if w.Branch != "" {
			tip = w.Branch + "\n" + tip
		}
		var menu func() []core.MenuItem
		if v.opts.OpenWorktree != nil && !shown() {
			menu = func() []core.MenuItem {
				g := &menuGroups{}
				g.item(v.textf("ref_open_worktree", map[string]any{"Worktree": worktreeName(w.Path)}), false, func() { v.opts.OpenWorktree(w.Path) })
				return g.items
			}
		}
		rows = append(rows, v.treeItem(cx, worktreeName(w.Path), tip, 0,
			func() bool { return v.section == sectionCommits && v.ref == id }, shown,
			func() {
				if w.Head != "" {
					v.revealCommit(w.Head)
					v.ref = id
					v.invalidate()
				}
			},
			func() {
				if !shown() && v.opts.OpenWorktree != nil {
					v.opts.OpenWorktree(w.Path)
				}
			}, menu, "ref-"+id))
	}
	return rows
}

// worktreeName is the last part of a worktree's path, on this computer or a
// host.
func worktreeName(p string) string { return path.Base(filepath.ToSlash(p)) }

// filterCheckouts keeps the worktrees whose name or branch contains query,
// ignoring case. An empty query keeps every worktree.
func filterCheckouts(checkouts []Checkout, query string) []Checkout {
	query = strings.ToLower(query)
	if query == "" {
		return checkouts
	}
	var kept []Checkout
	for _, w := range checkouts {
		if strings.Contains(strings.ToLower(worktreeName(w.Path)), query) || strings.Contains(strings.ToLower(w.Branch), query) {
			kept = append(kept, w)
		}
	}
	return kept
}

// iconButton is a small square ghost button with a tooltip; active ones
// show their option is on.
func (v *View) iconButton(label string, icon func() *core.SVG, size core.Points, active func() bool, disabled func() bool, press func(), testID string) core.Node {
	return ui.IconButton{Icon: icon, Label: label, Size: size, IconSize: 16, Selected: active, Disabled: disabled, OnPress: press, TestID: testID}
}
