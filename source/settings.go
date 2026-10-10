package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

type preferences struct {
	ProjectParentDir string           `json:"projectParentDir,omitempty"`
	Theme            mygo.ThemeSource `json:"theme,omitempty"`
	Language         string           `json:"language,omitempty"`
}

func loadPreferences(path string) (preferences, error) {
	var p preferences
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(data, &p)
	if err != nil {
		return preferences{}, err
	}
	if p.Theme != "" && p.Theme != mygo.ThemeSystem && p.Theme != mygo.ThemeLight && p.Theme != mygo.ThemeDark {
		return preferences{}, fmt.Errorf("unknown theme %q", p.Theme)
	}
	if p.Language != "" && p.Language != "en" && p.Language != "zh-Hans" {
		return preferences{}, fmt.Errorf("unknown language %q", p.Language)
	}
	return p, nil
}

func savePreferences(path string, p preferences) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

type settings struct {
	open              bool
	choosingDirectory bool
	view              components.SettingsState
	values            preferences
	path              string
	problem           string
	window            *mygo.Window
	systemLanguage    string
}

func newSettings() *settings {
	s := &settings{systemLanguage: mygo.App.Locale()}
	dir, err := os.UserConfigDir()
	if err == nil {
		s.path = filepath.Join(dir, "Onegit", "settings.json")
		s.values, err = loadPreferences(s.path)
	}
	if err != nil {
		s.problem = s.text("读取设置失败：") + err.Error()
	}
	mygo.Theme.SetSource(s.theme())
	return s
}

func (s *settings) show() {
	s.open = true
	if s.window != nil {
		s.window.Show()
		s.window.Focus()
		s.window.Invalidate()
	}
}

func (s *settings) toolbar(c *ui.Context) {
	if sidebar.IconButton(c, lucide.Settings(), s.text("设置")).Clicked() {
		s.show()
	}
	ui.Spacer(c)
	icon := lucide.Monitor()
	switch s.theme() {
	case mygo.ThemeLight:
		icon = lucide.Sun()
	case mygo.ThemeDark:
		icon = lucide.Moon()
	}
	sidebar.IconButton(c, icon, s.text("主题")).Menu(func(m *ui.Menu) {
		for _, choice := range []mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeLight, mygo.ThemeDark} {
			if m.Item(s.themeLabel(choice)).Checked(s.theme() == choice).Chosen() {
				s.setTheme(choice)
			}
		}
	})
}

func (s *settings) chooseDirectory() {
	s.startDirectorySelection(mygo.Dialog.Open, mygo.RunOnMain)
}

func (s *settings) startDirectorySelection(open func(mygo.OpenDialogOptions) ([]string, error), onMain func(func())) {
	if s.choosingDirectory {
		return
	}
	s.choosingDirectory = true
	opts := mygo.OpenDialogOptions{
		Parent: s.window, Title: s.text("选择项目父目录"), Message: s.text("选择你常用的项目所在的父目录。"),
		ButtonLabel: s.text("选择"), Directory: true, DefaultPath: s.values.ProjectParentDir,
	}
	// Dialog.Open waits for dismissal. Never wait inside a UI frame: native
	// events must be able to finish that frame while the sheet is open.
	go func() {
		paths, err := open(opts)
		onMain(func() {
			s.choosingDirectory = false
			s.applyDirectorySelection(paths, err)
			if s.window != nil {
				s.window.Invalidate()
			}
		})
	}()
}

func (s *settings) applyDirectorySelection(paths []string, err error) {
	if err != nil {
		s.problem = s.text("选择目录失败：") + err.Error()
		return
	}
	if len(paths) == 0 {
		return
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		s.problem = s.text("无法访问所选目录：") + err.Error()
		return
	}
	if !info.IsDir() {
		s.problem = s.text("请选择文件夹。")
		return
	}
	s.setDirectory(filepath.Clean(paths[0]))
}

func (s *settings) setDirectory(path string) {
	next := s.values
	next.ProjectParentDir = path
	s.save(next)
}

func (s *settings) save(next preferences) bool {
	if s.path == "" {
		s.problem = s.text("无法确定设置保存位置。")
		return false
	}
	if err := savePreferences(s.path, next); err != nil {
		s.problem = fmt.Sprintf(s.text("保存失败，设置未更改：%v"), err)
		return false
	}
	s.values, s.problem = next, ""
	return true
}

func (s *settings) render(c *ui.Context) {
	if !s.open {
		return
	}
	t := c.Theme()
	w, h := c.Size()
	inset := sidebar.HeaderHeight(c)
	width := min(t.Space(200), w*.88, max(0, w-2*inset))
	components.Dialog(c, &s.open, components.DialogOptions{Label: s.text("设置"), Inset: inset}, func(panel ui.Element) {
		panel.Width(width).Height(min(t.Space(180), h*.85)).Padding(0).Gap(0).Clip()
		if s.problem != "" {
			ui.Column(c).FillWidth().Shrink(0).Padding(16).Children(func() { components.Text(c, s.problem).Wrap() })
		}
		components.Settings(c, &s.view, s.categories(), components.SettingsOptions{
			Narrow: width < t.Space(170), Title: s.text("设置"), SearchLabel: s.text("搜索设置"), SearchPlaceholder: s.text("搜索设置…"), CategoryLabel: s.text("设置分类"), ResultsTitle: s.text("搜索结果"), EmptyText: s.text("没有匹配的设置"),
			HeaderActions: func(c *ui.Context) {
				if sidebar.IconButton(c, lucide.X(), s.text("关闭设置")).Clicked() {
					s.open = false
				}
			},
		})
	})
}

func (s *settings) menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout}, mygo.Separator(),
			{Label: s.text("设置…"), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) { s.show() }},
			mygo.Separator(), {Role: mygo.RoleServices}, mygo.Separator(),
			{Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide}, mygo.Separator(), {Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleFileMenu}, {Role: mygo.RoleEditMenu}, {Role: mygo.RoleViewMenu}, {Role: mygo.RoleWindowMenu},
	})
}
