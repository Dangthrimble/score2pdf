package imageutil

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

func TestTrimBounds(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.White)
		}
	}
	for y := 4; y < 15; y++ {
		for x := 3; x < 17; x++ {
			img.Set(x, y, color.Black)
		}
	}
	r, ok := trimBounds(img, 242)
	if !ok {
		t.Fatal("expected nonblank image")
	}
	want := image.Rect(3, 4, 17, 15)
	if r != want {
		t.Fatalf("trim bounds = %v, want %v", r, want)
	}
}

func TestLoadAndTrimSupportedFormats(t *testing.T) {
	d := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 40, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.White)
		}
	}
	for y := 8; y < 42; y++ {
		for x := 6; x < 34; x++ {
			img.Set(x, y, color.Black)
		}
	}

	writers := []struct {
		name  string
		write func(*os.File) error
	}{
		{"Page p01.png", func(f *os.File) error { return png.Encode(f, img) }},
		{"Page p02.jpg", func(f *os.File) error { return jpeg.Encode(f, img, &jpeg.Options{Quality: 100}) }},
		{"Page p03.bmp", func(f *os.File) error { return bmp.Encode(f, img) }},
		{"Page p04.tif", func(f *os.File) error { return tiff.Encode(f, img, nil) }},
	}

	for _, tc := range writers {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(d, tc.name)
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.write(f); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := LoadAndTrim(path, 242)
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds().Dx() <= 0 || got.Bounds().Dy() <= 0 {
				t.Fatalf("empty trimmed image: %v", got.Bounds())
			}
		})
	}
}

func TestEnsureSinglePageTIFFRejectsMultipleIFDs(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "multi.tif")

	// Minimal little-endian classic TIFF with two empty IFDs. The decoder is
	// never called; this tests only score2pdf's one-file-per-page guard.
	b := make([]byte, 24)
	copy(b[0:2], []byte("II"))
	binary.LittleEndian.PutUint16(b[2:4], 42)
	binary.LittleEndian.PutUint32(b[4:8], 8)
	binary.LittleEndian.PutUint16(b[8:10], 0)   // first IFD: zero entries
	binary.LittleEndian.PutUint32(b[10:14], 14) // next IFD offset
	binary.LittleEndian.PutUint16(b[14:16], 0)  // second IFD: zero entries
	binary.LittleEndian.PutUint32(b[16:20], 0)  // end of chain
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureSinglePageTIFF(path); err == nil {
		t.Fatal("expected multi-page TIFF error")
	}
}
