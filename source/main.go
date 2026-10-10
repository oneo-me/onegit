package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	configDir := filepath.Join(home, ".oneo", "onegit")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Fatal(err)
	}
	mygo.App.SetPath(mygo.PathUserData, configDir)
	mygo.App.SetName("Onegit")
	mygo.App.WhenReady(func() {
		settings := newSettings()
		shell := sidebar.NewShell(sidebar.ShellOptions{
			Title:           "Onegit",
			Width:           1100,
			Height:          720,
			MinWidth:        820,
			MinHeight:       600,
			LeftWidth:       240,
			LeftMinWidth:    180,
			LeftMaxWidth:    400,
			LeftToggleLabel: settings.text("切换侧边栏"),
			LeftResizeLabel: settings.text("调整侧边栏宽度"),
			Left:            settings.worktreeSidebar,
			LeftToolbar:     settings.toolbar,
			Content: func(c *ui.Context) {
				settings.welcome(c)
				settings.render(c)
				settings.renderCommands(c)
			},
			ContentTitle: func(c *ui.Context) {
				components.Text(c, "Onegit").Bold()
			},
		})
		settings.window = shell.Window()
		settings.scanProjects()
		mygo.App.SetMenu(settings.menu())
		shell.Window().OnClose(func(*mygo.CloseEvent) { settings.saveSidebarState() })
		shell.Window().OnClosed(func() {
			settings.scanVersion++
			if settings.scanCancel != nil {
				settings.scanCancel()
			}
			mygo.App.Quit()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (s *settings) welcome(c *ui.Context) {
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+32, 32, 32, 32).Gap(12).Children(func() {
		if s.selectedProject != nil {
			components.Text(c, filepath.Base(s.selectedProject.Path)).FontSize(24).Bold()
			components.Text(c, s.selectedProject.Path).Wrap()
			if s.selectedProject.Missing {
				components.Text(c, s.text("（已不存在）")).Wrap()
			}
			if s.selectedProject.Repository {
				components.Text(c, s.text("Git 项目")).Bold()
			}
			return
		}
		components.Text(c, s.text("欢迎使用 Onegit")).FontSize(24).Bold()
		components.Text(c, s.text("在一个地方查看多个 Git 仓库与 worktree。")).Wrap()
		components.Text(c, s.text("从侧边栏选择项目。分支状态与实时更新将在后续实现。")).Wrap().TextColor(c.Theme().TextMuted)
	})
}
