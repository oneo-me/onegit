package main

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// Project icons bound the chosen file's size and pixel count.
const (
	maxProjectIconBytes  = 16 << 20
	maxProjectIconPixels = 16_000_000
)

var errProjectIconTooLarge = errors.New("image too large")

var projectIconExtensions = []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"}

// projectImage is a repository's chosen icon, or nil, for the placeholder,
// without one, while it loads or when it cannot be read. The first request
// for a path reads it in the background; the result is applied on the UI
// thread.
func (s *settings) projectImage(node *projectNode) ui.ImageSource {
	if node.Icon == "" {
		return nil
	}
	if s.projectIcons == nil {
		s.projectIcons = make(map[string]ui.ImageSource)
	}
	path := node.Icon
	if img, ok := s.projectIcons[path]; ok {
		return img
	}
	s.projectIcons[path] = nil
	go func() {
		img, err := loadProjectIcon(path)
		if err != nil {
			return
		}
		s.repositoryDispatch()(func() {
			s.projectIcons[path] = img
			s.invalidate()
		})
	}()
	return nil
}

// chooseProjectIcon opens an image picker in the project's folder. Tests
// inject the native dialog.
func (s *settings) chooseProjectIcon(node *projectNode) {
	open := s.openDialog
	if open == nil {
		open = mygo.Dialog.Open
	}
	s.startIconSelection(node, open, s.repositoryDispatch())
}

// startIconSelection waits for the picker and decodes the image off the UI
// thread. An image that does not decode leaves the icon unchanged.
func (s *settings) startIconSelection(node *projectNode, open func(mygo.OpenDialogOptions) ([]string, error), onMain func(func())) {
	if s.choosingIcon {
		return
	}
	s.choosingIcon = true
	opts := mygo.OpenDialogOptions{
		Parent: s.window, Title: s.text("选择项目图标"), ButtonLabel: s.text("选择"), DefaultPath: node.Path,
		Filters: []mygo.FileFilter{{Name: s.text("图片"), Extensions: projectIconExtensions}},
	}
	go func() {
		paths, err := open(opts)
		var img ui.ImageSource
		if err == nil && len(paths) > 0 {
			img, err = loadProjectIcon(paths[0])
		}
		onMain(func() {
			s.choosingIcon = false
			switch {
			case err != nil:
				s.toast = &ui.Toast{ID: "project-icon", Title: s.text("无法使用该图片"), Description: err.Error(), Type: components.ToastError}
			case len(paths) > 0:
				if s.projectIcons == nil {
					s.projectIcons = make(map[string]ui.ImageSource)
				}
				node.Icon = filepath.Clean(paths[0])
				s.projectIcons[node.Icon] = img
				s.saveSidebarState()
			}
			s.invalidate()
		})
	}()
}

// resetProjectIcon returns a project to the placeholder icon.
func (s *settings) resetProjectIcon(node *projectNode) {
	delete(s.projectIcons, node.Icon)
	node.Icon = ""
	s.saveSidebarState()
}

// loadProjectIcon reads and decodes an icon file, off the UI thread.
func loadProjectIcon(path string) (ui.ImageSource, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxProjectIconBytes {
		return nil, errProjectIconTooLarge
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return nil, err
		}
		return svg, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if int64(config.Width)*int64(config.Height) > maxProjectIconPixels {
		return nil, errProjectIconTooLarge
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, err
	}
	return bitmap, nil
}
