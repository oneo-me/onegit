package gitview

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// fileHistoryDialog lists the commits that changed a file.
type fileHistoryDialog struct {
	open, loading bool
	generation    uint64
	path, err     string
	commits       []Commit
	list          components.ListState
}

// blameDialog lists a file's lines with the commit that last changed each.
type blameDialog struct {
	open, loading bool
	generation    uint64
	path, rev     string
	err           string
	lines         []BlameLine
	list          components.ListState
}

// openFileHistory reads the commits that changed f in the checked out
// branch's history.
func (v *View) openFileHistory(p *filePane, f FileChange) {
	d := &v.fileHistory
	d.open, d.loading, d.path, d.err, d.commits = true, true, f.Path, "", nil
	d.generation++
	generation, git, path := d.generation, v.git, v.path
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
		commits, err := git.FileHistory(path, f.Path)
		v.dispatch(func() {
			if generation != d.generation || v.closed {
				return
			}
			d.loading = false
			if err != nil {
				d.err = err.Error()
			} else {
				d.commits = commits
			}
			v.invalidate()
		})
	}()
}

// fileHistoryDialogView shows the file's history and closes after the user
// chooses a commit to show in the history.
func (v *View) fileHistoryDialogView(c *ui.Context) {
	d := &v.fileHistory
	if !d.open {
		return
	}
	title := v.textf("file_history_title", map[string]any{"Path": d.path})
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		w, h := c.Size()
		panel.Width(min(t.Space(130), max(0, w-2*v.dialogInset(c)))).Height(min(t.Space(120), h*.7)).Padding(t.Space(3))
		ui.Column(c).FillWidth().FillHeight().MinHeight(0).Gap(t.Space(2)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			switch {
			case d.loading:
				ui.Column(c).Fill().Center().Children(func() { components.Spinner(c, components.SpinnerRing) })
				return
			case d.err != "":
				components.Text(c, d.err).Wrap().FillWidth().TextColor(t.Danger)
			case len(d.commits) == 0:
				components.Text(c, v.text("file_history_empty")).Wrap().FillWidth().TextColor(t.TextMuted)
			default:
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

func (v *View) fileHistoryRow(c *ui.Context, commit Commit) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(42).AlignItems(ui.Center).Gap(t.Space(2)).Padding(0, t.Space(1)).Children(func() {
		components.Text(c, commit.Short).Font("SF Mono, Menlo, monospace").FontSize(12).Width(72).Shrink(0).TextColor(t.TextMuted).SingleLine()
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			components.Text(c, commit.Subject).FontSize(13).SingleLine().Ellipsis("…")
			components.Text(c, commit.Author+" · "+commit.AuthorDate.Local().Format("2006-01-02 15:04")).FontSize(11.5).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		})
	})
}

// openBlame reads the blame of f at the version pane shows.
func (v *View) openBlame(p *filePane, f FileChange) {
	rev := p.changes.To
	if rev == "" && f.Area == AreaStaged {
		rev = p.changes.From
	}
	d := &v.blame
	d.open, d.loading, d.path, d.rev, d.err, d.lines = true, true, f.Path, rev, "", nil
	d.generation++
	generation, git, path := d.generation, v.git, v.path
	d.list.Key = func(i int) any { return i }
	d.list.Label = func(i int) string { return d.lines[i].Text }
	v.invalidate()
	go func() {
		lines, err := git.Blame(path, rev, f.Path)
		v.dispatch(func() {
			if generation != d.generation || v.closed {
				return
			}
			d.loading = false
			if err != nil {
				d.err = err.Error()
			} else {
				d.lines = lines
			}
			v.invalidate()
		})
	}()
}

// blameDialogView shows a file's blame line by line.
func (v *View) blameDialogView(c *ui.Context) {
	d := &v.blame
	if !d.open {
		return
	}
	title := v.textf("blame_title", map[string]any{"Path": d.path})
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		w, h := c.Size()
		panel.Width(min(t.Space(170), max(0, w-2*v.dialogInset(c)))).Height(min(t.Space(150), h*.8)).Padding(t.Space(3))
		ui.Column(c).FillWidth().FillHeight().MinHeight(0).Gap(t.Space(2)).Children(func() {
			components.Text(c, title).Bold().FontSize(t.Rem(1.1)).Wrap().FillWidth()
			switch {
			case d.loading:
				ui.Column(c).Fill().Center().Children(func() { components.Spinner(c, components.SpinnerRing) })
				return
			case d.err != "":
				components.Text(c, d.err).Wrap().FillWidth().TextColor(t.Danger)
			case len(d.lines) == 0:
				components.Text(c, v.text("blame_empty")).Wrap().FillWidth().TextColor(t.TextMuted)
			default:
				ui.Row(c).FillWidth().Shrink(0).Gap(t.Space(2)).Padding(0, t.Space(1)).Children(func() {
					components.Text(c, v.text("commit_hash")).FontSize(11).Width(72).Shrink(0).TextColor(t.TextMuted)
					components.Text(c, v.text("commit_author")).FontSize(11).Width(120).Shrink(0).TextColor(t.TextMuted)
					components.Text(c, v.text("blame_when")).FontSize(11).Width(96).Shrink(0).TextColor(t.TextMuted)
					components.Text(c, v.text("blame_line")).FontSize(11).Grow(1).TextColor(t.TextMuted)
				})
				components.List(c, &d.list, len(d.lines), func(i int) {
					v.blameRow(c, d.lines[i])
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

func (v *View) blameRow(c *ui.Context, line BlameLine) {
	t := c.Theme()
	date := ""
	if !line.Date.IsZero() {
		date = line.Date.Local().Format("2006-01-02")
	}
	ui.Row(c).FillWidth().Height(20).AlignItems(ui.Center).Gap(t.Space(2)).Padding(0, t.Space(1)).Children(func() {
		components.Text(c, line.Short).Font("SF Mono, Menlo, monospace").FontSize(11).Width(72).Shrink(0).TextColor(t.TextMuted).SingleLine()
		components.Text(c, line.Author).FontSize(11.5).Width(120).Shrink(0).SingleLine().Ellipsis("…")
		components.Text(c, date).FontSize(11.5).Width(96).Shrink(0).TextColor(t.TextMuted).SingleLine()
		components.Text(c, line.Text).Font("SF Mono, Menlo, monospace").FontSize(11.5).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
	})
}
