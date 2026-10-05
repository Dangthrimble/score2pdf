package pages

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverNumericOrderMixedFormats(t *testing.T) {
	d := t.TempDir()
	for _, name := range []string{"Piece p10.bmp", "Piece p2.jpg", "Piece p01.png", "Piece p3.tiff", "other.gif"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Discover(d, "Piece")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d pages", len(got))
	}
	want := []int{1, 2, 3, 10}
	for i, n := range want {
		if got[i].Number != n {
			t.Fatalf("wrong page order: %+v", got)
		}
	}
}

func TestDiscoverRejectsDuplicateNumberAcrossFormats(t *testing.T) {
	d := t.TempDir()
	for _, name := range []string{"Piece p01.png", "Piece p1.jpg"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Discover(d, "Piece"); err == nil {
		t.Fatal("expected duplicate page number error")
	}
}
