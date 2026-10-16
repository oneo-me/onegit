package gitview

import (
	"path"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// fileKind is a file type's icon and color; an empty color uses the muted
// text color.
type fileKind struct {
	icon  func() *ui.SVG
	color string
}

// File type colors follow each ecosystem's own, darkened where needed to
// read on both light and dark backgrounds.
const (
	goColor     = "#00ADD8"
	reactColor  = "#149ECA"
	tsColor     = "#3178C6"
	jsColor     = "#D4A72C"
	gitColor    = "#F05032"
	mdColor     = "#519ABA"
	npmColor    = "#CB3837"
	imageColor  = "#A074C4"
	shellColor  = "#4EAA25"
	configColor = "#8D8D8D"
	styleColor  = "#C6538C"
)

// fileNames match whole names, case-insensitively, before extensions.
var fileNames = map[string]fileKind{
	".gitignore":        {lucide.GitBranch, gitColor},
	".gitattributes":    {lucide.GitBranch, gitColor},
	".gitmodules":       {lucide.GitBranch, gitColor},
	".gitkeep":          {lucide.GitBranch, gitColor},
	"license":           {lucide.Scale, jsColor},
	"license.md":        {lucide.Scale, jsColor},
	"license.txt":       {lucide.Scale, jsColor},
	"licence":           {lucide.Scale, jsColor},
	"copying":           {lucide.Scale, jsColor},
	"readme.md":         {lucide.BookOpen, mdColor},
	"changelog.md":      {lucide.FileClock, mdColor},
	"go.mod":            {lucide.Package, goColor},
	"go.sum":            {lucide.Lock, goColor},
	"go.work":           {lucide.Package, goColor},
	"go.work.sum":       {lucide.Lock, goColor},
	"package.json":      {lucide.Package, npmColor},
	"package-lock.json": {lucide.Lock, npmColor},
	"pnpm-lock.yaml":    {lucide.Lock, configColor},
	"yarn.lock":         {lucide.Lock, configColor},
	"bun.lock":          {lucide.Lock, configColor},
	"tsconfig.json":     {lucide.FileCog, tsColor},
	"dockerfile":        {lucide.Container, tsColor},
	"makefile":          {lucide.FileTerminal, configColor},
	".env":              {lucide.FileKey, configColor},
	".editorconfig":     {lucide.FileCog, configColor},
}

var fileExtensions = map[string]fileKind{
	".go":       {lucide.FileCode, goColor},
	".jsx":      {lucide.Atom, reactColor},
	".tsx":      {lucide.Atom, reactColor},
	".ts":       {lucide.FileCode, tsColor},
	".mts":      {lucide.FileCode, tsColor},
	".cts":      {lucide.FileCode, tsColor},
	".js":       {lucide.FileCode, jsColor},
	".mjs":      {lucide.FileCode, jsColor},
	".cjs":      {lucide.FileCode, jsColor},
	".md":       {lucide.FileText, mdColor},
	".mdx":      {lucide.FileText, mdColor},
	".markdown": {lucide.FileText, mdColor},
	".txt":      {lucide.FileText, ""},
	".json":     {lucide.FileBraces, jsColor},
	".yaml":     {lucide.FileCog, configColor},
	".yml":      {lucide.FileCog, configColor},
	".toml":     {lucide.FileCog, configColor},
	".css":      {lucide.FileCode, styleColor},
	".scss":     {lucide.FileCode, styleColor},
	".html":     {lucide.FileCode, gitColor},
	".sh":       {lucide.FileTerminal, shellColor},
	".zsh":      {lucide.FileTerminal, shellColor},
	".bash":     {lucide.FileTerminal, shellColor},
	".ps1":      {lucide.FileTerminal, tsColor},
	".png":      {lucide.FileImage, imageColor},
	".jpg":      {lucide.FileImage, imageColor},
	".jpeg":     {lucide.FileImage, imageColor},
	".gif":      {lucide.FileImage, imageColor},
	".webp":     {lucide.FileImage, imageColor},
	".bmp":      {lucide.FileImage, imageColor},
	".ico":      {lucide.FileImage, imageColor},
	".icns":     {lucide.FileImage, imageColor},
	".svg":      {lucide.FileImage, imageColor},
	".lock":     {lucide.Lock, configColor},
	".zip":      {lucide.FileArchive, configColor},
	".gz":       {lucide.FileArchive, configColor},
}

// fileKindOf chooses a file's icon by its name, then its extension.
func fileKindOf(name string) fileKind {
	lower := strings.ToLower(path.Base(name))
	if k, ok := fileNames[lower]; ok {
		return k
	}
	switch {
	case strings.HasPrefix(lower, ".env."):
		return fileNames[".env"]
	case strings.HasPrefix(lower, "license") || strings.HasPrefix(lower, "licence"):
		return fileNames["license"]
	case strings.HasPrefix(lower, "readme."):
		return fileNames["readme.md"]
	case strings.HasSuffix(lower, "_test.go"):
		return fileKind{lucide.FileCheck, goColor}
	}
	if k, ok := fileExtensions[path.Ext(lower)]; ok {
		return k
	}
	return fileKind{lucide.File, ""}
}

// fileIcon builds a file's type icon, as code editors' file trees show.
func fileIcon(c *ui.Context, name string, size float32) ui.Element {
	k := fileKindOf(name)
	color := c.Theme().TextMuted
	if k.color != "" {
		color = ui.Hex(k.color)
	}
	return components.Icon(c, k.icon).Size(size, size).Shrink(0).TextColor(color)
}

// changeTreeRow builds the content of a changes tree row: a folder icon and
// a directory's name when status is 0, or a file's type icon, its name and
// its added and deleted line counts. A file without counted lines, such as a
// binary file or a plain rename, shows Git's status letter, colored by kind.
func changeTreeRow(c *ui.Context, name string, status byte, added, deleted int) ui.Element {
	t := c.Theme()
	return ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(6).Children(func() {
		if status == 0 {
			components.Icon(c, lucide.Folder).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
			components.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13)
			return
		}
		fileIcon(c, name, 14)
		components.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13)
		if added > 0 {
			components.Text(c, "+"+strconv.Itoa(added)).Shrink(0).FontSize(12).TextColor(t.Success)
		}
		if deleted > 0 {
			components.Text(c, "−"+strconv.Itoa(deleted)).Shrink(0).FontSize(12).TextColor(t.Danger)
		}
		if added > 0 || deleted > 0 {
			return
		}
		color := t.Warning
		switch status {
		case 'A':
			color = t.Success
		case 'D':
			color = t.Danger
		case 'R':
			color = t.Accent
		}
		components.Text(c, string(status)).Shrink(0).FontSize(12).Bold().TextColor(color)
	})
}
