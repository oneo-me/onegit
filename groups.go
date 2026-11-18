package main

import (
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// ungroupedID is the reserved ID of Ungrouped Projects in a parent directory's
// group order. It has no name and cannot be renamed or deleted.
const ungroupedID = "ungrouped"

// projectGroup is a user-made sidebar group. Projects name their group by ID,
// so renaming a group keeps its projects.
type projectGroup struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name,omitempty"`
	Open *bool  `yaml:"open,omitempty"`
}

// groupDialog names a new group or renames one. A new group made from a
// project's menu takes that project.
type groupDialog struct {
	open    bool
	groupID string
	project *projectNode
	name    string
}

// projectGroups returns the current parent directory's groups in sidebar
// order, always including Ungrouped Projects.
func (s *settings) projectGroups() []*projectGroup {
	groups := s.state.Groups[s.projectRoot]
	for _, group := range groups {
		if group.ID == ungroupedID {
			return groups
		}
	}
	groups = append(groups, &projectGroup{ID: ungroupedID})
	if s.projectRoot != "" {
		if s.state.Groups == nil {
			s.state.Groups = make(map[string][]*projectGroup)
		}
		s.state.Groups[s.projectRoot] = groups
	}
	return groups
}

func (s *settings) findGroup(id string) *projectGroup {
	for _, group := range s.projectGroups() {
		if group.ID == id {
			return group
		}
	}
	return nil
}

// groupIDs lists the user-made groups that exist, so projects naming any
// other group show as ungrouped.
func (s *settings) groupIDs() map[string]bool {
	ids := map[string]bool{}
	for _, group := range s.projectGroups() {
		if group.ID != ungroupedID {
			ids[group.ID] = true
		}
	}
	return ids
}

// createGroup adds a group just before Ungrouped Projects and returns it.
func (s *settings) createGroup(name string) *projectGroup {
	groups := s.projectGroups()
	next := 1
	for _, group := range groups {
		if n, err := strconv.Atoi(strings.TrimPrefix(group.ID, "g")); err == nil && strings.HasPrefix(group.ID, "g") && n >= next {
			next = n + 1
		}
	}
	group := &projectGroup{ID: "g" + strconv.Itoa(next), Name: name}
	at := len(groups)
	for i, existing := range groups {
		if existing.ID == ungroupedID {
			at = i
		}
	}
	groups = append(groups[:at], append([]*projectGroup{group}, groups[at:]...)...)
	s.state.Groups[s.projectRoot] = groups
	return group
}

// deleteGroup removes a user-made group; its projects return to Ungrouped
// Projects.
func (s *settings) deleteGroup(id string) {
	if id == ungroupedID {
		return
	}
	groups := s.projectGroups()
	kept := groups[:0]
	for _, group := range groups {
		if group.ID != id {
			kept = append(kept, group)
		}
	}
	s.state.Groups[s.projectRoot] = kept
	walkProjects(s.projectTree, func(node *projectNode) {
		if node.Group == id {
			node.Group = ""
		}
	})
	s.saveSidebarState()
	s.invalidate()
}

// moveGroup places the group from before the group to, where the pointer
// dropped it.
func (s *settings) moveGroup(from, to string) {
	if from == to {
		return
	}
	groups := s.projectGroups()
	var moved *projectGroup
	kept := make([]*projectGroup, 0, len(groups))
	for _, group := range groups {
		if group.ID == from {
			moved = group
		} else {
			kept = append(kept, group)
		}
	}
	if moved == nil {
		return
	}
	at := len(kept)
	for i, group := range kept {
		if group.ID == to {
			at = i
		}
	}
	// Dragging down puts the group after the target, so every drop moves it.
	for i, group := range groups {
		if group.ID == from {
			if i <= at {
				at++
			}
			break
		}
	}
	at = min(at, len(kept))
	s.state.Groups[s.projectRoot] = append(kept[:at], append([]*projectGroup{moved}, kept[at:]...)...)
	s.saveSidebarState()
	s.invalidate()
}

