package main

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/oneo-me/onegit/launchers"
)

func (s *settings) theme() string {
	if s.values.Theme == "" {
		return themeSystem
	}
	return s.values.Theme
}

func (s *settings) themeLabel(value string) string {
	switch value {
	case themeLight:
		return s.text("浅色")
	case themeDark:
		return s.text("深色")
	default:
		return s.text("跟随系统")
	}
}

// appearance is the appearance a theme preference asks for.
func appearance(theme string) core.Appearance {
	switch theme {
	case themeLight:
		return core.AppearanceLight
	case themeDark:
		return core.AppearanceDark
	}
	return core.AppearanceSystem
}

func (s *settings) setTheme(value string) {
	next := s.values
	next.Theme = value
	if s.save(next) && s.cx != nil {
		s.cx.SetAppearance(appearance(value))
	}
}

func (s *settings) setLanguage(value string) {
	next := s.values
	next.Language = value
	if s.save(next) && s.cx != nil {
		s.cx.SetLocale(value)
	}
}

func (s *settings) categories(cx *core.Scope) []ui.SettingsCategory {
	description := func() string {
		s.track()
		if s.values.ProjectParentDir != "" {
			return s.values.ProjectParentDir
		}
		return s.text("尚未选择目录")
	}
	languages := []ui.Option[string]{{Value: "", Label: s.text("跟随系统")}, {Value: "zh-Hans", Label: "简体中文"}, {Value: "en", Label: "English"}}
	themes := []ui.Option[string]{{Value: themeSystem, Label: s.themeLabel(themeSystem)}, {Value: themeLight, Label: s.themeLabel(themeLight)},
		{Value: themeDark, Label: s.themeLabel(themeDark)}}
	directory := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, FlexShrink: 1, MinWidth: core.Pt(0), MaxWidth: core.Pct(100)}, Children: []core.Node{
		core.Text{ContentFunc: description, MaxLines: 1, Style: core.Style{FontSize: 12, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)},
		ui.Button{Text: s.text("选择目录…"), Variant: ui.ButtonSecondary, TestID: "choose-directory",
			Disabled: func() bool { s.track(); return s.choosingDirectory }, OnPress: s.chooseDirectory},
	}}
	return []ui.SettingsCategory{
		{ID: "general", Title: s.text("通用"), Section: s.text("应用"), Icon: lucide.Settings, Groups: []ui.SettingsGroup{
			{ID: "language", Items: []ui.SettingsItem{{ID: "language", Title: s.text("语言"), Description: s.text("修改后立即生效，并自动保存。"),
				Control: settingsSelect(s.text("语言偏好"), languages, func() string { s.track(); return s.values.Language }, s.setLanguage, "language")}}},
			{ID: "projects", Title: s.text("项目"), Items: []ui.SettingsItem{{ID: "parent-directory", Title: s.text("项目父目录"),
				Keywords: []string{"路径", "文件夹", "directory", "folder"}, Control: directory, Stacked: true}}},
		}},
		{ID: "tools", Title: s.text("工具"), Section: s.text("应用"), Icon: lucide.Wrench, Groups: []ui.SettingsGroup{{ID: "launchers", Items: []ui.SettingsItem{
			{ID: "terminal", Title: s.text("终端"), Keywords: []string{"terminal", "shell", "console", "命令行"},
				Control: s.launcherSetting(launchers.Terminal, s.text("终端"))},
			{ID: "editor", Title: s.text("开发工具"), Keywords: []string{"editor", "IDE", "code", "编辑器", "代码"},
				Control: s.launcherSetting(launchers.Editor, s.text("开发工具"))},
		}}}},
		{ID: "appearance", Title: s.text("外观"), Section: s.text("应用"), Icon: lucide.Palette, Groups: []ui.SettingsGroup{{ID: "theme", Items: []ui.SettingsItem{{
			ID: "theme", Title: s.text("主题"), Description: s.text("与侧边栏底部的主题选项同步。"),
			Control: settingsSelect(s.text("主题偏好"), themes, func() string { s.track(); return s.theme() }, s.setTheme, "theme-select"),
		}}}}},
	}
}

// settingsSelect chooses one of options for a setting.
func settingsSelect(label string, options []ui.Option[string], value func() string, change func(string), testID string) core.Node {
	return ui.Select[string]{Options: options, Label: label, Value: value, OnChange: change, TestID: testID,
		Style: core.Style{Width: core.Pt(160), MaxWidth: core.Pct(100)}}
}
