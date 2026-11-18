package gitview

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// historySearchDialog searches the repository's commits by message, author,
// changed path or changed content, over every ref or the checked out branch.
type historySearchDialog struct {
	open, loading bool
	generation    uint64
	query         string
	typ           SearchType
	scope         SearchScope
	err           string
	commits       []Commit
}

// openHistorySearch asks for a query and searches after the button or Return.
func (v *View) openHistorySearch() {
	if !v.available() {
		return
	}
	v.search.open, v.search.err, v.search.commits = true, "", nil
	v.search.query = ""
	v.invalidate()
}

// runHistorySearch reads the commits matching the dialog's query.
func (v *View) runHistorySearch() {
	d := &v.search
	query := strings.TrimSpace(d.query)
	if query == "" {
		d.commits, d.err = nil, ""
		v.invalidate()
		return
	}
	d.loading, d.generation, d.err, d.commits = true, d.generation+1, "", nil
	generation, git, path, opt := d.generation, v.git, v.path, SearchOptions{Type: d.typ, Scope: d.scope, Query: query}
	v.invalidate()
	go func() {
		commits, err := git.SearchCommits(path, opt)
		v.dispatch(func() {
			if generation != d.generation || v.closed {
				return
			}
			d.loading = false
			if err != nil {
				d.err = err.Error()
			} else {
				if commits == nil {
					commits = []Commit{}
				}
				d.commits = commits
			}
			v.invalidate()
		})
	}()
}

// historySearchDialogView shows the history search and its results.
func (v *View) historySearchDialogView(cx *core.Scope) core.Node {
	d := &v.search
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 600, height: 600, testID: "search-dialog",
		title: func() string { return v.text("search_history") },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			types := []ui.Option[SearchType]{
				{Value: SearchAll, Label: v.text("search_all")},
				{Value: SearchMessage, Label: v.text("search_message")},
				{Value: SearchAuthor, Label: v.text("search_author")},
				{Value: SearchPath, Label: v.text("search_path")},
				{Value: SearchContent, Label: v.text("search_content")},
			}
			typeField := ui.Select[SearchType]{Options: types, Label: v.text("search_type"), TestID: "search-type", Style: core.Style{Width: core.Pt(136), FlexShrink: 0},
				Value: func() SearchType { v.watch(); return d.typ }, OnChange: func(t SearchType) { d.typ = t; v.edited() }}
			allRefs := d.scope == SearchRepository
			options := row(12, typeField, v.toggle(cx, &allRefs, v.text("search_all_refs"), func() {
				if allRefs {
					d.scope = SearchRepository
				} else {
					d.scope = SearchCurrentBranch
				}
			}), ui.Spacer{}, ui.Button{Text: v.text("search"), Variant: ui.ButtonPrimary, TestID: "search", OnPress: v.runHistorySearch})
			options.Style.Wrap = core.Wrap
			key := func() string {
				v.track()
				return fmt.Sprint(d.loading, d.err, d.commits == nil, len(d.commits), d.generation)
			}
			results := region(cx, key, func(cx *core.Scope) core.Node {
				switch {
				case d.loading:
					return centered(spinner(24))
				case d.err != "":
					return dialogNote(d.err, 13, colors.Danger)
				case d.commits == nil:
					return muted(cx, v.text("search_history_hint"), 13)
				case len(d.commits) == 0:
					return muted(cx, v.text("search_empty"), 13)
				}
				return core.View{Style: ui.FillRest.Merge(core.Style{Gap: 4}), Children: []core.Node{
					muted(cx, v.textf("search_results", map[string]any{"Count": len(d.commits)}), 12),
					v.commitList(cx, d.commits, func(hash string) {
						d.open = false
						v.revealCommit(hash)
					}, "search-results"),
				}}
			})
			return []core.Node{
				v.textField(&d.query, v.text("search_history"), v.text("search_history_placeholder"), true, v.runHistorySearch, "search-query"),
				options, hairline(cx), results,
			}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{ui.Button{Text: v.text("close_details"), Variant: ui.ButtonSecondary, TestID: "close", OnPress: v.closeDialog(&d.open)}}
		}})
}
