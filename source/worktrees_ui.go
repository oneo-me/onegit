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
	if s.repo.view != nil {
		s.repo.view.SetPath("")
	}
}

func (s *settings) selectProject(node *projectNode, onMain func(func())) {
	s.cancelWorktrees()
	s.repo.dispatch = onMain
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

// worktreeTabs builds the segmented worktree tabs in the title bar. They
// name the project in place of a title, so a repository with only its main
// worktree, or whose worktrees are still loading, shows one tab.
func (s *settings) worktreeTabs(c *ui.Context) {
	project := s.selectedProject
	if project == nil || !project.Repository {
		return
	}
	labels := []string{projectName(project.Path)}
	if len(s.overview.Worktrees) > 0 {
		labels = make([]string, len(s.overview.Worktrees))
		for i, item := range s.overview.Worktrees {
			labels[i] = worktreeLabel(item)
		}
	}
	selected := max(0, s.overview.selectedIndex)
	// Many worktrees scroll rather than push the actions out of the title bar.
	components.ScrollView(c, components.ScrollViewOptions{Axes: components.ScrollHorizontal}).
		Shrink(1).MinWidth(0).AlignItems(ui.Center).Children(func() {
		parts := components.Tabs(c, &selected, components.TabsSegmented, labels...)
		parts.List.AlignSelf(ui.Center).Shrink(0)
		if parts.List.Changed() {
			s.selectWorktreeTab(selected)
		}
	})
}

// worktreeLabel names a worktree after its directory; the main worktree's
// directory is the project's name.
func worktreeLabel(item worktree) string {
	return projectName(item.Path)
}

func projectName(path string) string {
	label := filepath.Base(path)
	if label == "" || label == "." || label == string(filepath.Separator) {
		label = path
	}
	return label
}
