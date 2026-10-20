package main

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
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
	s.groupDialogView(c)
	t := c.Theme()
	// The layout follows Neorio's Git window sidebar. Collapsible indents its
	// panel by half a space, which the scroll content gives up and the group
	// headings add back.
	ui.Column(c).Fill().MinHeight(0).Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(0, t.Space(2)).Children(func() { s.commandTrigger(c) })
		if s.values.ProjectParentDir == "" {
			return
		}
		components.ScrollView(c, components.ScrollViewOptions{}).FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Shrink(0).Gap(t.Space(1)).Padding(t.Space(1.5), t.Space(2), t.Space(1.5), t.Space(1.5)).Children(func() {
				for _, problem := range []string{s.stateProblem, s.scanProblemText()} {
					if problem != "" {
						components.Text(c, problem).FontSize(12).Wrap().FillWidth().TextColor(t.TextMuted).Padding(t.Space(1), t.Space(2)+t.Space(0.5))
					}
				}
				pinned, grouped, visible := sidebarProjects(s.projectTree, s.groupIDs())
				s.projectGroup(c, "", s.text("置顶"), &s.state.PinnedOpen, len(pinned), func() {
					for _, node := range pinned {
						s.projectRow(c, node, 0, nil)
					}
				})
				for _, group := range s.projectGroups() {
					if group.ID != ungroupedID {
						s.projectGroup(c, group.ID, group.Name, &group.Open, len(grouped[group.ID]), func() {
							for _, node := range grouped[group.ID] {
								s.projectRow(c, node, 0, nil)
							}
						})
						continue
					}
					ungrouped := 0
					for node := range visible {
						if node.Repository {
							ungrouped++
						}
					}
					s.projectGroup(c, ungroupedID, s.text("未分组项目"), &s.state.OtherOpen, ungrouped, func() {
						if s.projectTree == nil {
							return
						}
						if s.projectTree.Repository {
							if visible[s.projectTree] {
								s.projectRow(c, s.projectTree, 0, visible)
							}
							return
						}
						for _, child := range s.projectTree.Children {
							if visible[child] {
								s.projectRow(c, child, 0, visible)
							}
						}
					})
				}
			})
		})
	})
}

func (s *settings) scanProblemText() string {
	if s.scanProblem == "" {
		return ""
	}
	return s.text("扫描未完成：") + s.scanProblem
}

// Group collapse is a sidebar preference, independent of project expansion.
// Every group but Pinned, whose id is empty, reorders by dragging its header
// onto another group. Headings follow Neorio's Git window sidebar, ending in
// their number of projects unless they have none.
func (s *settings) projectGroup(c *ui.Context, id, label string, expanded **bool, count int, children func()) {
	if *expanded == nil {
		open := true
		*expanded = &open
	}
	key := "group:" + id
	if id == "" {
		key = "pinned"
	}
	t := c.Theme()
	group := ui.Column(c).Key(key).FillWidth().Shrink(0).Radius(t.Radius)
	if id != "" {
		if dragged, ok := ui.DragOver[groupDrag](group); ok && dragged.id != id {
			group.Background(t.Accent.Alpha(.12))
		}
		if dropped, ok := ui.Drop[groupDrag](group); ok {
			s.moveGroup(dropped.id, id)
		}
	}
	group.Children(func() {
		var options components.CollapsibleOptions
		if count > 0 {
			// The trigger's vertical padding centers the count on the label.
			options.Trailing = func() {
				components.Text(c, strconv.Itoa(count)).FontSize(t.Rem(.8)).TextColor(t.TextMuted).
					Padding(t.Space(1), t.Space(2), t.Space(1), 0)
			}
		}
		parts := components.Collapsible(c, label, *expanded, options, func() {
			ui.Column(c).FillWidth().Shrink(0).Gap(1).Children(children)
		})
		parts.Root.FillWidth()
		// The trigger shares a row with the count, if any, and takes its room.
		if count > 0 {
			parts.Trigger.Grow(1)
		} else {
			parts.Trigger.FillWidth()
		}
		parts.Trigger.FontSize(t.Rem(.8)).Bold().TextColor(t.TextMuted).
			Padding(t.Space(1), t.Space(2), t.Space(1), t.Space(2)+t.Space(0.5))
		if id != "" {
			parts.Trigger.Drag(groupDrag{id})
		}
		parts.Trigger.ContextMenu(func(m *ui.Menu) { s.groupMenu(m, id) })
		if parts.Changed() {
			s.saveSidebarState()
		}
	})
}

