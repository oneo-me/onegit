package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/oneo-me/onegit/gitview"
)

// The sidebar's width when the window first opens, and its limits.
const (
	sidebarWidth, sidebarMinWidth, sidebarMaxWidth = 200, 160, 400
)

// reason is why a dialog asks to close, which the window ignores.
type reason = base.Reason

// repositoryState holds the selected worktree's Git view. The view owns the
// project sidebar, content and Git dialogs; Onegit's title bar starts its
// actions.
type repositoryState struct {
	view *gitview.View
}

// language is the effective language tag: the preference, else the system's.
func (s *settings) language() string {
	if s.values.Language != "" {
		return s.values.Language
	}
	return s.systemLanguage
}

// gitView returns the repository view, creating it on first use with the
// remembered layout. Avatars are cached beside the configuration.
func (s *settings) gitView() *gitview.View {
	if s.repo.view != nil {
		return s.repo.view
	}
	var state gitview.State
	if s.state.GitView != nil {
		state = *s.state.GitView
	}
	avatars := ""
	if s.path != "" {
		avatars = filepath.Join(filepath.Dir(s.path), "avatars")
	}
	s.repo.view = gitview.New(gitview.Options{
		Dispatch: s.post,
		// The title bar follows the view, such as its GitHub page.
		Invalidate: s.invalidate,
		Git:        gitview.Git{Env: gitEnv()},
		Language:   s.language(),
		// Without a window, as in tests, failures stay unshown.
		Toaster:   s.toasts,
		AvatarDir: avatars,
		State:     state,
		OnState: func(state gitview.State) {
			s.state.GitView = &state
			s.saveSidebarState()
		},
		DialogInset: shellInset,
		// Files open in their default application or show in the file
		// manager.
		OpenFile:   s.openFile,
		RevealFile: s.revealFile,
		// A worktree, or a branch another worktree has checked out, switches
		// the page to that worktree.
		OpenWorktree: s.openWorktree,
		// The refs pane lists the worktrees, including one added from a
		// ref's menu once Git lists it.
		Worktrees: true,
		OnWrite: func(op, _ string) {
			if op == gitview.OpWorktree {
				s.reloadWorktrees()
			}
		},
	})
	return s.repo.view
}

// shellInset keeps dialogs below the window's title bar.
const shellInset = 52

// openFile opens a file in its default application, or a folder in the
// file manager, waiting off the engine for the answer.
func (s *settings) openFile(path string) error {
	done := make(chan error, 1)
	s.post(func() {
		if s.cx == nil {
			done <- nil
			return
		}
		s.cx.OpenFile(core.SharedFile{Path: path}, func(_ bool, err error) { done <- err })
	})
	return <-done
}

// revealFile shows a path in the file manager.
func (s *settings) revealFile(path string) {
	s.post(func() {
		if s.cx != nil {
			desktop.RevealFile(s.cx, path, nil)
		}
	})
}

// gitEnv drops Git overrides from a launcher or development shell, which
// would otherwise describe every path as the launcher's repository.
func gitEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return env
}

// repositoryChanged shows the selected worktree, rereading it when it was
// already shown.
func (s *settings) repositoryChanged() {
	view := s.gitView()
	if view.Path() == s.overview.SelectedPath {
		view.Reload()
		return
	}
	view.SetPath(s.overview.SelectedPath)
}

// repositoryPage is the selected project's page: its Git view once its
// worktrees are read.
func (s *settings) repositoryPage(cx *core.Scope) core.Node {
	state := core.NewMemo(cx, func() string {
		s.track()
		switch {
		case s.selectedProject == nil || !s.selectedProject.Repository:
			return ""
		case s.overview.Loading:
			return "loading"
		case s.overview.Problem != "":
			return "problem:" + s.overview.Problem
		}
		return "view"
	})
	view := s.gitView().Node()
	return core.Dynamic(func(cx *core.Scope) core.Node {
		st := state.Get()
		var node core.Node
		cx.Untrack(func() {
			switch {
			case st == "":
			case st == "loading":
				node = s.repositoryMessage(cx, s.text("正在读取 worktree…"))
			case strings.HasPrefix(st, "problem:"):
				node = s.repositoryMessage(cx, s.text("读取 worktree 失败：")+s.overview.Problem)
			default:
				node = view
			}
		})
		return node
	})
}

func (s *settings) repositoryMessage(cx *core.Scope, message string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: ui.FillRest.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(16)}),
		Children: []core.Node{core.Text{Content: message, Style: core.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Muted)}}}
}

// repositoryTitle fills the window's title bar, instead of adding a toolbar
// strip: the project's name, the view's toolbar centered between the sides,
// and the launchers.
func (s *settings) repositoryTitle(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	shown := core.NewMemo(cx, func() string {
		s.track()
		if p := s.selectedProject; p != nil && p.Repository {
			return p.Path
		}
		return ""
	})
	toolbar := s.gitView().Toolbar(32)
	return core.Dynamic(func(cx *core.Scope) core.Node {
		path := shown.Get()
		if path == "" {
			return line("Onegit", 13, ui.Bold, colors.Text)
		}
		var node core.Node
		cx.Untrack(func() {
			name := line(projectName(path), 13, ui.Bold, colors.Text)
			// Equal sides keep the toolbar centered; the launchers' side
			// never shrinks below them, and the name gives way first.
			side := func(justify core.Justify, children ...core.Node) core.Node {
				return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: justify, Gap: 6,
					FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0)}, Children: children}
			}
			node = core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
				Children: []core.Node{side(core.JustifyStart, name), toolbar, side(core.JustifyEnd, s.launcherButtons(cx))}}
		})
		return node
	})
}

