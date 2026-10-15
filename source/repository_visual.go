package main

import (
	"hash/fnv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// toolbarButton is a toolbar action with an icon over its label, sized like
// the rest of the toolbar rather than Onevra's square icon button.
func (s *settings) toolbarButton(c *ui.Context, icon *ui.SVG, label string, disabled bool, action func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Key("toolbar-" + label).Width(t.Space(15)).Height(t.Space(12)).
		Column().Gap(t.Space(.5)).Radius(t.Radius).Label(label).TextColor(t.TextMuted)
	if disabled {
		b.Disabled(true)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		if b.IsDisabled() {
			return
		}
		switch {
		case b.Pressed():
			p.Fill(r, t.Text.Alpha(.14), t.Radius)
		case b.Hovered():
			p.Fill(r, t.Text.Alpha(.08), t.Radius)
		}
	}).Children(func() {
		components.Icon(c, icon).Size(t.Space(4.5), t.Space(4.5)).TextColor(t.Text)
		components.Text(c, label).FontSize(t.Rem(.68)).SingleLine()
	})
	components.Tooltip(c, b, label)
	if !disabled && b.Clicked() {
		action()
	}
}

// toolbarMenuButton is toolbarButton with a dropdown menu, matching the
// reference's paired action and arrow controls.
func (s *settings) toolbarMenuButton(c *ui.Context, icon *ui.SVG, label string, disabled bool, menu func(*ui.Menu)) {
	t := c.Theme()
	b := ui.ButtonBase(c).Key("toolbar-menu-" + label).Width(t.Space(15)).Height(t.Space(12)).
		Column().Gap(t.Space(.5)).Radius(t.Radius).Label(label).TextColor(t.TextMuted)
	if disabled {
		b.Disabled(true)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		if b.IsDisabled() {
			return
		}
		switch {
		case b.Pressed():
			p.Fill(r, t.Text.Alpha(.14), t.Radius)
		case b.Hovered():
			p.Fill(r, t.Text.Alpha(.08), t.Radius)
		}
	}).Children(func() {
		components.Icon(c, icon).Size(t.Space(4.5), t.Space(4.5)).TextColor(t.Text)
		components.Text(c, label).FontSize(t.Rem(.68)).SingleLine()
	})
	if !disabled {
		b.Menu(menu)
	}
	components.Tooltip(c, b, label)
}

// authorPalette colors deterministic author avatars, in the spirit of the
// reference's per-user avatars.
var authorPalette = []ui.Color{
	ui.Hex("#E57373"), ui.Hex("#BA68C8"), ui.Hex("#7986CB"), ui.Hex("#4FC3F7"),
	ui.Hex("#4DB6AC"), ui.Hex("#81C784"), ui.Hex("#FFB74D"), ui.Hex("#A1887F"),
}

func authorColor(name string) ui.Color {
	if name == "" {
		return authorPalette[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return authorPalette[int(h.Sum32())%len(authorPalette)]
}

func authorInitials(name string) string {
	fields := strings.Fields(name)
	switch {
	case len(fields) == 0:
		return "?"
	case len(fields) == 1:
		runes := []rune(fields[0])
		if len(runes) > 1 {
			return strings.ToUpper(string(runes[:2]))
		}
		return strings.ToUpper(string(runes))
	default:
		first := []rune(fields[0])
		lastField := []rune(fields[len(fields)-1])
		return strings.ToUpper(string(first[:1]) + string(lastField[:1]))
	}
}

// authorAvatar renders a colored square with the author's initials.
func authorAvatar(c *ui.Context, name string, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).Radius(size * .22).
		Background(authorColor(name)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		components.Text(c, authorInitials(name)).FontSize(size * .42).Bold().
			TextColor(ui.Hex("#FFFFFF")).SingleLine()
	})
}

