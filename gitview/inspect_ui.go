package gitview

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// fileHistoryDialog lists the commits that changed a file.
type fileHistoryDialog struct {
	open, loading bool
	generation    uint64
	path, err     string
	commits       []Commit
}

// blameDialog lists a file's lines with the commit that last changed each.
type blameDialog struct {
	open, loading bool
	generation    uint64
	path, rev     string
	err           string
	lines         []BlameLine
}

// openFileHistory reads the commits that changed f in the checked out
// branch's history.
func (v *View) openFileHistory(p *filePane, f FileChange) {
	d := &v.fileHistory
	d.open, d.loading, d.path, d.err, d.commits = true, true, f.Path, "", nil
	d.generation++
	generation, git, path := d.generation, v.git, v.path
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
func (v *View) fileHistoryDialogView(cx *core.Scope) core.Node {
	d := &v.fileHistory
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 520, height: 480, testID: "file-history-dialog",
		title: func() string { return v.textf("file_history_title", map[string]any{"Path": d.path}) },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			key := func() string { v.track(); return fmt.Sprint(d.loading, d.err, len(d.commits), d.generation) }
			return []core.Node{region(cx, key, func(cx *core.Scope) core.Node {
				switch {
				case d.loading:
					return centered(spinner(24))
				case d.err != "":
					return dialogNote(d.err, 13, colors.Danger)
				case len(d.commits) == 0:
					return muted(cx, v.text("file_history_empty"), 13)
				}
				return v.commitList(cx, d.commits, func(hash string) {
					d.open = false
					v.revealCommit(hash)
				}, "file-history")
			})}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{ui.Button{Text: v.text("close_details"), Variant: ui.ButtonSecondary, TestID: "close", OnPress: v.closeDialog(&d.open)}}
		}})
}

// commitList lists commits by hash, subject, author and date; activating
// one runs choose with its hash.
func (v *View) commitList(cx *core.Scope, commits []Commit, choose func(hash string), testID string) core.Node {
	colors := ui.ColorsOf(cx)
	return ui.List{TestID: testID, Count: func() int { return len(commits) }, EstimatedSize: 42, Selection: ui.NewSelection(cx, false),
		Key:      func(i int) any { return commits[i].Hash },
		RowLabel: func(i int) string { return commits[i].Subject },
		OnActivate: func(i int) {
			if i >= 0 && i < len(commits) {
				choose(commits[i].Hash)
			}
		},
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			return core.Dynamic(func(cx *core.Scope) core.Node {
				i := row.Index()
				if i < 0 || i >= len(commits) {
					return nil
				}
				c := commits[i]
				hash := line(c.Short, 12, ui.Regular, colors.Muted)
				hash.Style.Width, hash.Style.FlexShrink, hash.Style.FontFamily = core.Pt(72), 0, ui.Mono
				text := column(1, line(c.Subject, 13, ui.Regular, colors.Text),
					line(c.Author+" · "+c.AuthorDate.Local().Format("2006-01-02 15:04"), 11.5, ui.Regular, colors.Muted))
				text.Style.FlexGrow, text.Style.FlexShrink, text.Style.MinWidth = 1, 1, core.Pt(0)
				r := row8(hash, text)
				r.Style.Height = core.Pt(42)
				return r
			})
		},
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(160)}}
}

// row8 lays out children in a row 8 points apart, filling the width.
func row8(children ...core.Node) core.View {
	r := row(8, children...)
	r.Style.FlexGrow, r.Style.FlexShrink, r.Style.MinWidth, r.Style.Padding = 1, 1, core.Pt(0), core.Symmetric(0, 4)
	return r
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
func (v *View) blameDialogView(cx *core.Scope) core.Node {
	d := &v.blame
	return v.dialogNode(cx, dialogSpec{open: &d.open, width: 680, height: 600, testID: "blame-dialog",
		title: func() string { return v.textf("blame_title", map[string]any{"Path": d.path}) },
		body: func(cx *core.Scope) []core.Node {
			colors := ui.ColorsOf(cx)
			key := func() string { v.track(); return fmt.Sprint(d.loading, d.err, len(d.lines), d.generation) }
			return []core.Node{region(cx, key, func(cx *core.Scope) core.Node {
				switch {
				case d.loading:
					return centered(spinner(24))
				case d.err != "":
					return dialogNote(d.err, 13, colors.Danger)
				case len(d.lines) == 0:
					return muted(cx, v.text("blame_empty"), 13)
				}
				heading := func(text string, width core.Points) core.Text {
					t := line(text, 11, ui.Regular, colors.Muted)
					if width > 0 {
						t.Style.Width, t.Style.FlexShrink = core.Pt(width), 0
					} else {
						t.Style.FlexGrow = 1
					}
					return t
				}
				header := row8(heading(v.text("commit_hash"), 72), heading(v.text("commit_author"), 120), heading(v.text("blame_when"), 96), heading(v.text("blame_line"), 0))
				lines := d.lines
				list := ui.List{TestID: "blame", Count: func() int { return len(lines) }, EstimatedSize: 20,
					Key:      func(i int) any { return i },
					RowLabel: func(i int) string { return lines[i].Text },
					Row: func(cx *core.Scope, r core.ListRow) core.Node {
						return core.Dynamic(func(cx *core.Scope) core.Node {
							i := r.Index()
							if i < 0 || i >= len(lines) {
								return nil
							}
							return v.blameRow(cx, lines[i])
						})
					},
					Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(160)}}
				return core.View{Style: ui.FillRest.Merge(core.Style{Gap: 4}), Children: []core.Node{header, list}}
			})}
		},
		actions: func(cx *core.Scope) []core.Node {
			return []core.Node{ui.Button{Text: v.text("close_details"), Variant: ui.ButtonSecondary, TestID: "close", OnPress: v.closeDialog(&d.open)}}
		}})
}

func (v *View) blameRow(cx *core.Scope, l BlameLine) core.Node {
	colors := ui.ColorsOf(cx)
	date := ""
	if !l.Date.IsZero() {
		date = l.Date.Local().Format("2006-01-02")
	}
	cell := func(text string, width core.Points, color func() core.Color, mono bool) core.Text {
		t := line(text, 11.5, ui.Regular, color)
		if width > 0 {
			t.Style.Width, t.Style.FlexShrink = core.Pt(width), 0
		} else {
			t.Style.FlexGrow = 1
		}
		if mono {
			t.Style.FontFamily = ui.Mono
		}
		return t
	}
	r := row8(cell(l.Short, 72, colors.Muted, true), cell(l.Author, 120, colors.Text, false), cell(date, 96, colors.Muted, false), cell(l.Text, 0, colors.Text, true))
	r.Style.Height = core.Pt(20)
	return r
}
