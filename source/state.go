package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

type sidebarState struct {
	SelectedProjects  map[string]string       `yaml:"selectedProjects,omitempty"`
	SelectedWorktrees map[string]string       `yaml:"selectedWorktrees,omitempty"`
	SelectedSections  map[string]string       `yaml:"selectedSections,omitempty"`
	Roots             map[string]*projectNode `yaml:"roots,omitempty"`
	PinnedOpen        *bool                   `yaml:"pinnedOpen,omitempty"`
	OtherOpen         *bool                   `yaml:"otherOpen,omitempty"`
}

func (s *settings) restoreProjectSelection(onMain func(func())) {
	if s.selectedProject != nil {
		if !s.selectedProject.Missing {
			return
		}
		s.cancelWorktrees()
		s.selectedProject = nil
	}
	path := s.state.SelectedProjects[s.projectRoot]
	var selected, first *projectNode
	pinned, _ := sidebarProjects(s.projectTree)
	for _, node := range pinned {
		if !node.Missing {
			first = node
			break
		}
	}
	walkProjects(s.projectTree, func(node *projectNode) {
		if !node.Repository || node.Missing {
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

func (s *settings) saveSidebarState() {
	if s.statePath == "" {
		return
	}
	if s.state.Roots == nil {
		s.state.Roots = make(map[string]*projectNode)
	}
	if s.projectRoot != "" {
		s.state.Roots[s.projectRoot] = s.projectTree
	}
	if err := saveYAML(s.statePath, s.state); err != nil {
		s.stateProblem = s.text("保存状态失败：") + err.Error()
		return
	}
	s.stateProblem = ""
	walkProjects(s.projectTree, func(n *projectNode) { n.savedOpen = n.Open })
}

// Reuse cached nodes so selection and expansion changes made during a scan survive.
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
	for i, child := range old.Children {
		old.Children[i] = mergeProjects(child, children[child.Path], missing)
		delete(children, child.Path)
	}
	for _, child := range children {
		old.Children = append(old.Children, child)
	}
	sort.Slice(old.Children, func(i, j int) bool { return old.Children[i].Path < old.Children[j].Path })
	return old
}
