package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

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
		s.projectRoot = root
		s.projectTree, s.selectedProject = s.state.Roots[root], nil
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
		missing := map[string]bool{}
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
	defer func() {
		changed := false
		walkProjects(s.projectTree, func(n *projectNode) { changed = changed || n.Open != n.savedOpen })
		if changed {
			s.saveSidebarState()
		}
	}()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().ScrollbarInsets(0).Children(func() {
			ui.Column(c).FillWidth().Shrink(0).Padding(16, 8, 16, 8).Gap(12).Children(func() {
				switch {
				case s.values.ProjectParentDir == "":
					components.Text(c, s.text("请在设置中选择项目父目录。")).Wrap().TextColor(c.Theme().TextMuted)
					if components.Button(c, s.text("选择目录…"), components.ButtonSecondary).Clicked() {
						s.chooseDirectory()
					}
				default:
					if s.scanning {
						components.Text(c, s.text("正在扫描 Git 项目…")).Wrap()
					}
					if s.stateProblem != "" {
						components.Text(c, s.stateProblem).Wrap().TextColor(c.Theme().TextMuted)
					}
					if s.scanProblem != "" {
						components.Text(c, s.text("扫描未完成：")+s.scanProblem).Wrap().TextColor(c.Theme().TextMuted)
					}
					if s.projectTree == nil && s.scanProblem == "" && !s.scanning {
						components.Text(c, s.text("未找到 Git 项目。")).Wrap().TextColor(c.Theme().TextMuted)
					}
					components.Tree(c, func() {
						if s.projectTree != nil {
							for _, child := range s.projectTree.Children {
								s.projectRow(c, child)
							}
						}
					})
				}
			})
		})
	})
}

func (s *settings) projectRow(c *ui.Context, node *projectNode) {
	label := filepath.Base(node.Path)
	if node.Missing {
		label += s.text("（已不存在）")
	}
	var children func()
	if len(node.Children) > 0 {
		children = func() {
			for _, child := range node.Children {
				s.projectRow(c, child)
			}
		}
	}
	ui.Column(c).Key(node.Path).FillWidth().Shrink(0).Children(func() {
		if components.TreeItem(c, label, &node.Open, s.selectedProject == node, children).Clicked() {
			s.selectedProject = node
		}
	})
}
