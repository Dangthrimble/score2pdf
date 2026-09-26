package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestParseLength(t *testing.T) {
	got, err := parseLength("12.7mm")
	if err != nil {
		t.Fatal(err)
	}
	if !near(got, 36.0) {
		t.Fatalf("12.7mm = %f pt, want 36", got)
	}

	got, err = parseLength("0.5in")
	if err != nil {
		t.Fatal(err)
	}
	if !near(got, 36.0) {
		t.Fatalf("0.5in = %f pt, want 36", got)
	}
}

func TestParsePageSize(t *testing.T) {
	a4, err := parsePageSize("A4")
	if err != nil {
		t.Fatal(err)
	}
	if !near(pointsToMM(a4.Width), 210) || !near(pointsToMM(a4.Height), 297) {
		t.Fatalf("unexpected A4 size: %.2f x %.2f mm", pointsToMM(a4.Width), pointsToMM(a4.Height))
	}

	custom, err := parsePageSize("8.5x11in")
	if err != nil {
		t.Fatal(err)
	}
	if !near(custom.Width, 612) || !near(custom.Height, 792) {
		t.Fatalf("unexpected custom size: %f x %f", custom.Width, custom.Height)
	}
}

func TestMirroredMarginsLeftBinding(t *testing.T) {
	mc := marginConfig{Top: 10, Bottom: 11, Inner: 20, Outer: 12, Mirror: true, Binding: "left"}
	_, _, l1, r1 := mc.forPage(1)
	_, _, l2, r2 := mc.forPage(2)
	if l1 != 20 || r1 != 12 {
		t.Fatalf("odd page margins = %v/%v", l1, r1)
	}
	if l2 != 12 || r2 != 20 {
		t.Fatalf("even page margins = %v/%v", l2, r2)
	}
}

func TestMirroredMarginsRightBinding(t *testing.T) {
	mc := marginConfig{Top: 10, Bottom: 11, Inner: 20, Outer: 12, Mirror: true, Binding: "right"}
	_, _, l1, r1 := mc.forPage(1)
	_, _, l2, r2 := mc.forPage(2)
	if l1 != 12 || r1 != 20 {
		t.Fatalf("odd page margins = %v/%v", l1, r1)
	}
	if l2 != 20 || r2 != 12 {
		t.Fatalf("even page margins = %v/%v", l2, r2)
	}
}

func TestFitPlacement(t *testing.T) {
	ps := pageSize{Width: 600, Height: 800}
	p := fitPlacement(1000, 1200, ps, 50, 50, 50, 50, "top")
	if p.X < 50 || p.Y < 50 || p.X+p.Width > 550.01 || p.Y+p.Height > 750.01 {
		t.Fatalf("placement outside printable area: %+v", p)
	}
	if !near(p.X, 50) {
		t.Fatalf("expected width-limited image at left margin, got x=%f", p.X)
	}
	if !near(p.Y+p.Height, 750) {
		t.Fatalf("expected top alignment, got top=%f", p.Y+p.Height)
	}
}

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

func TestDiscoverPagesNumericOrderMixedFormats(t *testing.T) {
	d := t.TempDir()
	for _, name := range []string{"Piece p10.bmp", "Piece p2.jpg", "Piece p01.png", "Piece p3.tiff", "other.gif"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pages, err := discoverPages(d, "Piece")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 4 {
		t.Fatalf("got %d pages", len(pages))
	}
	want := []int{1, 2, 3, 10}
	for i, n := range want {
		if pages[i].Number != n {
			t.Fatalf("wrong page order: %+v", pages)
		}
	}
}

func TestDiscoverPagesRejectsDuplicateNumberAcrossFormats(t *testing.T) {
	d := t.TempDir()
	for _, name := range []string{"Piece p01.png", "Piece p1.jpg"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := discoverPages(d, "Piece"); err == nil {
		t.Fatal("expected duplicate page number error")
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
			got, err := loadAndTrimImage(path, 242)
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

	if err := ensureSinglePageTIFF(path); err == nil {
		t.Fatal("expected multi-page TIFF error")
	}
}

func TestCreatePDF(t *testing.T) {
	d := t.TempDir()
	for i := 1; i <= 2; i++ {
		img := image.NewNRGBA(image.Rect(0, 0, 120, 160))
		for y := 0; y < 160; y++ {
			for x := 0; x < 120; x++ {
				img.Set(x, y, color.White)
			}
		}
		for y := 20; y < 140; y++ {
			for x := 15; x < 105; x++ {
				if (x+y+i)%11 == 0 {
					img.Set(x, y, color.Black)
				}
			}
		}
		name := filepath.Join(d, "Test p0"+string(rune('0'+i))+".png")
		f, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	pages, err := discoverPages(d, "Test")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(d, "Test.pdf")
	m := marginConfig{Top: 36, Bottom: 36, Left: 36, Right: 36, Inner: 36, Outer: 36, Binding: "left"}
	if err := createPDF(out, pages, pageSize{mmToPoints(210), mmToPoints(297)}, m, "top", 242, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 100 || string(b[:8]) != "%PDF-1.4" {
		t.Fatalf("not a PDF: %q", b[:min(8, len(b))])
	}
}
