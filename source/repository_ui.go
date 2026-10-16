package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onegit/source/gitview"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

// repositoryState holds the selected worktree's Git view. The view owns the
// project sidebar, content and Git dialogs; Onegit owns the toolbar that
// starts its actions.
type repositoryState struct {
	view     *gitview.View
	dispatch func(func())
}

func (s *settings) repositoryDispatch() func(func()) {
	if s.repo.dispatch != nil {
		return s.repo.dispatch
	}
	return mygo.RunOnMain
}

func (s *settings) invalidate() {
	if s.window != nil {
		s.window.Invalidate()
	}
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
		Dispatch:   func(fn func()) { s.repositoryDispatch()(fn) },
		Invalidate: s.invalidate,
		Git:        gitview.Git{Env: gitEnv()},
		Language:   s.language(),
		AvatarDir:  avatars,
		State:      state,
		OnState: func(state gitview.State) {
			s.state.GitView = &state
			s.saveSidebarState()
		},
		DialogInset: sidebar.HeaderHeight,
	})
	return s.repo.view
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

// welcome builds the repository page of the selected project.
func (s *settings) welcome(c *ui.Context) {
	if s.selectedProject == nil || !s.selectedProject.Repository {
		return
	}
	s.repositoryPage(c)
}

func (s *settings) repositoryPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		s.worktreeTabs(c)
		s.repositoryToolbar(c)
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
		if s.overview.Loading {
			s.repositoryMessage(c, s.text("正在读取 worktree…"))
			return
		}
		if s.overview.Problem != "" {
			s.repositoryMessage(c, s.text("读取 worktree 失败：")+s.overview.Problem)
			return
		}
		view := s.gitView()
		// The language preference can change while the view lives.
		view.SetLanguage(s.language())
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() { view.Layout(c) })
	})
}

func (s *settings) repositoryMessage(c *ui.Context, message string) {
	ui.Column(c).Grow(1).FillWidth().AlignItems(ui.Center).Justify(ui.Center).Padding(16).Children(func() {
		components.Text(c, message).Wrap().TextColor(c.Theme().TextMuted)
	})
}

// repositoryToolbar is the strip below the worktree tabs: the view's fetch,
// pull and push with their commit counts, a stash, and the branch menu.
// Every action runs through the view, which reports failures as toasts and
// rereads the worktree.
func (s *settings) repositoryToolbar(c *ui.Context) {
	t := c.Theme()
	view := s.gitView()
	tr := view.Tracking()
	ui.Row(c).FillWidth().Shrink(0).Height(t.Space(12)).AlignItems(ui.Center).
		Background(t.Background).Padding(0, t.Space(1)).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(.5)).Children(func() {
			s.toolbarButton(c, gitview.OpFetch, lucide.Download(), s.text("获取"), 0, !view.CanSync(), view.Fetch)
			s.toolbarButton(c, gitview.OpPull, lucide.ArrowDownToLine(), s.text("拉取"), tr.Behind, !view.CanPull(), view.OpenPull)
			s.toolbarButton(c, gitview.OpPush, lucide.ArrowUpFromLine(), s.text("推送"), tr.Ahead, !view.CanSync(), view.OpenPush)
			s.toolbarButton(c, "stash", lucide.Archive(), s.text("贮藏"), 0, !view.CanSync(), func() {
				view.Run("stash", s.text("贮藏失败"), func(git gitview.Git, path string) error {
					_, err := git.Run(path, "stash", "push", "--include-untracked")
					return err
				})
			})
		})
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Justify(ui.End).Gap(t.Space(.5)).Children(func() {
			s.toolbarMenuButton(c, lucide.GitBranch(), s.text("分支操作"), !view.CanSync(), s.branchMenu)
		})
	})
}

// branchMenu switches the worktree to another local branch.
func (s *settings) branchMenu(m *ui.Menu) {
	view := s.gitView()
	current := view.Tracking().Branch
	branches := view.LocalBranches()
	for _, branch := range branches {
		if m.Item(branch).Checked(branch == current).Chosen() && branch != current {
			view.Run("switch", s.text("切换分支失败"), func(git gitview.Git, path string) error {
				_, err := git.Run(path, "switch", branch)
				return err
			})
		}
	}
	if len(branches) == 0 {
		m.Item(s.text("没有分支。")).Disabled(true)
	}
}

// toolbarButton is a toolbar action with an icon over its label, sized like
// the rest of the toolbar rather than Onevra's square icon button. A
// running action shows a spinner, and count, when set, the commits it moves.
func (s *settings) toolbarButton(c *ui.Context, op string, icon *ui.SVG, label string, count int, disabled bool, action func()) {
	t := c.Theme()
	view := s.gitView()
	b := ui.ButtonBase(c).Key("toolbar-" + op).Width(t.Space(15)).Height(t.Space(12)).
		Column().Gap(t.Space(.5)).Radius(t.Radius).Label(label).TextColor(t.TextMuted)
	if disabled {
		b.Disabled(true)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		if b.IsDisabled() {
			return
		}
		switch {
		case b.Pressed():
			p.Fill(r, t.Text.Alpha(.14), t.Radius)
		case b.Hovered():
			p.Fill(r, t.Text.Alpha(.08), t.Radius)
		}
	}).Children(func() {
		ui.Box(c).Size(t.Space(4.5), t.Space(4.5)).Children(func() {
			if view.Syncing() == op {
				components.Spinner(c, components.SpinnerRing).Fill()
			} else {
				components.Icon(c, icon).Fill().TextColor(t.Text)
			}
			if count > 0 {
				ui.Box(c).Absolute().Top(-4).Left(t.Space(3)).Height(14).Padding(0, 4).Radius(7).Center().
					Background(t.Accent).Children(func() {
					components.Text(c, strconv.Itoa(count)).FontSize(10).Bold().TextColor(ui.RGB(255, 255, 255)).SingleLine()
				})
			}
		})
		components.Text(c, label).FontSize(t.Rem(.68)).SingleLine()
	})
	tip := view.ActionTooltip(op)
	if tip == "" {
		tip = label
	}
	components.Tooltip(c, b, tip)
	if !disabled && b.Clicked() {
		action()
	}
}

// toolbarMenuButton is toolbarButton with a dropdown menu.
func (s *settings) toolbarMenuButton(c *ui.Context, icon *ui.SVG, label string, disabled bool, menu func(*ui.Menu)) {
	t := c.Theme()
	b := ui.ButtonBase(c).Key("toolbar-menu-" + label).Width(t.Space(15)).Height(t.Space(12)).
		Column().Gap(t.Space(.5)).Radius(t.Radius).Label(label).TextColor(t.TextMuted)
	if disabled {
		b.Disabled(true)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		if b.IsDisabled() {
			return
		}
		switch {
		case b.Pressed():
			p.Fill(r, t.Text.Alpha(.14), t.Radius)
		case b.Hovered():
			p.Fill(r, t.Text.Alpha(.08), t.Radius)
		}
	}).Children(func() {
		components.Icon(c, icon).Size(t.Space(4.5), t.Space(4.5)).TextColor(t.Text)
		components.Text(c, label).FontSize(t.Rem(.68)).SingleLine()
	})
	if !disabled {
		b.Menu(menu)
	}
	components.Tooltip(c, b, label)
}

// repositoryLoading reports whether the view is reading the selected
// worktree.
func (s *settings) repositoryLoading() bool { return s.repo.view != nil && s.repo.view.Loading() }
