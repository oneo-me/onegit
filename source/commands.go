package main

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/plugins/command"
)

type commandState struct {
	open    bool
	palette command.State
}

func (s *settings) openCommands() {
	if s.open || s.choosingDirectory {
		return
	}
	s.commands.palette.Query = ""
	s.commands.palette.ActiveID = ""
	s.commands.open = true
}

func (s *settings) commandTrigger(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Shrink(0).Padding(0, t.Space(3), t.Space(2), t.Space(3)).Children(func() {
		trigger := components.Button(c, "", components.ButtonSecondary).
			Label(s.text("搜索项目与设置")).FillWidth().MinWidth(0).
			Padding(t.Space(1.5), t.Space(2.5)).Gap(t.Space(2)).TextColor(t.TextMuted)
		tint, borderAlpha := float32(.08), float32(.18)
		if trigger.Pressed() {
			tint, borderAlpha = .18, .4
		} else if trigger.Hovered() {
			tint, borderAlpha = .12, .3
		}
		trigger.Background(t.Background.Mix(t.Text, tint).Alpha(.3)).Border(1, t.Text.Alpha(borderAlpha))
		trigger.Children(func() {
			components.Icon(c, lucide.Search()).Size(t.Space(4), t.Space(4)).Shrink(0)
			components.Text(c, s.text("搜索…")).Grow(1).MinWidth(0).SingleLine()
			components.Text(c, "⌘E").FontSize(t.Rem(.8)).Shrink(0).SingleLine()
		})
		if trigger.Clicked() {
			s.openCommands()
		}
	})
}

func (s *settings) commandItems() []command.Item {
	items := []command.Item{
		{ID: "settings", Title: s.text("设置"), Group: s.text("设置"), Icon: lucide.Settings(), Keywords: []string{"设置", "settings", "preferences"}},
		{ID: "settings/theme", Title: s.text("主题"), Group: s.text("设置"), Icon: lucide.Palette(), Keywords: []string{"主题", "外观", "theme", "appearance", "light", "dark"}},
		{ID: "settings/language", Title: s.text("语言"), Group: s.text("设置"), Icon: lucide.Settings(), Keywords: []string{"语言", "language", "English", "中文"}},
		{ID: "settings/directory", Title: s.text("项目父目录"), Group: s.text("设置"), Icon: lucide.Folder(), Keywords: []string{"项目父目录", "路径", "文件夹", "project", "parent", "directory", "folder"}},
	}
	var projects []command.Item
	walkProjects(s.projectTree, func(node *projectNode) {
		if !node.Repository {
			return
		}
		title := filepath.Base(node.Path)
		if node.Missing {
			title += s.text("（已不存在）")
		}
		projects = append(projects, command.Item{ID: "project/" + node.Path, Title: title, Description: node.Path, Group: s.text("项目"), Icon: lucide.Folder(), Keywords: []string{node.Path}})
	})
	sort.Slice(projects, func(i, j int) bool {
		a, b := strings.ToLower(projects[i].Title), strings.ToLower(projects[j].Title)
		if a == b {
			return projects[i].ID < projects[j].ID
		}
		return a < b
	})
	return append(items, projects...)
}

func (s *settings) activateCommand(id string) {
	if id == "settings" || strings.HasPrefix(id, "settings/") {
		s.commands.open = false
		s.view = components.SettingsState{}
		switch id {
		case "settings/theme":
			s.view.Category = "appearance"
		case "settings/language":
			s.view.Query = s.text("语言")
		case "settings/directory":
			s.view.Query = s.text("项目父目录")
		}
		s.show()
		return
	}
	walkProjects(s.projectTree, func(node *projectNode) {
		if node.Repository && id == "project/"+node.Path {
			s.selectedProject = node
			s.commands.open = false
		}
	})
}

func (s *settings) renderCommands(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyE) {
		s.openCommands()
	}
	var items []command.Item
	if s.commands.open {
		items = s.commandItems()
	}
	ui.Box(c).Key("command-palette").Absolute().Children(func() {
		command.CommandDialog(c, &s.commands.open, &s.commands.palette, items, command.Options{
			SearchLabel: s.text("命令面板"), Placeholder: s.text("搜索项目与设置…"),
			ResultsLabel: s.text("搜索结果"), EmptyText: s.text("没有匹配的项目或设置"),
			OnActivate: s.activateCommand,
		}, components.DialogOptions{Label: s.text("命令面板")})
	})
}