// refPill renders one branch, remote or tag label as the reference does.
func refPill(c *ui.Context, ref string) {
	t := c.Theme()
	icon := lucide.GitBranch()
	name := ref
	if strings.HasPrefix(ref, "tag: ") {
		icon = lucide.Tag()
		name = strings.TrimPrefix(ref, "tag: ")
	} else if strings.HasPrefix(ref, "HEAD -> ") {
		icon = lucide.CircleDot()
		name = strings.TrimPrefix(ref, "HEAD -> ")
	} else if ref == "HEAD" {
		icon = lucide.CircleDot()
	}
	bg, fg := ui.Hex("#FBE3CC"), ui.Hex("#9A5B22")
	if t.Dark {
		bg, fg = ui.Hex("#4A3624"), ui.Hex("#E8B27C")
	}
	ui.Row(c).Shrink(0).Height(t.Space(4.5)).Radius(t.Space(.75)).Background(bg).
		Padding(0, t.Space(1.25)).Gap(t.Space(.75)).AlignItems(ui.Center).Children(func() {
		components.Icon(c, icon).Size(t.Space(2.75), t.Space(2.75)).TextColor(fg)
		components.Text(c, name).FontSize(t.Rem(.72)).TextColor(fg).Shrink(0).SingleLine()
	})
}

// refPills splits the `git log --decorate` string into individual labels.
func refPills(c *ui.Context, refs string) {
	if strings.TrimSpace(refs) == "" {
		return
	}
	for _, ref := range strings.Split(refs, ", ") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		refPill(c, ref)
	}
}

// graphPalette colors the graph lanes, matching the reference graph's
// branch colors so parallel lines stay distinguishable.
var graphPalette = []ui.Color{
	ui.Hex("#FF9502"), ui.Hex("#FFCC00"), ui.Hex("#FF3B30"), ui.Hex("#A2845E"),
	ui.Hex("#64DA38"), ui.Hex("#1CADF8"), ui.Hex("#CB73E1"), ui.Hex("#8E8E91"),
	ui.Hex("#FF2968"), ui.Hex("#30D5C8"), ui.Hex("#5856D6"), ui.Hex("#B4D435"),
	ui.Hex("#FF6F61"),
}

func graphColor(lane int) ui.Color {
	if lane < 0 {
		lane = 0
	}
	return graphPalette[lane%len(graphPalette)]
}

// graphLine is one line of a revision graph row, following the reference's
// GraphLine: it enters the row's top edge at Top, passes through Column at the
// row's centre and leaves the bottom edge at Bottom. -1 means the line does
// not touch that edge, so a line either ends at the commit point (no Bottom)
// or starts from it (no Top).
type graphLine struct {
	ID     int
	Top    int
	Column int
	Bottom int
}

// graphRow is the revision graph of one commit: the lines that cross the row
// and the column of the commit point itself. CommitLineID is the color of the
// line the commit belongs to.
type graphRow struct {
	Lines        []graphLine
	CommitColumn int
	CommitLineID int
}

// maxGraphLines bounds the active lines of a graph so a repository with an
// unusual number of open branches cannot grow it without limit.
const maxGraphLines = 200

// activeGraphLine is a line waiting for the commit it connects to next as it
// descends from a child to a parent.
type activeGraphLine struct {
	next string
	id   int
}

// buildGraph follows the reference application's revision graph layout: it
// walks the commits newest first, keeps a line open for each parent still to
// come and gives every line a stable id, so a branch keeps its column and
// color while it is open and a merge folds it back into its first parent. It
// returns the rows and the number of columns the graph needs.
func buildGraph(commits []commitEntry) ([]graphRow, int) {
	rows := make([]graphRow, len(commits))
	// The reference alternates two line lists; each row reads one and fills
	// the other, so a row never overwrites the state it is reading.
	lists := [2][]activeGraphLine{}
	parity := 0
	nextID := 0
	columns := 0
	for i, commit := range commits {
		previous := lists[parity]
		current := &lists[1-parity]
		*current = (*current)[:0]
		parents := commit.Parents
		hasFirstParent := len(parents) > 0
		firstParent := ""
		if hasFirstParent {
			firstParent = parents[0]
		}
		commitColumn := -1
		commitLineID := -1
		found := false
		var lines []graphLine
		for j, line := range previous {
			switch {
			case line.next != commit.Hash:
				// The line skips this commit: carry it down, compacted.
				lines = append(lines, graphLine{line.id, j, len(*current), len(*current)})
				*current = append(*current, line)
			case !found:
				// This is the commit's own line: it continues into the first
				// parent, or ends here for a root commit.
				found = true
				commitColumn = len(*current)
				commitLineID = line.id
				if !hasFirstParent {
					lines = append(lines, graphLine{line.id, j, commitColumn, -1})
					continue
				}
				*current = append(*current, activeGraphLine{firstParent, line.id})
				lines = append(lines, graphLine{line.id, j, commitColumn, commitColumn})
			default:
				// A duplicate occurrence of a merge's second parent ends at
				// this commit point.
				lines = append(lines, graphLine{line.id, j, commitColumn, -1})
			}
		}
		if len(*current) < maxGraphLines {
			if !found && hasFirstParent {
				// A branch tip whose child is not in the list opens a new line.
				commitColumn = len(*current)
				line := activeGraphLine{firstParent, nextID}
				nextID++
				*current = append(*current, line)
				commitLineID = line.id
				lines = append(lines, graphLine{line.id, -1, commitColumn, commitColumn})
			}
			// Merge parents after the first connect to the commit point, reusing
			// the line already waiting for that parent when there is one.
			if hasFirstParent {
				for _, parent := range parents[1:] {
					bottom := 0
					matched := false
					for _, line := range *current {
						if line.next == parent {
							lines = append(lines, graphLine{line.id, -1, commitColumn, bottom})
							matched = true
							break
						}
						bottom++
					}
					if !matched {
						line := activeGraphLine{parent, nextID}
						nextID++
						lines = append(lines, graphLine{line.id, -1, commitColumn, len(*current)})
						*current = append(*current, line)
					}
				}
			}
			if len(*current) > columns {
				columns = len(*current)
			}
		}
		rows[i] = graphRow{Lines: lines, CommitColumn: commitColumn, CommitLineID: commitLineID}
		parity = 1 - parity
	}
	return rows, columns
}

