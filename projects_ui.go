package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// scanProjects reads the parent directory's projects in the background,
// merging them into the cached tree once found.
func (s *settings) scanProjects() {
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
		s.restoreProjectSelection()
	}
	var paths []string
	walkProjects(s.projectTree, func(n *projectNode) { paths = append(paths, n.Path) })
	s.scanning = root != ""
	s.invalidate()
	if root == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.scanCancel = cancel
	post := s.poster()
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
		post(func() {
			if version != s.scanVersion {
				return
			}
			s.scanning = false
			s.projectTree = mergeProjects(s.projectTree, tree, missing)
			s.restoreProjectSelection()
			s.saveSidebarState()
			if err != nil {
				s.scanProblem = err.Error()
			}
			s.invalidate()
		})
	}()
}

// showToast shows a toast, and keeps it as the last one for tests.
func (s *settings) showToast(t ui.ToastContent) {
	s.lastToast = t
	if s.toasts != nil {
		s.toasts.Show(t)
	}
}

// sidebarKey says what the project sidebar's rows show, so it is built
// again only when they change; the selected row follows on its own.
func (s *settings) sidebarKey() string {
	s.track()
	var b strings.Builder
	fmt.Fprint(&b, s.language(), s.values.ProjectParentDir, "\x00", s.stateProblem, "\x00", s.scanProblem, "\x00", s.projectRoot)
	walkProjects(s.projectTree, func(n *projectNode) {
		fmt.Fprint(&b, n.Path, n.Repository, n.Open, n.Missing, n.Pinned, n.Removed, n.Group, n.Icon, s.projectIcons[n.Icon] != nil, ";")
	})
	for _, g := range s.projectGroups() {
		fmt.Fprint(&b, g.ID, g.Name, ";")
	}
	return b.String()
}

// worktreeSidebar is the sidebar: the command palette's trigger above the
// projects in their groups.
func (s *settings) worktreeSidebar(cx *core.Scope) core.Node {
	rows := region(cx, s.sidebarKey, func(cx *core.Scope) core.Node {
		if s.values.ProjectParentDir == "" {
			return nil
		}
		return core.View{Style: core.Style{Gap: 4, Padding: core.Edges{Top: 6, Right: 8, Bottom: 6, Left: 6}}, Children: s.sidebarRows(cx)}
	})
	return core.View{Style: ui.FillRest, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.Symmetric(0, 8), FlexShrink: 0}, Children: []core.Node{s.commandTrigger(cx)}},
		ui.ScrollView{Style: ui.FillRest, Children: []core.Node{rows}},
	}}
}

// sidebarRows are the problems reading the projects, then the projects in
// their groups: pinned, the user's groups and the ungrouped ones.
func (s *settings) sidebarRows(cx *core.Scope) []core.Node {
	colors := ui.ColorsOf(cx)
	var nodes []core.Node
	for _, problem := range []string{s.stateProblem, s.scanProblemText()} {
		if problem != "" {
			nodes = append(nodes, core.View{Style: core.Style{Padding: core.Symmetric(4, 10)}, Children: []core.Node{
				core.Text{Content: problem, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)}}})
		}
	}
	pinned, grouped, visible := sidebarProjects(s.projectTree, s.groupIDs())
	var pinnedRows []core.Node
	for _, node := range pinned {
		pinnedRows = append(pinnedRows, s.projectRow(cx, node, 0, nil)...)
	}
	nodes = append(nodes, s.projectGroup(cx, "", s.text("置顶"), &s.state.PinnedOpen, len(pinned), pinnedRows))
	for _, group := range s.projectGroups() {
		if group.ID != ungroupedID {
			var rows []core.Node
			for _, node := range grouped[group.ID] {
				rows = append(rows, s.projectRow(cx, node, 0, nil)...)
			}
			nodes = append(nodes, s.projectGroup(cx, group.ID, group.Name, &group.Open, len(grouped[group.ID]), rows))
			continue
		}
		ungrouped := 0
		for node := range visible {
			if node.Repository {
				ungrouped++
			}
		}
		var rows []core.Node
		switch {
		case s.projectTree == nil:
		case s.projectTree.Repository:
			if visible[s.projectTree] {
				rows = s.projectRow(cx, s.projectTree, 0, visible)
			}
		default:
			for _, child := range s.projectTree.Children {
				if visible[child] {
					rows = append(rows, s.projectRow(cx, child, 0, visible)...)
				}
			}
		}
		nodes = append(nodes, s.projectGroup(cx, ungroupedID, s.text("未分组项目"), &s.state.OtherOpen, ungrouped, rows))
	}
	return nodes
}

