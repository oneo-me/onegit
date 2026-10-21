package gitview

import (
	"github.com/egoist/mygo/ui"
)

// graphPalette colors the revision graph's lines, so parallel branches stay
// distinguishable, as Onegit draws them.
var graphPalette = []ui.Color{
	ui.Hex("#FF9502"), ui.Hex("#FFCC00"), ui.Hex("#FF3B30"), ui.Hex("#A2845E"),
	ui.Hex("#64DA38"), ui.Hex("#1CADF8"), ui.Hex("#CB73E1"), ui.Hex("#8E8E91"),
	ui.Hex("#FF2968"), ui.Hex("#30D5C8"), ui.Hex("#5856D6"), ui.Hex("#B4D435"),
	ui.Hex("#FF6F61"),
}

func graphColor(line int) ui.Color {
	return graphPalette[max(line, 0)%len(graphPalette)]
}

// graphLine is one line crossing a commit's row: it enters the row's top edge
// in column Top, passes the row's middle in Column and leaves the bottom edge
// in Bottom. -1 means the line does not touch that edge, so it ends at the
// commit (no Bottom) or starts from it (no Top). ID keeps its color.
type graphLine struct {
	ID, Top, Column, Bottom int
}

// graphRow is the revision graph of one commit: the lines crossing its row,
// the column of the commit itself, the line it belongs to and the number of
// columns the row draws in.
type graphRow struct {
	Lines                 []graphLine
	Column, Line, Columns int
}

// maxGraphLines bounds a graph's open lines, so a repository with very many
// open branches cannot widen it without limit.
const maxGraphLines = 64

// openLine waits for the commit it reaches next, descending from a child to
// its parent.
type openLine struct {
	next string
	id   int
}

// buildGraph lays out the revision graph of commits, listed newest first with
// each after its children. It keeps a line open for each parent still to come
// and numbers a new line by the column it opens in, as Fork does, so a branch
// takes that column's color and keeps it while open and a merge folds it back
// into its first parent. It returns a row per commit.
func buildGraph(commits []Commit) []graphRow {
	rows := make([]graphRow, len(commits))
	var previous, current []openLine
	for i, commit := range commits {
		current = current[:0]
		var first string
		if len(commit.Parents) > 0 {
			first = commit.Parents[0]
		}
		column, id, found := -1, -1, false
		var lines []graphLine
		for j, line := range previous {
			switch {
			case line.next != commit.Hash:
				// The line passes this commit: carry it down, compacted.
				lines = append(lines, graphLine{line.id, j, len(current), len(current)})
				current = append(current, line)
			case !found:
				// The commit's own line continues into its first parent, or
				// ends at a root commit.
				found, column, id = true, len(current), line.id
				if first == "" {
					lines = append(lines, graphLine{line.id, j, column, -1})
					continue
				}
				current = append(current, openLine{first, line.id})
				lines = append(lines, graphLine{line.id, j, column, column})
			default:
				// Another child's line to this commit ends at it.
				lines = append(lines, graphLine{line.id, j, column, -1})
			}
		}
		if len(current) < maxGraphLines {
			if !found {
				// A branch tip opens a line in the next column.
				column, id = len(current), len(current)
				if first != "" {
					current = append(current, openLine{first, id})
					lines = append(lines, graphLine{id, -1, column, column})
				}
			}
			// A merge's later parents connect to the commit, joining the
			// line already waiting for that parent when there is one.
			for _, parent := range commit.Parents[min(1, len(commit.Parents)):] {
				joined := false
				for k, line := range current {
					if line.next == parent {
						lines = append(lines, graphLine{line.id, -1, column, k})
						joined = true
						break
					}
				}
				if !joined {
					lines = append(lines, graphLine{len(current), -1, column, len(current)})
					current = append(current, openLine{parent, len(current)})
				}
			}
		}
		columns := column + 1
		for _, line := range lines {
			columns = max(columns, line.Top+1, line.Column+1, line.Bottom+1)
		}
		rows[i] = graphRow{Lines: lines, Column: column, Line: id, Columns: columns}
		previous, current = current, previous
	}
	return rows
}

// Graph cells share a lane width, so the rows of the list line up.
const graphLane = 12

// mergeToggle is a merge's button in the revision graph, which collapses the
// commits it brings in into its own row or expands them again.
type mergeToggle struct {
	collapsed bool
	label     string
	toggle    func()
}