// line is a single line of text, cut where it runs out of room.
func line(text string, size core.Points, weight core.FontWeight, color func() core.Color) core.Text {
	t := ui.Text(text, size, weight, color)
	t.MaxLines = 1
	t.Style.FlexShrink, t.Style.MinWidth = 1, core.Pt(0)
	return t
}

// repositoryLoading reports whether the view is reading the selected
// worktree.
func (s *settings) repositoryLoading() bool { return s.repo.view != nil && s.repo.view.Loading() }

// region builds its nodes again only when key changes, so the controls
// inside keep their state as the rest of the window changes.
func region(cx *core.Scope, key func() string, build func(cx *core.Scope) core.Node) core.Node {
	k := core.NewMemo(cx, key)
	return core.Dynamic(func(cx *core.Scope) core.Node {
		k.Get()
		var node core.Node
		cx.Untrack(func() { node = build(cx) })
		return node
	})
}

// root is the app: the main window's sidebar of projects beside the
// selected one's page, with the settings, the command palette and the
// toasts over them.
func (s *settings) root(cx *core.Scope) core.Node {
	s.systemLanguage = core.Untracked(cx, cx.Env().Locale)
	s.attach(cx)
	if s.repo.view != nil {
		s.repo.view.SetLanguage(s.language())
	}
	// The warm palette of NSGo's Warm Dusk theme.
	cx.SetTheme(ui.WarmDuskTheme())
	cx.SetAppearance(appearance(s.theme()))
	cx.SetLocale(s.values.Language)
	s.keepWindow(cx)
	s.gitView().Bind(cx)
	// The system's language changes follow while the preference is unset.
	cx.Effect(func() {
		system := cx.Env().Locale()
		cx.Untrack(func() {
			if system != s.systemLanguage {
				s.systemLanguage = system
				s.invalidate()
			}
		})
	})
	cx.Effect(func() {
		s.track()
		language := s.language()
		cx.Untrack(func() {
			if s.repo.view != nil {
				s.repo.view.SetLanguage(language)
			}
			desktop.SetApplicationMenu(cx, s.menu())
		})
	})
	s.scanProjects()
	s.detectLaunchers()
	return s.localized(cx, func(cx *core.Scope) core.Node {
		return core.Group{s.windowContent(cx), s.settingsDialog(cx), s.commandPalette(cx), s.groupDialogView(cx),
			ui.Toasts{Toaster: s.toasts, Placement: ui.ToastBottomEnd, CloseLabel: s.text("关闭")}}
	})
}

// localized builds the window's nodes again whenever the language changes,
// as their words do.
func (s *settings) localized(cx *core.Scope, build func(cx *core.Scope) core.Node) core.Node {
	language := core.NewMemo(cx, func() string { s.track(); return s.language() })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		language.Get()
		var node core.Node
		cx.Untrack(func() { node = build(cx) })
		return node
	})
}

// attach keeps the window's state in cx, the scope it shows in.
func (s *settings) attach(cx *core.Scope) {
	s.cx, s.rev = cx, core.NewSignal(cx, 0)
	s.toasts = ui.NewToaster(cx)
	s.category, s.query = core.NewSignal(cx, ""), core.NewSignal(cx, "")
	cx.OnKey("mod+,", func() { s.show("", "") })
	cx.OnKey("mod+f", s.openCommands)
}

// windowContent is the main window's content.
func (s *settings) windowContent(cx *core.Scope) core.Node {
	k := s.window
	// The sidebar shows the window's material, the content its background.
	return ui.SidebarShell{Variant: ui.SidebarShellPlain,
		Open: k.open.Get, OnOpenChange: k.open.Set, ToggleLabel: s.text("切换侧边栏"),
		Width: k.width.Get, OnWidthChange: k.width.Set, ResizeLabel: s.text("调整侧边栏宽度"),
		DefaultWidth: sidebarWidth, MinWidth: sidebarMinWidth, MaxWidth: sidebarMaxWidth,
		Sidebar:       []core.Node{s.worktreeSidebar(cx)},
		SidebarFooter: []core.Node{s.toolbar(cx)},
		Header:        []core.Node{s.repositoryTitle(cx)},
		Content:       []core.Node{s.repositoryPage(cx)},
	}
}

// close ends the window's work: its reads and the Git view's.
func (s *settings) close() {
	s.saveSidebarState()
	s.cancelWorktrees()
	if s.repo.view != nil {
		s.repo.view.Close()
	}
	s.scanVersion++
	if s.scanCancel != nil {
		s.scanCancel()
	}
}