func (s *settings) scanProblemText() string {
	if s.scanProblem == "" {
		return ""
	}
	return s.text("扫描未完成：") + s.scanProblem
}

// projectGroup lists rows under a group's heading: the label with the
// arrow after it, turning as the rows spring open, and the number of
// projects at the row's end unless there are none. Group collapse is a sidebar preference, independent of project
// expansion. Every group but Pinned, whose id is empty, reorders by
// dragging its heading onto another group.
func (s *settings) projectGroup(cx *core.Scope, id, label string, expanded **bool, count int, rows []core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	open := func() bool { s.track(); return *expanded == nil || **expanded }
	key := "group:" + id
	if id == "" {
		key = "pinned"
	}
	title := ui.Text(label, sidebarHeading, ui.Bold, colors.Muted)
	title.MaxLines, title.Style.FlexShrink, title.Style.MinWidth = 1, 1, core.Pt(0)
	heading := core.View{Role: core.RoleButton, Label: label, TestID: key, NoFocus: true, Expanded: open,
		Style: core.Style{Direction: core.Row, AlignSelf: core.AlignStart, AlignItems: core.AlignCenter, Gap: 4, FlexShrink: 1, MinWidth: core.Pt(0),
			Padding: core.Edges{Top: 4, Right: 8, Bottom: 4, Left: 10}, CornerRadius: 8, Cursor: core.CursorDefault},
		OnPress: func(core.PressEvent) {
			next := !open()
			*expanded = &next
			s.saveSidebarState()
			s.invalidate()
		},
		OnContextMenu: func(e core.ContextMenuEvent) { s.popup(cx, e.PagePosition, s.groupMenu(id)) },
		Children:      []core.Node{title, disclosureArrow(cx, open)}}
	header := []core.Node{heading}
	if count > 0 {
		// The count's padding centers it on the label.
		number := ui.Text(strconv.Itoa(count), sidebarHeading, ui.Regular, colors.Muted)
		number.Style.Padding, number.Style.FlexShrink = core.Edges{Top: 4, Right: 8, Bottom: 4}, 0
		header = append(header, ui.Spacer{}, number)
	}
	group := core.View{TestID: key + "-group", Style: core.Style{CornerRadius: 8, FlexShrink: 0},
		OnLayout: func(b core.Bounds) {
			if s.groupFrames == nil {
				s.groupFrames = map[string]core.Bounds{}
			}
			s.groupFrames[id] = b
		}}
	if id != "" {
		var start core.Bounds
		heading.OnPan = func(e core.PanEvent) {
			switch e.Phase {
			case core.GestureBegan:
				start = s.groupFrames[id]
			case core.GestureChanged:
				target := s.groupAt(start.Origin.Y + start.Size.Height/2 + e.Translation.Y)
				if target == id {
					target = ""
				}
				if target != s.dragTarget {
					s.dragTarget = target
					s.invalidate()
				}
			case core.GestureEnded:
				if s.dragTarget != "" {
					s.moveGroup(id, s.dragTarget)
				}
				s.dragTarget = ""
				s.invalidate()
			case core.GestureCancelled:
				s.dragTarget = ""
				s.invalidate()
			}
		}
		group.StyleFunc = func() core.Style {
			s.track()
			if s.dragTarget == id {
				return core.Style{Background: ui.Alpha(colors.Accent(), 0.12)}
			}
			return core.Style{}
		}
	}
	header[0] = heading
	group.Children = []core.Node{base.Collapsible{Open: open, Reveal: func() bool { return true },
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: header}},
		Content:  core.View{Style: core.Style{Gap: 1, Padding: core.Edges{Top: 4, Bottom: 4, Left: 2}}, Children: rows}}}
	return group
}

