package main

import (
	"os"
	"path/filepath"
	"runtime"
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
// project sidebar, content and Git dialogs; Onegit's title bar starts its
// actions.
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
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
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

// repositoryTitle fills the window's title bar, as Neorio does, instead of
// adding a toolbar strip: the worktree tabs, which name the project, then the
// view's fetch, pull and push with their commit counts, a stash and the branch
// menu. Every action
// runs through the view, which reports failures as toasts and rereads the
// worktree.
func (s *settings) repositoryTitle(c *ui.Context) {
	project := s.selectedProject
	if project == nil || !project.Repository {
		components.Text(c, "Onegit").Bold()
		return
	}
	view := s.gitView()
	tr := view.Tracking()
	size := max(32, c.Theme().Rem(2))
	ui.Row(c).Grow(1).MinWidth(0).FillHeight().AlignItems(ui.Center).Gap(8).Margin(0, titleTrailing(c, size), 0, 0).Children(func() {
		s.worktreeTabs(c)
		ui.Spacer(c)
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
			onClick(s.titleButton(c, gitview.OpFetch, lucide.Download, s.text("获取"), size, 0, !view.CanSync()), view.OpenFetch)
			onClick(s.titleButton(c, gitview.OpPull, lucide.ArrowDownToLine, s.text("拉取"), size, tr.Behind, !view.CanPull()), view.OpenPull)
			onClick(s.titleButton(c, gitview.OpPush, lucide.ArrowUpFromLine, s.text("推送"), size, tr.Ahead, !view.CanSync()), view.OpenPush)
			onClick(s.titleButton(c, "stash", lucide.Archive, s.text("贮藏"), size, 0, !view.CanSync()), func() {
				view.Run("stash", s.text("贮藏失败"), func(git gitview.Git, path string) error {
					_, err := git.Run(path, "stash", "push", "--include-untracked")
					return err
				})
			})
			if b := s.titleButton(c, "branch", lucide.GitBranch, s.text("分支操作"), size, 0, !view.CanSync()); !b.IsDisabled() {
				b.Menu(s.branchMenu)
			}
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

// titleButton is a round toolbar button in the title bar, raised like the
// chosen worktree tab.
// A running action shows a spinner, and count, when set, the commits it
// moves.
func (s *settings) titleButton(c *ui.Context, op string, icon func() *ui.SVG, label string, size float32, count int, disabled bool) ui.Element {
	t := c.Theme()
	view := s.gitView()
	b := components.Button(c, "", components.ButtonToolbar).Key("title-"+op).Label(label).Size(size, size).
		Padding(0).Shrink(0).Justify(ui.Center).AlignItems(ui.Center)
	if disabled {
		b.Disabled(true)
	}
	b.Children(func() {
		if view.Syncing() == op {
			components.Spinner(c, components.SpinnerRing).Size(16, 16)
		} else {
			components.Icon(c, icon).Size(16, 16).Shrink(0)
		}
		if count > 0 {
			ui.Box(c).Absolute().Top(1).Right(1).Height(14).MinWidth(14).Padding(0, 4).Radius(7).Center().
				Background(t.Accent).Children(func() {
				components.Text(c, strconv.Itoa(count)).FontSize(10).Bold().TextColor(t.AccentText).SingleLine()
			})
		}
	})
	tip := view.ActionTooltip(op)
	if tip == "" {
		tip = label
	}
	components.Tooltip(c, b, tip)
	return b
}

// onClick runs action when the enabled button b is clicked.
func onClick(b ui.Element, action func()) {
	if !b.IsDisabled() && b.Clicked() {
		action()
	}
}

// titleTrailing balances the space after the title bar actions with the space
// above them. The shell pads its header and title area; a negative margin may
// reclaim only the title area's own padding.
func titleTrailing(c *ui.Context, size float32) float32 {
	if runtime.GOOS == "windows" {
		return 0 // caption buttons follow the title area
	}
	bar := c.TitleBar()
	vertical := (sidebar.HeaderHeight(c) - size) / 2
	return max(-8, vertical-max(8, bar.Right)-8)
}

// repositoryLoading reports whether the view is reading the selected
// worktree.
func (s *settings) repositoryLoading() bool { return s.repo.view != nil && s.repo.view.Loading() }
