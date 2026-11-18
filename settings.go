package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/oneo-me/onegit/launchers"
)

// Theme preferences: the system's appearance, or light or dark whatever it
// is.
const (
	themeSystem = "system"
	themeLight  = "light"
	themeDark   = "dark"
)

type preferences struct {
	ProjectParentDir string `json:"projectParentDir,omitempty" yaml:"projectParentDir,omitempty"`
	Theme            string `json:"theme,omitempty" yaml:"theme,omitempty"`
	Language         string `json:"language,omitempty" yaml:"language,omitempty"`
	// Terminal and Editor choose the title bar's launchers by ID, empty for
	// the first installed one or launchers.None to hide the button.
	Terminal string `json:"terminal,omitempty" yaml:"terminal,omitempty"`
	Editor   string `json:"editor,omitempty" yaml:"editor,omitempty"`
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
	err = yaml.Unmarshal(data, &p)
	if err != nil {
		return preferences{}, err
	}
	if p.Theme != "" && p.Theme != themeSystem && p.Theme != themeLight && p.Theme != themeDark {
		return preferences{}, fmt.Errorf("unknown theme %q", p.Theme)
	}
	if p.Language != "" && p.Language != "en" && p.Language != "zh-Hans" {
		return preferences{}, fmt.Errorf("unknown language %q", p.Language)
	}
	p.Terminal, p.Editor = launchers.Normalize(p.Terminal, launchers.Terminal), launchers.Normalize(p.Editor, launchers.Editor)
	return p, nil
}

func savePreferences(path string, p preferences) error {
	return saveYAML(path, p)
}

type settings struct {
	commands        commandState
	overview        projectOverview
	repo            repositoryState
	state           sidebarState
	statePath       string
	stateProblem    string
	projectRoot     string
	projectTree     *projectNode
	selectedProject *projectNode
	scanning        bool
	scanProblem     string
	scanVersion     uint64
	scanCancel      context.CancelFunc
	// open shows the settings, on category and query.
	open              bool
	category, query   *core.Signal[string]
	choosingDirectory bool
	values            preferences
	path              string
	problem           string
	choosingIcon      bool
	projectIcons      map[string][]byte
	groupDialog       groupDialog
	systemLanguage    string
	// rev counts the changes the window shows while it is built, in cx;
	// toasts shows the window's toasts and window keeps its frame.
	rev    *core.Signal[int]
	cx     *core.Scope
	toasts *ui.Toaster
	window *windowKeeper
	// openPath opens a file or folder, pickDirectory and pickIcon show the
	// system's pickers, and runLauncher starts a launcher's command;
	// tests replace them so nothing opens.
	openPath      func(path string, done func(error))
	pickDirectory func(o core.PickDirectoriesOptions, done func(path string, err error))
	pickIcon      func(o core.PickFilesOptions, done func(path string, err error))
	runLauncher   func(*exec.Cmd) error
	// installed lists this computer's launchers once found.
	installed []launchers.Installed
	// dispatch runs work's results on the engine, as Scope.Post does.
	dispatch func(func())
	// lastToast is the last toast shown; groupFrames are where the
	// sidebar's groups are, and dragTarget the group a dragged heading is
	// over.
	lastToast   ui.ToastContent
	groupFrames map[string]core.Bounds
	dragTarget  string
}

func newSettings(dir string) *settings {
	s := &settings{groupFrames: map[string]core.Bounds{}}
	var err error
	if dir != "" {
		s.path = filepath.Join(dir, "config.yaml")
		s.statePath = filepath.Join(dir, "state.yaml")
		legacyDir, legacyErr := os.UserConfigDir()
		if legacyErr == nil {
			s.values, err = loadOrMigratePreferences(s.path, filepath.Join(legacyDir, "Onegit", "settings.json"))
		} else {
			s.values, err = loadPreferences(s.path)
		}
		var stateErr error
		s.state, stateErr = loadSidebarState(s.statePath)
		if stateErr != nil {
			s.stateProblem = s.text("读取状态失败：") + stateErr.Error()
		}
	}
	if err != nil {
		s.problem = s.text("读取设置失败：") + err.Error()
	}
	return s
}

// track subscribes the binding reading it to the window's changes.
func (s *settings) track() {
	if s.rev != nil {
		s.rev.Get()
	}
}

// invalidate shows the window's state again.
func (s *settings) invalidate() {
	if s.rev != nil {
		s.rev.Update(func(n int) int { return n + 1 })
	}
}

// post runs fn on the engine, as results of background work return.
func (s *settings) post(fn func()) { s.poster()(fn) }

// poster is how results return to the engine, read on the engine before
// work starts so the work never reads the window's state.
func (s *settings) poster() func(func()) {
	if s.dispatch != nil {
		return s.dispatch
	}
	if cx := s.cx; cx != nil {
		return cx.Post
	}
	return func(func()) {}
}

// show opens the settings, on category and searching query when they are
// set.
func (s *settings) show(category, query string) {
	s.commands.open = false
	s.open = true
	if s.category != nil {
		s.category.Set(category)
		s.query.Set(query)
	}
	if s.cx != nil {
		desktop.OrderFront(s.cx)
	}
	s.invalidate()
}