// sidebarHeading is the size of the sidebar's headings: 0.8 rem of
// macOS's 13 point text.
const sidebarHeading = 10.4

// disclosureArrow is a disclosure's arrow, pointing along the line and
// turning down as open says it opens.
func disclosureArrow(cx *core.Scope, open func() bool) core.Node {
	style := core.Style{FlexShrink: 0, Width: core.Pt(16), Height: core.Pt(16), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
	if !ui.ReduceMotion(cx) {
		style.Transition = core.Transition{Duration: 200 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionTransform}
	}
	return core.View{Style: style, StyleFunc: func() core.Style {
		var degrees float32
		if open() {
			degrees = 90
		}
		return core.Style{Transform: core.Transform{Rotate: degrees}}
	}, Children: []core.Node{ui.Icon(lucide.ChevronRight, 12, ui.ColorsOf(cx).Muted)}}
}

// groupAt is the group, other than Pinned, whose frame holds y, the
// nearest one past the first or last.
func (s *settings) groupAt(y core.Points) string {
	best, distance := "", core.Points(-1)
	for id, b := range s.groupFrames {
		if id == "" {
			continue
		}
		d := core.Points(0)
		switch {
		case y < b.Origin.Y:
			d = b.Origin.Y - y
		case y > b.Origin.Y+b.Size.Height:
			d = y - b.Origin.Y - b.Size.Height
		}
		if distance < 0 || d < distance {
			best, distance = id, d
		}
	}
	return best
}

// projectRow shows a repository led by its icon, or a folder led by a folder
// icon; a folder's children follow, indented, while it is open. Clicking a
// folder opens and closes it.
func (s *settings) projectRow(cx *core.Scope, node *projectNode, depth int, visible map[*projectNode]bool) []core.Node {
	colors := ui.ColorsOf(cx)
	label := filepath.Base(node.Path)
	if node.Missing {
		label += s.text("（已不存在）")
	}
	folder := !node.Repository && len(node.Children) > 0
	// An 18 point mark leads the label.
	const size = 18
	var lead core.Node
	switch {
	case node.Repository:
		img := s.projectImage(node)
		svg, _ := core.ParseSVG(img)
		switch {
		case img != nil && strings.EqualFold(filepath.Ext(node.Icon), ".svg") && svg != nil:
			lead = core.Icon{SVG: svg, Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), FlexShrink: 0}}
		case img != nil:
			lead = core.Image{Source: core.ImageSource{Data: img}, ResizeMode: core.Contain, ElementsHidden: true,
				Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size * .2, FlexShrink: 0}}
		default:
			// A blank tile holds the place of a project without an icon.
			lead = core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size * .2, FlexShrink: 0},
				StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Muted(), .18)} }}
		}
	case folder && node.Open:
		lead = ui.Icon(lucide.FolderOpen, size, colors.Muted)
	default:
		lead = ui.Icon(lucide.Folder, size, colors.Muted)
	}
	text := ui.Text(label, 13, ui.Regular, colors.Text)
	text.MaxLines, text.Style.FlexGrow, text.Style.FlexShrink, text.Style.MinWidth = 1, 1, 1, core.Pt(0)
	row := core.View{Role: core.RoleButton, Label: label, TestID: "project:" + node.Path, NoFocus: true,
		Selected: func() bool { s.track(); return s.selectedProject == node },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexShrink: 0,
			Padding: core.Edges{Top: 7, Bottom: 7, Left: 8 + 16*core.Points(depth), Right: 8}, CornerRadius: 8, Cursor: core.CursorDefault},
		StyleFunc: func() core.Style { return core.Style{Selected: &core.Style{Background: ui.Alpha(colors.Text(), 0.1)}} },
		OnPress: func(core.PressEvent) {
			if folder {
				node.Open = !node.Open
				s.saveSidebarState()
				s.invalidate()
				return
			}
			s.selectProject(node)
		},
		OnContextMenu: func(e core.ContextMenuEvent) { s.popup(cx, e.PagePosition, s.projectMenu(node)) },
		Children:      []core.Node{lead, text}}
	if folder {
		row.Expanded = func() bool { return node.Open }
	}
	rows := []core.Node{row}
	if folder && node.Open {
		for _, child := range node.Children {
			if visible[child] {
				rows = append(rows, s.projectRow(cx, child, depth+1, visible)...)
			}
		}
	}
	return rows
}

