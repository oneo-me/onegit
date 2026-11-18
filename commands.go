package main

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type commandState struct {
	open bool
}

func (s *settings) openCommands() {
	if s.open || s.choosingDirectory {
		return
	}
	s.commands.open = true
	s.invalidate()
}

// commandTrigger opens the command palette from a box shaped like a search
// field.
func (s *settings) commandTrigger(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	hint := ui.Text("⌘F", 11.5, ui.Regular, colors.Muted)
	hint.MaxLines, hint.Style.FlexShrink = 1, 0
	label := ui.Text(s.text("搜索…"), 12, ui.Regular, colors.Muted)
	label.MaxLines, label.Style.FlexGrow, label.Style.FlexShrink, label.Style.MinWidth = 1, 1, 1, core.Pt(0)
	return base.Button{Label: s.text("搜索项目与设置"), TestID: "command-trigger", OnPress: func(core.PressEvent) { s.openCommands() },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pt(28), Padding: core.Symmetric(0, 10),
			CornerRadius: 8, BorderWidth: 1},
		StyleFunc: func() core.Style {
			return core.Style{Background: ui.Alpha(colors.Surface(), .5), BorderColor: colors.Border(),
				Hover: &core.Style{BorderColor: ui.Alpha(colors.Text(), .25)}, FocusVisible: ui.FocusRing(cx, -1)}
		},
		Children: []core.Node{ui.Icon(lucide.Search, 16, colors.Muted), label, hint}}
}

// command is a palette item and what choosing it does.
type command struct {
	item base.CommandItem
	run  func()
}

func (s *settings) commandItems() []command {
	settingsGroup, projectsGroup := s.text("设置"), s.text("项目")
	items := []command{
		{base.CommandItem{ID: "settings", Label: s.text("设置"), Group: settingsGroup, Icon: lucide.Settings(), Keywords: []string{"设置", "settings", "preferences"}},
			func() { s.show("", "") }},
		{base.CommandItem{ID: "settings/theme", Label: s.text("主题"), Group: settingsGroup, Icon: lucide.Palette(), Keywords: []string{"主题", "外观", "theme", "appearance", "light", "dark"}},
			func() { s.show("appearance", "") }},
		{base.CommandItem{ID: "settings/language", Label: s.text("语言"), Group: settingsGroup, Icon: lucide.Settings(), Keywords: []string{"语言", "language", "English", "中文"}},
			func() { s.show("", s.text("语言")) }},
		{base.CommandItem{ID: "settings/directory", Label: s.text("项目父目录"), Group: settingsGroup, Icon: lucide.Folder(), Keywords: []string{"项目父目录", "路径", "文件夹", "project", "parent", "directory", "folder"}},
			func() { s.show("", s.text("项目父目录")) }},
	}
	if s.hasRemovedProjects() {
		items = append(items, command{base.CommandItem{ID: "projects/restore", Label: s.text("恢复已移除的项目"), Group: projectsGroup, Icon: lucide.Folder(),
			Keywords: []string{"恢复", "移除", "项目", "restore", "removed", "project"}}, s.restoreRemovedProjects})
	}
	var projects []command
	walkProjects(s.projectTree, func(node *projectNode) {
		if !node.Repository || node.Removed {
			return
		}
		title := filepath.Base(node.Path)
		if node.Missing {
			title += s.text("（已不存在）")
		}
		projects = append(projects, command{base.CommandItem{ID: "project/" + node.Path, Label: title, Description: node.Path, Group: projectsGroup,
			Icon: lucide.Folder(), Keywords: []string{node.Path}}, func() { s.selectProject(node) }})
	})
	sort.Slice(projects, func(i, j int) bool {
		a, b := strings.ToLower(projects[i].item.Label), strings.ToLower(projects[j].item.Label)
		if a == b {
			return projects[i].item.ID < projects[j].item.ID
		}
		return a < b
	})
	return append(items, projects...)
}

// activateCommand runs the command of id, closing the palette.
func (s *settings) activateCommand(id string) {
	for _, c := range s.commandItems() {
		if c.item.ID == id {
			s.commands.open = false
			s.invalidate()
			c.run()
			return
		}
	}
}

// commandPalette searches the projects and settings; ⌘F opens it.
func (s *settings) commandPalette(cx *core.Scope) core.Node {
	items := func() []base.CommandItem {
		s.track()
		var out []base.CommandItem
		if s.commands.open {
			for _, c := range s.commandItems() {
				out = append(out, c.item)
			}
		}
		return out
	}
	return ui.CommandDialog{TestID: "command-palette", Inset: ui.ShellHeaderHeight(cx),
		Open:         func() bool { s.track(); return s.commands.open },
		OnOpenChange: func(open bool, _ reason) { s.commands.open = open; s.invalidate() },
		Command: ui.Command{Items: items, Label: s.text("命令面板"), SearchLabel: s.text("命令面板"), Placeholder: s.text("搜索项目与设置…"),
			Empty: s.text("没有匹配的项目或设置"), AutoFocus: true, OnSelect: s.activateCommand}}
}
