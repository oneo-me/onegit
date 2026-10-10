package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

func main() {
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
			},
			ContentTitle: func(c *ui.Context) {
				components.Text(c, "Onegit").Bold()
			},
		})
		settings.window = shell.Window()
		mygo.App.SetMenu(settings.menu())
		shell.Window().OnClosed(func() { mygo.App.Quit() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (s *settings) worktreeSidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+16, 16, 16, 16).Gap(12).Children(func() {
		components.Text(c, "Worktrees").Bold()
		components.Text(c, s.text("尚未接入仓库")).Wrap().TextColor(c.Theme().TextMuted)
	})
}

func (s *settings) welcome(c *ui.Context) {
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c)+32, 32, 32, 32).Gap(12).Children(func() {
		components.Text(c, s.text("欢迎使用 Onegit")).FontSize(24).Bold()
		components.Text(c, s.text("在一个地方查看多个 Git 仓库与 worktree。")).Wrap()
		components.Text(c, s.text("应用已初始化。仓库接入、分支状态与实时更新将在后续实现。")).Wrap().TextColor(c.Theme().TextMuted)
	})
}