// graphCell draws one commit's row of the revision graph, height high: lines
// cross the row and curve into and out of the commit. A commit is a ring in
// its line's color; a merge, given its toggle, is a disc with a chevron
// pointing down while expanded and right, inside a ring, while collapsed. The cell
// is only as wide as its own row's columns, so a busy stretch of history
// does not push every subject away from its graph.
func graphCell(c *ui.Context, row graphRow, height float32, merge *mergeToggle) ui.Element {
	t := c.Theme()
	hovered := false
	cell := ui.Box(c).Width(float32(row.Columns)*graphLane + graphLane/2).Height(height).Shrink(0).Clip()
	if merge != nil && row.Column >= 0 {
		cell.Children(func() {
			const size = 18
			button := ui.Box(c).Absolute().Left(graphLane/2+float32(row.Column)*graphLane-size/2).Top((height-size)/2).
				Size(size, size).Role(ui.RoleButton).Label(merge.label).Tooltip(merge.label).Cursor(ui.CursorPointer).
				Expanded(!merge.collapsed)
			if button.Clicked() {
				merge.toggle()
				c.Invalidate()
			}
			hovered = button.Hovered()
		})
	}
	return cell.
		DrawOver(func(p *ui.Painter, r ui.Rect) {
			at := func(column int) float32 { return r.X + graphLane/2 + float32(column)*graphLane }
			top, mid, bottom := r.Y, r.Y+height/2, r.Y+height
			for _, line := range row.Lines {
				color := graphColor(line.ID)
				switch {
				case line.Top < 0:
					graphBend(p, at(line.Column), mid, at(line.Bottom), bottom, at(line.Bottom), mid, color)
				case line.Bottom < 0:
					graphBend(p, at(line.Top), top, at(line.Column), mid, at(line.Top), mid, color)
				default:
					graphStroke(p, at(line.Top), top, at(line.Bottom), bottom, color)
				}
			}
			if row.Column < 0 {
				return
			}
			x, y, color := at(row.Column), mid, graphColor(row.Line)
			if merge != nil {
				// A solid disc with a cut-out chevron reads as a button; a
				// collapsed merge adds an outer ring for the commits it holds.
				radius := float32(6.5)
				if merge.collapsed {
					p.Fill(ui.Rect{X: x - 9.5, Y: y - 9.5, W: 19, H: 19}, t.Background, 9.5)
				}
				if hovered {
					p.Fill(ui.Rect{X: x - 10, Y: y - 10, W: 20, H: 20}, color.Alpha(.22), 10)
				}
				if merge.collapsed {
					radius = 5.5
					p.Stroke(ui.Rect{X: x - 8.5, Y: y - 8.5, W: 17, H: 17}, color, 8.5, 1.25)
				}
				p.Fill(ui.Rect{X: x - radius, Y: y - radius, W: radius * 2, H: radius * 2}, color, radius)
				var path ui.Path
				if merge.collapsed {
					path.MoveTo(x-1, y-2.5).LineTo(x+1.5, y).LineTo(x-1, y+2.5)
				} else {
					path.MoveTo(x-2.75, y-1).LineTo(x, y+1.75).LineTo(x+2.75, y-1)
				}
				p.StrokePath(&path, 1.5, t.Background)
				return
			}
			p.Fill(ui.Rect{X: x - 4, Y: y - 4, W: 8, H: 8}, color, 4)
			p.Fill(ui.Rect{X: x - 1.75, Y: y - 1.75, W: 3.5, H: 3.5}, t.Background, 2)
		})
}

// graphStroke draws a straight segment within a column, or a curve between
// two, so branches and merges read as smooth joins.
func graphStroke(p *ui.Painter, fromX, fromY, toX, toY float32, color ui.Color) {
	var path ui.Path
	path.MoveTo(fromX, fromY)
	if fromX == toX {
		path.LineTo(toX, toY)
	} else {
		path.CubeTo(fromX, toY-5, toX, fromY+5, toX, toY)
	}
	p.StrokePath(&path, 1.5, color)
}

// graphBend draws a line that starts or ends at a commit: across columns it
// bends through the corner, so it meets the commit's side at its middle
// rather than curving out from under it.
func graphBend(p *ui.Painter, fromX, fromY, toX, toY, cornerX, cornerY float32, color ui.Color) {
	if fromX == toX {
		graphStroke(p, fromX, fromY, toX, toY, color)
		return
	}
	var path ui.Path
	path.MoveTo(fromX, fromY).QuadTo(cornerX, cornerY, toX, toY)
	p.StrokePath(&path, 1.5, color)
}

// foldCommits hides the commits that each collapsed merge brings in: those
// its later parents reach but its first parent does not, mapping each to the
// merge that hides it. A commit stays shown when a shown commit other than
// the merge descends from it, so every shown line still reaches its parent.
// commits are listed with each after its children.
func foldCommits(commits []Commit, collapsed map[string]bool) map[string]string {
	if len(collapsed) == 0 {
		return nil
	}
	index := make(map[string]int, len(commits))
	children := make([][]int, len(commits))
	for i, commit := range commits {
		index[commit.Hash] = i
	}
	for i, commit := range commits {
		for _, parent := range commit.Parents {
			if j, ok := index[parent]; ok {
				children[j] = append(children[j], i)
			}
		}
	}
	hidden := map[string]string{}
	const (
		first = 1 << iota
		side
	)
	for m, merge := range commits {
		if !collapsed[merge.Hash] || len(merge.Parents) < 2 || hidden[merge.Hash] != "" {
			continue
		}
		// Children come first, so a commit's marks are complete once the walk
		// reaches it; the walk stops when no side commit is left to reach.
		marks, pending := map[int]int{}, 0
		mark := func(hash string, bit int) {
			if j, ok := index[hash]; ok && j > m {
				if bit == side && marks[j]&side == 0 {
					pending++
				}
				marks[j] |= bit
			}
		}
		mark(merge.Parents[0], first)
		for _, parent := range merge.Parents[1:] {
			mark(parent, side)
		}
		for i := m + 1; i < len(commits) && pending > 0; i++ {
			bits := marks[i]
			if bits&side != 0 {
				pending--
			}
			switch {
			case bits&first != 0:
				for _, parent := range commits[i].Parents {
					mark(parent, first)
				}
			case bits&side != 0:
				shown := false
				for _, child := range children[i] {
					if child != m && hidden[commits[child].Hash] == "" {
						shown = true
						break
					}
				}
				if !shown {
					hidden[commits[i].Hash] = merge.Hash
				}
				for _, parent := range commits[i].Parents {
					mark(parent, side)
				}
			}
		}
	}
	return hidden
}
