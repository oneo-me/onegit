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

// styles the graph lane colors, one per parallel branch line.
var graphPalette = []ui.Color{
	ui.Hex("#3E9FF8"), ui.Hex("#34C84A"), ui.Hex("#E0A33E"), ui.Hex("#C678DD"),
	ui.Hex("#E06C75"), ui.Hex("#56B6C2"), ui.Hex("#98C379"), ui.Hex("#D19A66"),
}

func graphColor(lane int) ui.Color {
	if lane < 0 {
		lane = 0
	}
	return graphPalette[lane%len(graphPalette)]
}

type graphSegment struct{ From, To int }

type graphRow struct {
	Lane     int
	Segments []graphSegment
}

// buildGraph assigns each commit a lane and the line segments that connect it
// to its parents, following the shape of the reference's revision graph.
func buildGraph(commits []commitEntry) ([]graphRow, int) {
	rows := make([]graphRow, len(commits))
	var active []string
	laneOf := func(hash string) int {
		for i, h := range active {
			if h == hash {
				return i
			}
		}
		return -1
	}
	firstFree := func() int {
		for i, h := range active {
			if h == "" {
				return i
			}
		}
		active = append(active, "")
		return len(active) - 1
	}
	lanes := 0
	for i, commit := range commits {
		lane := laneOf(commit.Hash)
		if lane < 0 {
			lane = firstFree()
			active[lane] = commit.Hash
		}
		row := graphRow{Lane: lane}
		for j, h := range active {
			if h != "" && j != lane {
				row.Segments = append(row.Segments, graphSegment{j, j})
			}
		}
		firstParentAtLane := false
		for pi, parent := range commit.Parents {
			pl := laneOf(parent)
			if pl < 0 {
				if pi == 0 {
					pl = lane
				} else {
					pl = firstFree()
				}
				active[pl] = parent
			}
			if pl != lane {
				row.Segments = append(row.Segments, graphSegment{lane, pl})
			}
			if pi == 0 && pl == lane {
				firstParentAtLane = true
			}
		}
		if !firstParentAtLane {
			active[lane] = ""
		}
		rows[i] = row
		if len(active) > lanes {
			lanes = len(active)
		}
	}
	return rows, lanes
}

// graphCell draws one row of the revision graph within a fixed lane width.
func graphCell(c *ui.Context, row graphRow, lanes int) ui.Element {
	t := c.Theme()
	const laneWidth = 12
	const height = 23
	width := float32(lanes)*laneWidth + 8
	return ui.Box(c).Width(width).Height(height).Shrink(0).Clip().
		DrawOver(func(p *ui.Painter, r ui.Rect) {
			center := func(lane int) float32 { return r.X + 6 + float32(lane)*laneWidth + laneWidth/2 }
			for _, seg := range row.Segments {
				p.Line(center(seg.From), r.Y, center(seg.To), r.Y+r.H, 1.5, graphColor(seg.To))
			}
			x, cy := center(row.Lane), r.Y+r.H/2
			p.Fill(ui.Rect{X: x - 4, Y: cy - 4, W: 8, H: 8}, graphColor(row.Lane), 4)
			p.Fill(ui.Rect{X: x - 1.75, Y: cy - 1.75, W: 3.5, H: 3.5}, t.Background, 2)
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
