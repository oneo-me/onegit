package gitview

import (
	"bytes"
	"errors"
	"image"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Image previews bound each side's file size and pixel count.
const (
	maxImageBytes  = 32 << 20
	maxImagePixels = 40_000_000
)

var errImageTooLarge = errors.New("image too large")

// imagePreview is a changed image's two versions; the side a change lacks,
// before an addition or after a deletion, is empty.
type imagePreview struct {
	before, after previewImage
}

type previewImage struct {
	source ui.ImageSource
	// width and height are the image's own size in pixels.
	width, height int
}

// isImagePath reports a file the changes window previews as an image.
func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	}
	return false
}

// imagePreviewFor decodes a changed image's versions, off the UI thread. It
// returns nil for a file that is not an image on each side it has, or that
// does not decode, such as a Git LFS pointer, which then compares as text.
func imagePreviewFor(f FileChange, before, after []byte) (*imagePreview, error) {
	if before == nil && after == nil || before != nil && !isImagePath(f.OldPath) || after != nil && !isImagePath(f.Path) {
		return nil, nil
	}
	if len(before) > maxImageBytes || len(after) > maxImageBytes {
		return nil, errImageTooLarge
	}
	var preview imagePreview
	for _, side := range []struct {
		path string
		data []byte
		into *previewImage
	}{{f.OldPath, before, &preview.before}, {f.Path, after, &preview.after}} {
		if side.data == nil {
			continue
		}
		img, err := decodePreviewImage(side.path, side.data)
		if errors.Is(err, errImageTooLarge) {
			return nil, err
		}
		if err != nil {
			return nil, nil
		}
		*side.into = img
	}
	return &preview, nil
}

func decodePreviewImage(path string, data []byte) (previewImage, error) {
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		svg, err := ui.ParseSVG(data)
		if err != nil {
			return previewImage{}, err
		}
		w, h := svg.Size()
		return previewImage{source: svg, width: int(w), height: int(h)}, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return previewImage{}, err
	}
	if int64(config.Width)*int64(config.Height) > maxImagePixels {
		return previewImage{}, errImageTooLarge
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return previewImage{}, err
	}
	w, h := bitmap.Size()
	return previewImage{source: bitmap, width: w, height: h}, nil
}