// toolbar ends the sidebar: Settings, then the theme menu.
func (s *settings) toolbar(cx *core.Scope) core.Node {
	icon := func() *core.SVG {
		s.track()
		switch s.theme() {
		case themeLight:
			return lucide.Sun()
		case themeDark:
			return lucide.Moon()
		}
		return lucide.Monitor()
	}
	var items []core.Node
	for _, choice := range []string{themeSystem, themeLight, themeDark} {
		items = append(items, ui.MenuItem{Text: s.themeLabel(choice), TestID: "theme-" + choice,
			Checked: func() bool { s.track(); return s.theme() == choice }, OnSelect: func() { s.setTheme(choice) }})
	}
	return core.Group{
		ui.IconButton{Icon: lucide.Settings, Label: s.text("设置"), Tooltip: func() string { return s.text("设置") + " (⌘,)" }, TestID: "open-settings",
			Size: 32, IconSize: 16, OnPress: func() { s.show("", "") }},
		ui.Spacer{},
		ui.Menu{Label: s.text("主题"), TestID: "theme", Face: core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), AlignItems: core.AlignCenter,
			JustifyContent: core.JustifyCenter}, Children: []core.Node{ui.IconOf(icon, 16, ui.ColorsOf(cx).Muted)}},
			Children: items},
	}
}

// chooseDirectory asks for the parent directory of the projects.
func (s *settings) chooseDirectory() {
	if s.choosingDirectory {
		return
	}
	pick := s.pickDirectory
	if pick == nil {
		pick = func(o core.PickDirectoriesOptions, done func(string, error)) {
			s.cx.PickDirectories(o, func(dirs []core.Directory, err error) {
				path := ""
				if len(dirs) > 0 {
					path = dirs[0].Path
				}
				done(path, err)
			})
		}
	}
	s.choosingDirectory = true
	s.invalidate()
	opts := core.PickDirectoriesOptions{Title: s.text("选择你常用的项目所在的父目录。"), Button: s.text("选择"), Start: s.values.ProjectParentDir}
	pick(opts, func(path string, err error) {
		s.choosingDirectory = false
		s.applyDirectorySelection(path, err)
		s.invalidate()
	})
}

func (s *settings) applyDirectorySelection(path string, err error) {
	if err != nil {
		s.problem = s.text("选择目录失败：") + err.Error()
		return
	}
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		s.problem = s.text("无法访问所选目录：") + err.Error()
		return
	}
	if !info.IsDir() {
		s.problem = s.text("请选择文件夹。")
		return
	}
	s.setDirectory(filepath.Clean(path))
}

func (s *settings) setDirectory(path string) {
	next := s.values
	next.ProjectParentDir = path
	changed := next.ProjectParentDir != s.values.ProjectParentDir
	if s.save(next) && changed {
		s.scanProjects()
	}
}

func (s *settings) save(next preferences) bool {
	defer s.invalidate()
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

// settingsDialog shows the settings while they are open.
func (s *settings) settingsDialog(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return ui.SettingsDialog{Label: s.text("设置"), TestID: "settings-dialog", Inset: shellInset,
		Open:         func() bool { s.track(); return s.open },
		OnOpenChange: func(open bool, _ reason) { s.open = open; s.invalidate() },
		Content: func(narrow func() bool) core.Node {
			problem := core.Show{When: func() bool { s.track(); return s.problem != "" }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Padding: core.All(16), FlexShrink: 0}, Children: []core.Node{
					core.Text{ContentFunc: func() string { s.track(); return s.problem }, Style: core.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Danger)},
				}}
			}}
			return core.View{Style: ui.FillRest, Children: []core.Node{problem, ui.Settings{Categories: s.categories(cx), Category: s.category, Query: s.query, Narrow: narrow,
				TestID: "settings", Title: s.text("设置"), SearchLabel: s.text("搜索设置"), SearchPlaceholder: s.text("搜索设置…"),
				ResultsTitle: s.text("搜索结果"), EmptyText: s.text("没有匹配的设置"),
				OnClose: func() { s.open = false; s.invalidate() }, CloseLabel: s.text("关闭设置")}}}
		}}
}

// menu is the application menu.
func (s *settings) menu() []desktop.MenuItem {
	return []desktop.MenuItem{
		{Label: "Onegit", Items: []desktop.MenuItem{
			{Role: desktop.MenuAbout}, {Separator: true},
			{Label: s.text("设置…"), Shortcut: "mod+,", OnSelect: func() { s.show("", "") }},
			{Separator: true}, {Role: desktop.MenuServices}, {Separator: true},
			{Role: desktop.MenuHide}, {Role: desktop.MenuHideOthers}, {Role: desktop.MenuShowAll}, {Separator: true}, {Role: desktop.MenuQuit},
		}},
		{Label: s.text("文件"), Items: []desktop.MenuItem{{Role: desktop.MenuClose}}},
		{Label: s.text("编辑"), Items: []desktop.MenuItem{
			{Role: desktop.MenuUndo}, {Role: desktop.MenuRedo}, {Separator: true},
			{Role: desktop.MenuCut}, {Role: desktop.MenuCopy}, {Role: desktop.MenuPaste}, {Role: desktop.MenuSelectAll},
		}},
		{Label: s.text("显示"), Items: []desktop.MenuItem{{Role: desktop.MenuToggleFullScreen}}},
		{Label: s.text("窗口"), Items: []desktop.MenuItem{{Role: desktop.MenuMinimize}, {Role: desktop.MenuZoom}, {Separator: true}, {Role: desktop.MenuBringAllToFront}}},
	}
}

// Import the old preferences only when the new config does not exist.
func loadOrMigratePreferences(path, legacy string) (preferences, error) {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return loadPreferences(path)
	}
	if _, err := os.Stat(legacy); errors.Is(err, os.ErrNotExist) {
		return preferences{}, nil
	}
	p, err := loadPreferences(legacy)
	if err != nil {
		return p, err
	}
	return p, savePreferences(path, p)
}

// Move the earlier ~/.oneo/onegit directory once, when the data directory does not exist.
func migrateDataDir(dir, legacy string) error {
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(legacy); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return os.Rename(legacy, dir)
}
