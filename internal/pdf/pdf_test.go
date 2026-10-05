package pdf

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/Dangthrimble/score2pdf/internal/geometry"
	"github.com/Dangthrimble/score2pdf/internal/pages"
)

func TestCreate(t *testing.T) {
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
	pageList, err := pages.Discover(d, "Test")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(d, "Test.pdf")
	m := geometry.MarginConfig{Top: 36, Bottom: 36, Left: 36, Right: 36, Inner: 36, Outer: 36, Binding: "left"}
	if err := Create(out, pageList, geometry.PageSize{Width: geometry.MmToPoints(210), Height: geometry.MmToPoints(297)}, m, "top", 242, false); err != nil {
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

func TestPublishReplacement(t *testing.T) {
	d := t.TempDir()
	dest := filepath.Join(d, "existing.pdf")
	source := filepath.Join(d, "completed.tmp")
	if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	// Failure to replace must not remove the previous PDF.
	if err := Publish(source, dest, true); err == nil {
		t.Fatal("expected missing source error")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "original" {
		t.Fatalf("original lost: %q, %v", got, err)
	}
	if err := os.WriteFile(source, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Publish(source, dest, false); err == nil {
		t.Fatal("overwrote without force")
	}
	if err := Publish(source, dest, true); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(dest)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("replacement failed: %q, %v", got, err)
	}
}
