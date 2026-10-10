package main

import (
	"context"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// Project overview is a separate navigation level from a future worktree page.
type projectOverview struct {
	Worktrees     []worktree
	SelectedPath  string
	list          components.ListState
	selectedIndex int
	Loading       bool
	Problem       string
	version       uint64
	cancel        context.CancelFunc
}

func (s *settings) cancelWorktrees() {
	if s.overview.cancel != nil {
		s.overview.cancel()
	}
	s.overview = projectOverview{version: s.overview.version + 1}
}

func (s *settings) selectProject(node *projectNode, onMain func(func())) {
	s.cancelWorktrees()
	s.selectedProject = node
	if node == nil || !node.Repository {
		return
	}
	if s.state.SelectedProjects == nil {
		s.state.SelectedProjects = make(map[string]string)
	}
	s.state.SelectedProjects[s.projectRoot] = node.Path
	s.saveSidebarState()
	s.overview.Loading = true
	version := s.overview.version
	ctx, cancel := context.WithCancel(context.Background())
	s.overview.cancel = cancel
	go func() {
		defer cancel()
		items, err := listWorktrees(ctx, node.Path)
		onMain(func() {
			if s.overview.version != version {
				return
			}
			s.overview.Loading = false
			s.overview.Worktrees = items
			s.overview.SelectedPath = ""
			if err == nil {
				if len(items) > 0 {
					s.overview.SelectedPath = items[0].Path
				}
				for _, item := range items {
					if item.Path == s.state.SelectedWorktrees[node.Path] {
						s.overview.SelectedPath = item.Path
						break
					}
				}
				s.rememberWorktreeSelection()
			}
			if err != nil {
				s.overview.Problem = err.Error()
			}
			if s.window != nil {
				s.window.Invalidate()
			}
		})
	}()
}

func (s *settings) projectWorktrees(c *ui.Context) {
	if s.overview.Loading {
		components.Text(c, s.text("正在读取 worktree…")).TextColor(c.Theme().TextMuted)
		return
	}
	if s.overview.Problem != "" {
		components.Text(c, s.text("读取 worktree 失败：")+s.overview.Problem).Wrap().TextColor(c.Theme().TextMuted)
		return
	}
	overview := &s.overview
	if overview.list.Selected == nil {
		overview.selectedIndex = -1
		for i, item := range overview.Worktrees {
			if item.Path == overview.SelectedPath {
				overview.selectedIndex = i
				break
			}
		}
		overview.list = components.ListState{
			Selected: &overview.selectedIndex,
			Key:      func(i int) any { return overview.Worktrees[i].Path },
			Label:    func(i int) string { return filepath.Base(overview.Worktrees[i].Path) },
			OnChange: func() {
				overview.SelectedPath = ""
				if i := overview.selectedIndex; i >= 0 && i < len(overview.Worktrees) {
					overview.SelectedPath = overview.Worktrees[i].Path
				}
				s.rememberWorktreeSelection()
			},
		}
	}
	components.List(c, &overview.list, len(overview.Worktrees), func(i int) {
		components.Text(c, filepath.Base(overview.Worktrees[i].Path)).SingleLine().Padding(6, 8)
	}).Fill()
}

func (s *settings) rememberWorktreeSelection() {
	if s.selectedProject == nil || !s.selectedProject.Repository {
		return
	}
	if s.state.SelectedWorktrees == nil {
		s.state.SelectedWorktrees = make(map[string]string)
	}
	s.state.SelectedWorktrees[s.selectedProject.Path] = s.overview.SelectedPath
	s.saveSidebarState()
}
