package gitview

import "github.com/egoist/mygo/ui"

// Ref labels borrow Fork's branch colors: a border in the graph's palette
// hue over a tint of it, keyed by the column of the commit they decorate.
// The graph's own lines stay bright in both themes, while Fork mutes these
// label colors in the dark one, so each theme has its own pair of palettes.
var (
	refBordersLight = []ui.Color{
		ui.Hex("#FF9502"), ui.Hex("#FFCC00"), ui.Hex("#FF3B30"), ui.Hex("#A2845E"),
		ui.Hex("#64DA38"), ui.Hex("#1CADF8"), ui.Hex("#CB73E1"), ui.Hex("#8E8E91"),
		ui.Hex("#FF2968"), ui.Hex("#30D5C8"), ui.Hex("#5856D6"), ui.Hex("#B4D435"),
		ui.Hex("#FF6F61"),
	}
	refBackgroundsLight = []ui.Color{
		ui.Hex("#FFF2DD"), ui.Hex("#FFF9DC"), ui.Hex("#FFE5E4"), ui.Hex("#F3F0EA"),
		ui.Hex("#E9FBE4"), ui.Hex("#DFF5FF"), ui.Hex("#FAECFC"), ui.Hex("#F1F1F1"),
		ui.Hex("#FFE3EC"), ui.Hex("#DEF7F5"), ui.Hex("#E5E5F7"), ui.Hex("#F2F9DE"),
		ui.Hex("#FFE5E2"),
	}
	refBordersDark = []ui.Color{
		ui.Hex("#F28B1F"), ui.Hex("#BF9A2F"), ui.Hex("#CB2327"), ui.Hex("#A68357"),
		ui.Hex("#27A649"), ui.Hex("#0082BA"), ui.Hex("#9D53A0"), ui.Hex("#6D6D6E"),
		ui.Hex("#CC1F56"), ui.Hex("#20A89E"), ui.Hex("#4A48B0"), ui.Hex("#8DAA28"),
		ui.Hex("#E05A4D"),
	}
	refBackgroundsDark = []ui.Color{
		ui.Hex("#5D3D14"), ui.Hex("#5B4C0E"), ui.Hex("#5F2425"), ui.Hex("#433A32"),
		ui.Hex("#285224"), ui.Hex("#13445B"), ui.Hex("#503455"), ui.Hex("#3D3D3F"),
		ui.Hex("#601E35"), ui.Hex("#1A4A45"), ui.Hex("#272650"), ui.Hex("#3D4E1A"),
		ui.Hex("#5D2A24"),
	}
)

// refColors returns the border and background of a ref's label at a commit
// in the graph's column, wrapping around the palette as Fork does. Every
// label is outlined, so a tip's label matches the hue of its own line.
func refColors(column int, dark bool) (border, background ui.Color) {
	borders, backgrounds := refBordersLight, refBackgroundsLight
	if dark {
		borders, backgrounds = refBordersDark, refBackgroundsDark
	}
	i := max(column, 0) % len(borders)
	return borders[i], backgrounds[i]
}

// tagColors returns the border and background of a tag's label, which Fork
// colors apart from branches with one lavender hue in either theme.
func tagColors(dark bool) (border, background ui.Color) {
	if dark {
		return ui.Hex("#B5B1F6"), ui.Hex("#3E3F4E")
	}
	return ui.Hex("#B5B1F6"), ui.Hex("#F6F5FE")
}
