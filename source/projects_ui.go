package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

func (s *settings) scanProjects() {
	s.startProjectScan(mygo.RunOnMain)
}

func (s *settings) startProjectScan(onMain func(func())) {
	if s.scanCancel != nil {
		s.scanCancel()
	}
	s.scanVersion++
	version := s.scanVersion
	s.scanProblem = ""
	root := s.values.ProjectParentDir
	if root != s.projectRoot {
		s.cancelWorktrees()
		s.projectRoot = root
		s.projectTree, s.selectedProject = s.state.Roots[root], nil
		s.restoreProjectSelection(onMain)
	}
	var paths []string
	walkProjects(s.projectTree, func(n *projectNode) { paths = append(paths, n.Path) })
	s.scanning = root != ""
	if root == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.scanCancel = cancel
	go func() {
		defer cancel()
		tree, err := scanProjects(ctx, root)
		// A missing parent directory, such as an unmounted volume, keeps the
		// cache; only paths under a present parent directory are dropped.
		missing := map[string]bool{}
		if _, statErr := os.Stat(root); errors.Is(statErr, os.ErrNotExist) {
			paths = nil
		}
		for _, path := range paths {
			if ctx.Err() != nil {
				return
			}
			_, statErr := os.Stat(path)
			if statErr == nil || errors.Is(statErr, os.ErrNotExist) {
				missing[path] = errors.Is(statErr, os.ErrNotExist)
			}
		}
		onMain(func() {
			if version != s.scanVersion {
				return
			}
			s.scanning = false
			s.projectTree = mergeProjects(s.projectTree, tree, missing)
			s.restoreProjectSelection(onMain)
			s.saveSidebarState()
			if err != nil {
				s.scanProblem = err.Error()
			}
			if s.window != nil {
				s.window.Invalidate()
			}
		})
	}()
}

func (s *settings) worktreeSidebar(c *ui.Context) {
	if s.toast != nil {
		c.AddToast(*s.toast)
		s.toast = nil
	}
	defer func() {
		changed := false
		walkProjects(s.projectTree, func(n *projectNode) { changed = changed || n.Open != n.savedOpen })
		if changed {
			s.saveSidebarState()
		}
	}()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		s.commandTrigger(c)
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().ScrollbarInsets(0).Children(func() {
			components.Sidebar(c).Children(func() {
				if s.values.ProjectParentDir != "" {
					if s.stateProblem != "" {
						components.Text(c, s.stateProblem).Wrap().TextColor(c.Theme().TextMuted)
					}
					if s.scanProblem != "" {
						components.Text(c, s.text("扫描未完成：")+s.scanProblem).Wrap().TextColor(c.Theme().TextMuted)
					}

					pinned, visible := sidebarProjects(s.projectTree)
					components.Tree(c, func() {
						s.projectGroup(c, "pinned", "置顶", &s.state.PinnedOpen, func() {
							for _, node := range pinned {
								s.projectRow(c, node, nil)
							}
						})
						s.projectGroup(c, "other", "其他项目", &s.state.OtherOpen, func() {
							if s.projectTree != nil {
								if s.projectTree.Repository {
									if visible[s.projectTree] {
										s.projectRow(c, s.projectTree, visible)
									}
								} else {
									for _, child := range s.projectTree.Children {
										if visible[child] {
											s.projectRow(c, child, visible)
										}
									}
								}
							}
						})
					}).Gap(c.Theme().Space(3))
				}
			})
		})
	})
}

// Group collapse is a sidebar preference, independent of project expansion.
func (s *settings) projectGroup(c *ui.Context, key, label string, expanded **bool, children func()) {
	if *expanded == nil {
		open := true
		*expanded = &open
	}
	ui.Column(c).Key(key).FillWidth().Shrink(0).Children(func() {
		parts := components.Collapsible(c, s.text(label), *expanded, components.CollapsibleOptions{}, func() {
			ui.Column(c).FillWidth().Shrink(0).Children(children)
		})
		parts.Root.FillWidth()
		parts.Trigger.FillWidth().TextColor(c.Theme().TextMuted)
		if parts.Changed() {
			s.saveSidebarState()
		}
	})
}

