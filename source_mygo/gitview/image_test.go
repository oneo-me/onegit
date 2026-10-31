package gitview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

func TestImagePreviewDecodesChangedImages(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	pngData := buf.Bytes()
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="4"><rect width="8" height="4"/></svg>`)

	modified := FileChange{Path: "logo.png", OldPath: "logo.png", Status: 'M'}
	p, err := imagePreviewFor(modified, pngData, pngData)
	if err != nil || p == nil || p.before.width != 3 || p.after.height != 2 {
		t.Fatalf("modified png = %+v, %v", p, err)
	}
	added := FileChange{Path: "icon.SVG", OldPath: "icon.SVG", Status: 'A'}
	if p, err := imagePreviewFor(added, nil, svg); err != nil || p == nil || p.before.source != nil || p.after.width != 8 {
		t.Fatalf("added svg = %+v, %v", p, err)
	}
	// Text files and images that do not decode, such as LFS pointers, diff as text.
	for _, tc := range []struct {
		file          FileChange
		before, after []byte
	}{
		{FileChange{Path: "main.go", OldPath: "main.go", Status: 'M'}, []byte("a"), []byte("b")},
		{modified, []byte("version https://git-lfs.github.com/spec/v1\n"), pngData},
		{FileChange{Path: "logo.png", OldPath: "logo.txt", Status: 'R'}, []byte("x"), pngData},
	} {
		if p, err := imagePreviewFor(tc.file, tc.before, tc.after); p != nil || err != nil {
			t.Fatalf("%+v = %+v, %v; want a text diff", tc.file, p, err)
		}
	}

	// A header claiming more pixels than the limit is refused before decoding.
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 10000)
	binary.BigEndian.PutUint32(ihdr[4:], 5000)
	ihdr[8], ihdr[9] = 8, 6
	huge := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")
	huge = append(huge, ihdr...)
	huge = binary.BigEndian.AppendUint32(huge, crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...)))
	if _, err := imagePreviewFor(modified, nil, huge); !errors.Is(err, errImageTooLarge) {
		t.Fatalf("huge png err = %v", err)
	}
}