func (s *settings) moveProjectToGroup(node *projectNode, id string) {
	if id == ungroupedID {
		id = ""
	}
	node.Group = id
	s.saveSidebarState()
	s.invalidate()
}

func (s *settings) openGroupDialog(groupID string, project *projectNode) {
	s.groupDialog = groupDialog{open: true, groupID: groupID, project: project}
	if group := s.findGroup(groupID); group != nil {
		s.groupDialog.name = group.Name
	}
	s.invalidate()
}

func (s *settings) submitGroupDialog() {
	d := &s.groupDialog
	name := strings.TrimSpace(d.name)
	if name == "" {
		return
	}
	d.open = false
	if group := s.findGroup(d.groupID); group != nil {
		group.Name = name
	} else {
		group = s.createGroup(name)
		if d.project != nil {
			d.project.Group = group.ID
		}
	}
	s.saveSidebarState()
	s.invalidate()
}

// groupMenu is the menu of every group heading.
func (s *settings) groupMenu(id string) []core.MenuItem {
	items := []core.MenuItem{{Label: s.text("新建分组…"), OnSelect: func() { s.openGroupDialog("", nil) }}}
	if id == "" || id == ungroupedID {
		return items
	}
	return append(items, core.MenuItem{Separator: true},
		core.MenuItem{Label: s.text("重命名分组…"), OnSelect: func() { s.openGroupDialog(id, nil) }},
		core.MenuItem{Label: s.text("删除分组"), OnSelect: func() { s.deleteGroup(id) }})
}

// projectGroupMenu moves a repository between groups.
func (s *settings) projectGroupMenu(node *projectNode) core.MenuItem {
	ids := s.groupIDs()
	var items []core.MenuItem
	for _, group := range s.projectGroups() {
		label := group.Name
		if group.ID == ungroupedID {
			label = s.text("未分组项目")
		}
		current := node.Group == group.ID || group.ID == ungroupedID && !ids[node.Group]
		items = append(items, core.MenuItem{Label: label, Checkable: true, Checked: current, OnSelect: func() { s.moveProjectToGroup(node, group.ID) }})
	}
	items = append(items, core.MenuItem{Separator: true}, core.MenuItem{Label: s.text("新建分组…"), OnSelect: func() { s.openGroupDialog("", node) }})
	return core.MenuItem{Label: s.text("移到分组"), Items: items}
}

// groupDialogView names a new group or renames one.
func (s *settings) groupDialogView(cx *core.Scope) core.Node {
	d := &s.groupDialog
	ready := func() bool { s.track(); return strings.TrimSpace(d.name) != "" }
	shown := core.NewMemo(cx, func() bool { s.track(); return d.open })
	field := core.Dynamic(func(cx *core.Scope) core.Node {
		if !shown.Get() {
			return nil
		}
		return ui.Input{Value: func() string { s.track(); return d.name }, Label: s.text("分组名称"), Placeholder: s.text("分组名称"), AutoFocus: true, TestID: "group-name",
			OnChange: func(name string) { d.name = name; s.invalidate() }, OnSubmit: func(name string) { d.name = name; s.submitGroupDialog() }}
	})
	return ui.Dialog{TestID: "group-dialog", Style: core.Style{Width: core.Pt(360), MaxWidth: core.Pct(100)},
		Open:         func() bool { s.track(); return d.open },
		OnOpenChange: func(open bool, _ reason) { d.open = open; s.invalidate() },
		TitleFunc: func() string {
			s.track()
			if d.groupID != "" {
				return s.text("重命名分组")
			}
			return s.text("新建分组")
		},
		Children: []core.Node{field},
		Footer: []core.Node{
			ui.Button{Text: s.text("取消"), Variant: ui.ButtonSecondary, TestID: "cancel", OnPress: func() { d.open = false; s.invalidate() }},
			core.Dynamic(func(cx *core.Scope) core.Node {
				s.track()
				action := s.text("创建")
				if d.groupID != "" {
					action = s.text("重命名")
				}
				return ui.Button{Text: action, Variant: ui.ButtonPrimary, TestID: "confirm", Disabled: func() bool { return !ready() }, OnPress: s.submitGroupDialog}
			}),
		}}
}
