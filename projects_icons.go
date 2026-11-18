package main

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
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
// for a path reads it in the background; the result is applied on the
// engine.
func (s *settings) projectImage(node *projectNode) []byte {
	if node.Icon == "" {
		return nil
	}
	if s.projectIcons == nil {
		s.projectIcons = make(map[string][]byte)
	}
	path := node.Icon
	if img, ok := s.projectIcons[path]; ok {
		return img
	}
	s.projectIcons[path] = nil
	post := s.poster()
	go func() {
		img, err := loadProjectIcon(path)
		if err != nil {
			return
		}
		post(func() {
			s.projectIcons[path] = img
			s.invalidate()
		})
	}()
	return nil
}

// chooseProjectIcon opens an image picker in the project's folder and
// decodes the image off the engine. An image that does not decode leaves
// the icon unchanged.
func (s *settings) chooseProjectIcon(node *projectNode) {
	if s.choosingIcon {
		return
	}
	pick := s.pickIcon
	if pick == nil {
		pick = func(o core.PickFilesOptions, done func(string, error)) {
			s.cx.PickFiles(o, func(files []core.File, err error) {
				path := ""
				if len(files) > 0 {
					path = files[0].Path
				}
				done(path, err)
			})
		}
	}
	s.choosingIcon = true
	var types []string
	for _, ext := range projectIconExtensions {
		types = append(types, "."+ext)
	}
	opts := core.PickFilesOptions{Title: s.text("选择项目图标"), Button: s.text("选择"), Start: node.Path, Types: types, TypesName: s.text("图片")}
	pick(opts, func(path string, err error) {
		post := s.poster()
		go func() {
			var img []byte
			if err == nil && path != "" {
				img, err = loadProjectIcon(path)
			}
			post(func() {
				s.choosingIcon = false
				switch {
				case err != nil:
					s.showToast(ui.ToastContent{ID: "project-icon", Type: ui.ToastError, Title: s.text("无法使用该图片"), Description: err.Error()})
				case path != "":
					if s.projectIcons == nil {
						s.projectIcons = make(map[string][]byte)
					}
					node.Icon = filepath.Clean(path)
					s.projectIcons[node.Icon] = img
					s.saveSidebarState()
				}
				s.invalidate()
			})
		}()
	})
}

// resetProjectIcon returns a project to the placeholder icon.
func (s *settings) resetProjectIcon(node *projectNode) {
	delete(s.projectIcons, node.Icon)
	node.Icon = ""
	s.saveSidebarState()
	s.invalidate()
}

// loadProjectIcon reads an icon file and checks that it decodes, off the
// engine.
func loadProjectIcon(path string) ([]byte, error) {
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
		if _, err := core.ParseSVG(data); err != nil {
			return nil, err
		}
		return data, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if int64(config.Width)*int64(config.Height) > maxProjectIconPixels {
		return nil, errProjectIconTooLarge
	}
	return data, nil
}
