package main

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/oneo-me/onegit/gitview"
)

type sidebarState struct {
	Window            windowState                `yaml:"window,omitempty"`
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

// windowState is where the main window was and how large, and whether it
// filled the screen.
type windowState struct {
	X          float32 `yaml:"x,omitempty"`
	Y          float32 `yaml:"y,omitempty"`
	Width      float32 `yaml:"width,omitempty"`
	Height     float32 `yaml:"height,omitempty"`
	Maximized  bool    `yaml:"maximized,omitempty"`
	FullScreen bool    `yaml:"fullscreen,omitempty"`
}

// frame is where the window opens, nil for the default.
func (w windowState) frame() *core.WindowFrame {
	for _, v := range []float32{w.X, w.Y, w.Width, w.Height} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil
		}
	}
	if w.Width <= 0 || w.Height <= 0 {
		return nil
	}
	return &core.WindowFrame{X: core.Points(w.X), Y: core.Points(w.Y), Move: true, Width: core.Points(w.Width), Height: core.Points(w.Height)}
}

// windowSettle is how long after the window or its sidebar last changed
// they are saved, which also outlasts macOS's zoom animation.
const windowSettle = 300 * time.Millisecond

// windowKeeper follows the main window's frame and its sidebar, saving them
// once they settle.
type windowKeeper struct {
	open  *core.Signal[bool]
	width *core.Signal[core.Points]
}

// keepWindow restores the sidebar and the window's state and saves them as
// they change.
func (s *settings) keepWindow(cx *core.Scope) {
	width := core.Points(s.state.SidebarWidth)
	if width <= 0 {
		width = sidebarWidth
	}
	k := &windowKeeper{open: core.NewSignal(cx, !s.state.SidebarCollapsed), width: core.NewSignal(cx, width)}
	s.window = k
	switch {
	case s.state.Window.FullScreen:
		desktop.ToggleFullScreen(cx)
	case s.state.Window.Maximized:
		desktop.ToggleMaximize(cx)
	}
	var stop func() bool
	cx.Effect(func() {
		f, maximized, fullScreen := desktop.Frame(cx), desktop.Maximized(cx), desktop.FullScreen(cx)
		open, width := k.open.Get(), k.width.Get()
		cx.Untrack(func() {
			if f.Width > 0 && f.Height > 0 {
				s.state.Window = windowState{X: float32(f.X), Y: float32(f.Y), Width: float32(f.Width), Height: float32(f.Height),
					Maximized: maximized, FullScreen: fullScreen}
			}
			s.state.SidebarCollapsed, s.state.SidebarWidth = !open, float32(width)
			if stop != nil {
				stop()
			}
			stop = cx.AfterFunc(windowSettle, func() { s.saveSidebarState() })
		})
	})
}

func (s *settings) restoreProjectSelection() {
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
		s.selectProject(selected)
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
