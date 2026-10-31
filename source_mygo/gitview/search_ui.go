package gitview

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
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
	list          components.ListState
}

// openHistorySearch asks for a query and searches after the button or Enter.
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
		return
	}
	d.loading, d.generation, d.err, d.commits = true, d.generation+1, "", nil
	generation, git, path, opt := d.generation, v.git, v.path, SearchOptions{Type: d.typ, Scope: d.scope, Query: query}
	d.list.Key = func(i int) any { return d.commits[i].Hash }
	d.list.Label = func(i int) string { return d.commits[i].Subject }
	d.list.OnSubmit = func(i int) {
		if i < 0 || i >= len(d.commits) {
			return
		}
		hash := d.commits[i].Hash
		d.open = false
		v.revealCommit(hash)
	}
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
func (v *View) historySearchDialogView(c *ui.Context) {
	d := &v.search
	if !d.open {
		return
	}
	title := v.text("search_history")
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		w, h := c.Size()
		panel.Width(min(t.Space(150), max(0, w-2*v.dialogInset(c)))).Height(min(t.Space(150), h*.8)).Padding(t.Space(3))
		ui.Column(c).FillWidth().FillHeight().MinHeight(0).Gap(t.Space(2)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1))
			box := components.TextBox(c, &d.query, components.TextBoxOptions{LeadingIcon: nil}).
				FillWidth().Label(title).Placeholder(v.text("search_history_placeholder")).AutoFocus()
			box.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods != 0 {
					return false
				}
				d.query = k.Text()
				v.runHistorySearch()
				return true
			})
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(t.Space(3)).Children(func() {
				types := []struct {
					value SearchType
					label string
				}{
					{SearchAll, v.text("search_all")},
					{SearchMessage, v.text("search_message")},
					{SearchAuthor, v.text("search_author")},
					{SearchPath, v.text("search_path")},
					{SearchContent, v.text("search_content")},
				}
				typeName := v.text("search_all")
				for _, item := range types {
					if item.value == d.typ {
						typeName = item.label
					}
				}
				sel := components.Select(c, &d.typ)
				sel.Trigger.Label(v.text("search_type")).Width(t.Space(34)).Children(func() {
					components.Text(c, typeName).SingleLine()
				})
				sel.Popup(func(panel ui.Element) {
					for _, item := range types {
						sel.Item(item.value).Label(item.label).Children(func() {
							components.Text(c, item.label).SingleLine()
						})
					}
				})
				allRefs := d.scope == SearchRepository
				if components.Switch(c, &allRefs, v.text("search_all_refs")).Changed() {
					if allRefs {
						d.scope = SearchRepository
					} else {
						d.scope = SearchCurrentBranch
					}
				}
				ui.Spacer(c)
				if components.Button(c, v.text("search"), components.ButtonPrimary).Clicked() {
					v.runHistorySearch()
				}
			})
			ui.Box(c).FillWidth().Height(1).Background(t.Border)
			switch {
			case d.loading:
				ui.Column(c).Fill().Center().Children(func() { components.Spinner(c, components.SpinnerRing) })
				return
			case d.err != "":
				components.Text(c, d.err).Wrap().FillWidth().TextColor(t.Danger)
			case d.commits == nil:
				components.Text(c, v.text("search_history_hint")).Wrap().FillWidth().TextColor(t.TextMuted)
			case len(d.commits) == 0:
				components.Text(c, v.text("search_empty")).Wrap().FillWidth().TextColor(t.TextMuted)
			default:
				components.Text(c, v.textf("search_results", map[string]any{"Count": len(d.commits)})).FontSize(12).TextColor(t.TextMuted)
				components.List(c, &d.list, len(d.commits), func(i int) {
					v.fileHistoryRow(c, d.commits[i])
				}).Fill().MinHeight(0)
			}
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if components.Button(c, v.text("close_details"), components.ButtonSecondary).Clicked() {
					d.open = false
				}
			})
		})
	})
}
