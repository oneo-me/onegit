package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/egoist/mygo"
	"github.com/oneo-me/onegit/source/gitview"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

type sidebarState struct {
	Window            *mygo.WindowState          `yaml:"window,omitempty"`
	SidebarWidth      float32                    `yaml:"sidebarWidth,omitempty"`
	SidebarCollapsed  bool                       `yaml:"sidebarCollapsed,omitempty"`
	SelectedProjects  map[string]string          `yaml:"selectedProjects,omitempty"`
	SelectedWorktrees map[string]string          `yaml:"selectedWorktrees,omitempty"`
	GitView           *gitview.State             `yaml:"gitView,omitempty"`
	Roots             map[string]*projectNode    `yaml:"roots,omitempty"`
	PinnedOpen        *bool                      `yaml:"pinnedOpen,omitempty"`
	OtherOpen         *bool                      `yaml:"otherOpen,omitempty"`
	Groups            map[string][]*projectGroup `yaml:"groups,omitempty"`
}

const mainWindowStateKey = "main"

// windowStateStore keeps MyGo's state of the main window in state.yaml, so
// either save writes the whole file. MyGo calls it on the main thread.
type windowStateStore struct{ s *settings }

func (w windowStateStore) LoadWindowStates() (map[string]mygo.WindowState, error) {
	if w.s.state.Window == nil {
		return nil, nil
	}
	return map[string]mygo.WindowState{mainWindowStateKey: *w.s.state.Window}, nil
}

func (w windowStateStore) SaveWindowStates(states map[string]mygo.WindowState) error {
	state, ok := states[mainWindowStateKey]
	if !ok {
		return nil
	}
	w.s.state.Window = &state
	return w.s.saveSidebarState()
}

// restoreShell applies the saved sidebar layout to the main window's shell.
func (s *settings) restoreShell(shell *sidebar.Shell) {
	s.shell = shell
	shell.SetLeftExpanded(!s.state.SidebarCollapsed)
	if s.state.SidebarWidth > 0 {
		shell.SetLeftWidth(s.state.SidebarWidth)
	}
}

func (s *settings) restoreProjectSelection(onMain func(func())) {
	if s.selectedProject != nil {
		if !s.selectedProject.Missing && !s.selectedProject.Removed {
			return
		}
		s.cancelWorktrees()
		s.selectedProject = nil
	}
	path := s.state.SelectedProjects[s.projectRoot]
	var selected, first *projectNode
	// Without a saved selection, open the first project in sidebar order.
	pinned, grouped, visible := sidebarProjects(s.projectTree, s.groupIDs())
	ordered := pinned
	for _, group := range s.projectGroups() {
		if group.ID != ungroupedID {
			ordered = append(ordered, grouped[group.ID]...)
			continue
		}
		walkProjects(s.projectTree, func(node *projectNode) {
			if node.Repository && visible[node] {
				ordered = append(ordered, node)
			}
		})
	}
	for _, node := range ordered {
		if !node.Missing {
			first = node
			break
		}
	}
	walkProjects(s.projectTree, func(node *projectNode) {
		if !node.Repository || node.Missing || node.Removed {
			return
		}
		if first == nil {
			first = node
		}
		if node.Path == path {
			selected = node
		}
	})
	if selected == nil {
		selected = first
	}
	if selected != nil {
		s.selectProject(selected, onMain)
	}
}

func walkProjects(node *projectNode, visit func(*projectNode)) {
	if node == nil {
		return
	}
	visit(node)
	for _, child := range node.Children {
		walkProjects(child, visit)
	}
}

func loadSidebarState(path string) (sidebarState, error) {
	var state sidebarState
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := yaml.Unmarshal(data, &state); err != nil {
		return sidebarState{}, err
	}
	for _, root := range state.Roots {
		walkProjects(root, func(n *projectNode) { n.savedOpen = n.Open })
	}
	return state, nil
}

// Match gallery's config/state storage: private YAML files replaced atomically.
func saveYAML(path string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".onegit-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s *settings) saveSidebarState() error {
	if s.statePath == "" {
		return nil
	}
	if s.shell != nil {
		s.state.SidebarWidth = s.shell.LeftWidth()
		s.state.SidebarCollapsed = !s.shell.LeftExpanded()
	}
	if s.state.Roots == nil {
		s.state.Roots = make(map[string]*projectNode)
	}
	if s.projectRoot != "" {
		s.state.Roots[s.projectRoot] = s.projectTree
	}
	if err := saveYAML(s.statePath, s.state); err != nil {
		s.stateProblem = s.text("保存状态失败：") + err.Error()
		return err
	}
	s.stateProblem = ""
	walkProjects(s.projectTree, func(n *projectNode) { n.savedOpen = n.Open })
	return nil
}

// Reuse cached nodes so selection and expansion changes made during a scan
// survive. A path confirmed missing is dropped, along with folders it leaves
// empty; a dropped node stays marked missing so its selection falls back.
func mergeProjects(old, fresh *projectNode, missing map[string]bool) *projectNode {
	if old == nil {
		return fresh
	}
	if fresh != nil && old.Path != fresh.Path {
		return fresh
	}
	if value, checked := missing[old.Path]; checked {
		old.Missing = value
	}
	if fresh != nil {
		old.Repository, old.Bare, old.Missing = fresh.Repository, fresh.Bare, false
	}
	children := map[string]*projectNode{}
	if fresh != nil {
		for _, child := range fresh.Children {
			children[child.Path] = child
		}
	}
	hadChildren := len(old.Children) > 0
	kept := old.Children[:0]
	for _, child := range old.Children {
		if merged := mergeProjects(child, children[child.Path], missing); merged != nil {
			kept = append(kept, merged)
		}
		delete(children, child.Path)
	}
	for _, child := range children {
		kept = append(kept, child)
	}
	old.Children = kept
	if fresh == nil && (old.Missing || !old.Repository && hadChildren && len(kept) == 0) {
		old.Missing = true
		return nil
	}
	sort.Slice(old.Children, func(i, j int) bool { return old.Children[i].Path < old.Children[j].Path })
	return old
}