// projectMenu is a project's menu: opening its folder, pinning it, moving it
// between groups, its icon and removing it from the sidebar.
func (s *settings) projectMenu(node *projectNode) []core.MenuItem {
	items := []core.MenuItem{{Label: s.text("打开文件夹"), OnSelect: func() { s.openFolder(node.Path) }}}
	if !node.Repository {
		return items
	}
	action := "置顶项目"
	if node.Pinned {
		action = "取消置顶"
	}
	items = append(items, core.MenuItem{Separator: true}, core.MenuItem{Label: s.text(action), OnSelect: func() {
		node.Pinned = !node.Pinned
		s.saveSidebarState()
		s.invalidate()
	}}, s.projectGroupMenu(node), core.MenuItem{Separator: true},
		core.MenuItem{Label: s.text("选择图标…"), OnSelect: func() { s.chooseProjectIcon(node) }})
	if node.Icon != "" {
		items = append(items, core.MenuItem{Label: s.text("恢复默认图标"), OnSelect: func() { s.resetProjectIcon(node) }})
	}
	return append(items, core.MenuItem{Separator: true}, core.MenuItem{Label: s.text("移除项目"), OnSelect: func() { s.removeProject(node) }})
}

// popup shows a menu the platform draws where a secondary click asked for
// it.
func (s *settings) popup(cx *core.Scope, at core.Point, items []core.MenuItem) {
	if len(items) > 0 {
		desktop.PopupMenu(cx, items, at, nil)
	}
}

// openFolder shows a path in the file manager: Finder, Explorer or the
// desktop's file manager. Tests inject the platform call.
func (s *settings) openFolder(path string) {
	open := s.openPath
	if open == nil {
		open = func(path string, done func(error)) {
			s.cx.OpenFile(core.SharedFile{Path: path}, func(_ bool, err error) { done(err) })
		}
	}
	open(path, func(err error) {
		if err != nil {
			s.showToast(ui.ToastContent{ID: "open-folder", Type: ui.ToastError, Title: s.text("无法打开目录"), Description: err.Error()})
		}
	})
}

// removeProject hides a project from the sidebar without touching its files.
// The hidden flag lives in the cached tree, so scans and restarts keep it.
func (s *settings) removeProject(node *projectNode) {
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
	s.showToast(ui.ToastContent{
		ID: "project-removed:" + node.Path, Title: s.text("已移除项目"), Description: projectName(node.Path),
		Action: s.text("撤销"), Duration: 8 * time.Second,
		OnAction: func() {
			node.Removed = false
			s.saveSidebarState()
			if s.selectedProject == nil {
				s.restoreProjectSelection()
			}
			s.invalidate()
		},
	})
	if selected {
		s.restoreProjectSelection()
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
		s.restoreProjectSelection()
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