func (s *settings) projectRow(c *ui.Context, node *projectNode, visible map[*projectNode]bool) {
	label := filepath.Base(node.Path)
	if node.Missing {
		label += s.text("（已不存在）")
	}
	var children func()
	if !node.Repository && len(node.Children) > 0 {
		children = func() {
			for _, child := range node.Children {
				if visible[child] {
					s.projectRow(c, child, visible)
				}
			}
		}
	}
	ui.Column(c).Key(node.Path).FillWidth().Shrink(0).Children(func() {
		row := components.TreeItem(c, label, &node.Open, s.selectedProject == node, children)
		if row.Clicked() {
			s.selectProject(node, mygo.RunOnMain)
		}
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(s.text("打开文件夹")).Chosen() {
				s.openFolder(node.Path)
			}
			if !node.Repository {
				return
			}
			m.Separator()
			action := "置顶项目"
			if node.Pinned {
				action = "取消置顶"
			}
			if m.Item(s.text(action)).Chosen() {
				node.Pinned = !node.Pinned
				s.saveSidebarState()
			}
			m.Separator()
			if m.Item(s.text("移除项目")).Chosen() {
				s.removeProject(node, s.repositoryDispatch())
			}
		})
	})
}

// openFolder shows a path in the file manager: Finder, Explorer or the
// desktop's file manager. Tests inject the platform call.
func (s *settings) openFolder(path string) {
	open := s.openPath
	if open == nil {
		open = mygo.Shell.OpenPath
	}
	if err := open(path); err != nil {
		s.toast = &ui.Toast{ID: "open-folder", Title: s.text("无法打开目录"), Description: err.Error(), Type: components.ToastError}
	}
	s.invalidate()
}

// removeProject hides a project from the sidebar without touching its files.
// The hidden flag lives in the cached tree, so scans and restarts keep it.
func (s *settings) removeProject(node *projectNode, onMain func(func())) {
	if node == nil || !node.Repository || node.Removed {
		return
	}
	node.Removed = true
	selected := s.selectedProject == node
	if selected {
		s.cancelWorktrees()
		s.selectedProject = nil
		if s.state.SelectedProjects != nil {
			delete(s.state.SelectedProjects, s.projectRoot)
		}
	}
	s.saveSidebarState()
	s.toast = &ui.Toast{
		ID: "project-removed:" + node.Path, Title: s.text("已移除项目"), Description: projectName(node.Path),
		Action: s.text("撤销"), Timeout: 8 * time.Second,
		OnAction: func() {
			node.Removed = false
			s.saveSidebarState()
			if s.selectedProject == nil {
				s.restoreProjectSelection(s.repositoryDispatch())
			}
			s.invalidate()
		},
	}
	if selected {
		s.restoreProjectSelection(onMain)
	}
	s.invalidate()
}

// restoreRemovedProjects brings back every project removed from the current
// parent directory.
func (s *settings) restoreRemovedProjects() {
	restored := false
	walkProjects(s.projectTree, func(node *projectNode) {
		if node.Removed {
			node.Removed = false
			restored = true
		}
	})
	if !restored {
		return
	}
	s.saveSidebarState()
	if s.selectedProject == nil {
		s.restoreProjectSelection(s.repositoryDispatch())
	}
	s.invalidate()
}

func (s *settings) hasRemovedProjects() bool {
	found := false
	walkProjects(s.projectTree, func(node *projectNode) { found = found || node.Removed })
	return found
}

// Derive display groups without moving cached nodes or changing expansion state.
func sidebarProjects(root *projectNode) (pinned []*projectNode, visible map[*projectNode]bool) {
	visible = make(map[*projectNode]bool)
	var visit func(*projectNode) bool
	visit = func(node *projectNode) bool {
		if node == nil {
			return false
		}
		if node.Repository {
			if node.Removed {
				return false
			}
			if node.Pinned {
				pinned = append(pinned, node)
			} else {
				visible[node] = true
			}
			return visible[node]
		}
		for _, child := range node.Children {
			if visit(child) {
				visible[node] = true
			}
		}
		return visible[node]
	}
	visit(root)
	sort.Slice(pinned, func(i, j int) bool {
		left := strings.ToLower(filepath.Base(pinned[i].Path))
		right := strings.ToLower(filepath.Base(pinned[j].Path))
		if left == right {
			return pinned[i].Path < pinned[j].Path
		}
		return left < right
	})
	return pinned, visible
}