// projectRow shows a repository led by its icon, or a folder led by a folder
// icon; a folder's children follow, indented, while it is open. Clicking a
// folder, Right and Left open and close it.
func (s *settings) projectRow(c *ui.Context, node *projectNode, depth int, visible map[*projectNode]bool) {
	label := filepath.Base(node.Path)
	if node.Missing {
		label += s.text("（已不存在）")
	}
	folder := !node.Repository && len(node.Children) > 0
	ui.Column(c).Key(node.Path).FillWidth().Shrink(0).Children(func() {
		t := c.Theme()
		row := components.SidebarItemWithLeading(c, label, s.selectedProject == node, func(c *ui.Context, size float32) {
			switch {
			case node.Repository:
				ui.Image(c, projectIcon()).Size(size, size).Fit(ui.Contain).Radius(size * .2).Shrink(0)
			case folder && node.Open:
				components.Icon(c, lucide.FolderOpen).Size(size, size).Shrink(0).TextColor(t.TextMuted)
			default:
				components.Icon(c, lucide.Folder).Size(size, size).Shrink(0).TextColor(t.TextMuted)
			}
		})
		if depth > 0 {
			row.Padding(t.Space(1.75), t.Space(2), t.Space(1.75), t.Space(2)+t.Space(4)*float32(depth))
		}
		if folder {
			row.Expanded(node.Open).
				OnShortcut(0, ui.KeyRight, func() { node.Open = true }).
				OnShortcut(0, ui.KeyLeft, func() { node.Open = false })
			if row.Clicked() {
				node.Open = !node.Open
			}
		} else if row.Clicked() {
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
			s.projectGroupMenu(m, node)
			m.Separator()
			if m.Item(s.text("移除项目")).Chosen() {
				s.removeProject(node, s.repositoryDispatch())
			}
		})
	})
	if folder && node.Open {
		for _, child := range node.Children {
			if visible[child] {
				s.projectRow(c, child, depth+1, visible)
			}
		}
	}
}

//go:embed assets/icon.png
var projectIconPNG []byte

// projectIcon is the app icon, a placeholder for projects' own icons. It is
// decoded once, on first use.
var projectIcon = sync.OnceValue(func() ui.ImageSource {
	if bitmap, err := ui.DecodeBitmap(projectIconPNG); err == nil {
		return bitmap
	}
	return nil
})

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

// Derive display groups without moving cached nodes or changing expansion
// state. Pinning wins over a group; projects of unknown groups are ungrouped.
func sidebarProjects(root *projectNode, groups map[string]bool) (pinned []*projectNode, grouped map[string][]*projectNode, visible map[*projectNode]bool) {
	grouped = make(map[string][]*projectNode)
	visible = make(map[*projectNode]bool)
	var visit func(*projectNode) bool
	visit = func(node *projectNode) bool {
		if node == nil {
			return false
		}
		if node.Repository {
			switch {
			case node.Removed:
			case node.Pinned:
				pinned = append(pinned, node)
			case groups[node.Group]:
				grouped[node.Group] = append(grouped[node.Group], node)
			default:
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
	sortProjects(pinned)
	for _, nodes := range grouped {
		sortProjects(nodes)
	}
	return pinned, grouped, visible
}

// sortProjects orders flat groups by name case-insensitively, then by path.
func sortProjects(nodes []*projectNode) {
	sort.Slice(nodes, func(i, j int) bool {
		left := strings.ToLower(filepath.Base(nodes[i].Path))
		right := strings.ToLower(filepath.Base(nodes[j].Path))
		if left == right {
			return nodes[i].Path < nodes[j].Path
		}
		return left < right
	})
}
