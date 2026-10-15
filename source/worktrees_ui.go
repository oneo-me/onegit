package main

import (
	"context"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// projectOverview owns the worktree tabs of the selected repository. The tab
// strip replaces the former worktree list: each worktree is one tab, and the
// strip stays hidden while the main worktree is the only entry.
type projectOverview struct {
	Worktrees     []worktree
	SelectedPath  string
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
	s.repo.reset()
}

func (s *settings) selectProject(node *projectNode, onMain func(func())) {
	s.cancelWorktrees()
	s.repo.dispatch = onMain
	s.selectedProject = node
	s.repo.section = s.sectionFor(node)
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
				s.applyWorktreeSelection(node.Path)
			}
			if err != nil {
				s.overview.Problem = err.Error()
			}
			s.repositoryChanged()
			if s.window != nil {
				s.window.Invalidate()
			}
		})
	}()
}

// applyWorktreeSelection restores the remembered worktree of a repository and
// falls back to the first entry Git returned.
func (s *settings) applyWorktreeSelection(project string) {
	s.overview.SelectedPath = ""
	s.overview.selectedIndex = -1
	if len(s.overview.Worktrees) == 0 {
		return
	}
	wanted := s.state.SelectedWorktrees[project]
	for i, item := range s.overview.Worktrees {
		if item.Path == wanted {
			s.overview.SelectedPath = item.Path
			s.overview.selectedIndex = i
			break
		}
	}
	if s.overview.SelectedPath == "" {
		s.overview.SelectedPath = s.overview.Worktrees[0].Path
		s.overview.selectedIndex = 0
	}
	s.rememberWorktreeSelection()
}

// selectWorktreeTab switches the active worktree and reloads its repository
// data. It is a navigation change, never a Git branch checkout.
func (s *settings) selectWorktreeTab(index int) {
	if index < 0 || index >= len(s.overview.Worktrees) || index == s.overview.selectedIndex {
		return
	}
	s.overview.selectedIndex = index
	s.overview.SelectedPath = s.overview.Worktrees[index].Path
	s.rememberWorktreeSelection()
	s.repositoryChanged()
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

// worktreeTabs builds the tab strip above the repository toolbar. A repository
// with a single main worktree shows no tabs.
func (s *settings) worktreeTabs(c *ui.Context) {
	if len(s.overview.Worktrees) <= 1 {
		return
	}
	labels := make([]string, len(s.overview.Worktrees))
	for i, item := range s.overview.Worktrees {
		labels[i] = worktreeLabel(item)
	}
	selected := s.overview.selectedIndex
	parts := components.Tabs(c, &selected, labels...)
	parts.List.Padding(8, 6, 6, 6)
	if parts.List.Changed() {
		s.selectWorktreeTab(selected)
	}
}

func worktreeLabel(item worktree) string {
	label := filepath.Base(item.Path)
	if label == "" || label == "." || label == string(filepath.Separator) {
		label = item.Path
	}
	return label
}