// graphStroke draws one graph line segment. A segment whose ends share a
// column is straight; otherwise it bends with a cubic curve so branch joins
// read as smooth merges.
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

// graphCell draws one row of the revision graph on a fixed lane grid, so the
// rows of the list line up: lines cross the row and curve into and out of the
// commit point. A commit is a ring in its line's color; a merge opens into a
// larger circle with a chevron, as the reference marks merge commits.
func graphCell(c *ui.Context, row graphRow, columns int, merge bool) ui.Element {
	t := c.Theme()
	const laneWidth = 12
	const height = 23
	width := float32(columns)*laneWidth + laneWidth
	return ui.Box(c).Width(width).Height(height).Shrink(0).Clip().
		DrawOver(func(p *ui.Painter, r ui.Rect) {
			at := func(column int) float32 { return r.X + laneWidth/2 + float32(column)*laneWidth }
			top, mid, bottom := r.Y, r.Y+height/2, r.Y+height
			for _, line := range row.Lines {
				color := graphColor(line.ID)
				switch {
				case line.Top < 0 && line.Bottom >= 0:
					graphStroke(p, at(line.Column), mid, at(line.Bottom), bottom, color)
				case line.Top >= 0 && line.Bottom < 0:
					graphStroke(p, at(line.Top), top, at(line.Column), mid, color)
				case line.Top >= 0 && line.Bottom >= 0:
					graphStroke(p, at(line.Top), top, at(line.Bottom), bottom, color)
				}
			}
			if row.CommitColumn < 0 || row.CommitLineID < 0 {
				return
			}
			x, y := at(row.CommitColumn), mid
			color := graphColor(row.CommitLineID)
			if merge {
				p.Fill(ui.Rect{X: x - 5.75, Y: y - 5.75, W: 11.5, H: 11.5}, t.Background, 6)
				p.Stroke(ui.Rect{X: x - 5.75, Y: y - 5.75, W: 11.5, H: 11.5}, color, 5.75, 1.5)
				var path ui.Path
				path.MoveTo(x-3.5, y-1.75).LineTo(x, y+1.75).LineTo(x+3.5, y-1.75)
				p.StrokePath(&path, 1.5, color)
				return
			}
			p.Fill(ui.Rect{X: x - 4, Y: y - 4, W: 8, H: 8}, color, 4)
			p.Fill(ui.Rect{X: x - 1.75, Y: y - 1.75, W: 3.5, H: 3.5}, t.Background, 2)
		})
}

func (s *settings) chinese() bool {
	language := s.values.Language
	if language == "" {
		language = s.systemLanguage
	}
	return strings.HasPrefix(strings.ToLower(language), "zh")
}

func (s *settings) dateLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if s.chinese() {
		return t.Format("2 1月 2006 15:04")
	}
	return t.Format("Jan 2, 2006 15:04")
}
