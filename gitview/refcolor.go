package gitview

import "github.com/nsgo-dev/nsgo/core"

// graphPalette colors the revision graph's lines, so parallel branches stay
// distinguishable.
var graphPalette = []core.Color{
	core.RGB(0xFF9502), core.RGB(0xFFCC00), core.RGB(0xFF3B30), core.RGB(0xA2845E),
	core.RGB(0x64DA38), core.RGB(0x1CADF8), core.RGB(0xCB73E1), core.RGB(0x8E8E91),
	core.RGB(0xFF2968), core.RGB(0x30D5C8), core.RGB(0x5856D6), core.RGB(0xB4D435),
	core.RGB(0xFF6F61),
}

// graphColor is a graph line's color.
func graphColor(line int) core.Color { return graphPalette[max(line, 0)%len(graphPalette)] }

// Ref labels borrow Fork's branch colors: a border in the graph's palette
// hue over a tint of it, keyed by the column of the commit they decorate.
// The graph's own lines stay bright in both themes, while Fork mutes these
// label colors in the dark one, so each theme has its own pair of palettes.
var (
	refBordersLight = []core.Color{
		core.RGB(0xFF9502), core.RGB(0xFFCC00), core.RGB(0xFF3B30), core.RGB(0xA2845E),
		core.RGB(0x64DA38), core.RGB(0x1CADF8), core.RGB(0xCB73E1), core.RGB(0x8E8E91),
		core.RGB(0xFF2968), core.RGB(0x30D5C8), core.RGB(0x5856D6), core.RGB(0xB4D435),
		core.RGB(0xFF6F61),
	}
	refBackgroundsLight = []core.Color{
		core.RGB(0xFFF2DD), core.RGB(0xFFF9DC), core.RGB(0xFFE5E4), core.RGB(0xF3F0EA),
		core.RGB(0xE9FBE4), core.RGB(0xDFF5FF), core.RGB(0xFAECFC), core.RGB(0xF1F1F1),
		core.RGB(0xFFE3EC), core.RGB(0xDEF7F5), core.RGB(0xE5E5F7), core.RGB(0xF2F9DE),
		core.RGB(0xFFE5E2),
	}
	refBordersDark = []core.Color{
		core.RGB(0xF28B1F), core.RGB(0xBF9A2F), core.RGB(0xCB2327), core.RGB(0xA68357),
		core.RGB(0x27A649), core.RGB(0x0082BA), core.RGB(0x9D53A0), core.RGB(0x6D6D6E),
		core.RGB(0xCC1F56), core.RGB(0x20A89E), core.RGB(0x4A48B0), core.RGB(0x8DAA28),
		core.RGB(0xE05A4D),
	}
	refBackgroundsDark = []core.Color{
		core.RGB(0x5D3D14), core.RGB(0x5B4C0E), core.RGB(0x5F2425), core.RGB(0x433A32),
		core.RGB(0x285224), core.RGB(0x13445B), core.RGB(0x503455), core.RGB(0x3D3D3F),
		core.RGB(0x601E35), core.RGB(0x1A4A45), core.RGB(0x272650), core.RGB(0x3D4E1A),
		core.RGB(0x5D2A24),
	}
)

// refColors returns the border and background of a ref's label at a commit
// in the graph's column, wrapping around the palette as Fork does. Every
// label is outlined, so a tip's label matches the hue of its own line.
func refColors(column int, dark bool) (border, background core.Color) {
	borders, backgrounds := refBordersLight, refBackgroundsLight
	if dark {
		borders, backgrounds = refBordersDark, refBackgroundsDark
	}
	i := max(column, 0) % len(borders)
	return borders[i], backgrounds[i]
}

// tagColors returns the border and background of a tag's label, which Fork
// colors apart from branches with one lavender hue in either theme.
func tagColors(dark bool) (border, background core.Color) {
	if dark {
		return core.RGB(0xB5B1F6), core.RGB(0x3E3F4E)
	}
	return core.RGB(0xB5B1F6), core.RGB(0xF6F5FE)
}
