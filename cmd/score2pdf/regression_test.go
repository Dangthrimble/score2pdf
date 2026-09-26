package main

import (
	"bytes"
	"encoding/binary"
	"golang.org/x/image/tiff"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMultipageTIFFWithPNGExtension(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	if err := tiff.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	first := buf.Bytes()
	var order binary.ByteOrder = binary.LittleEndian
	if string(first[:2]) == "MM" {
		order = binary.BigEndian
	}
	off := int(order.Uint32(first[4:8]))
	next := off + 2 + int(order.Uint16(first[off:off+2]))*12
	both := append(append([]byte(nil), first...), first...)
	order.PutUint32(both[next:next+4], uint32(len(first)+off))
	path := filepath.Join(t.TempDir(), "Piece p01.png")
	if err := os.WriteFile(path, both, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureSinglePageTIFF(path); err == nil {
		t.Fatal("fixture should have multiple TIFF pages")
	}
	if _, err := loadAndTrimImage(path, 242); err == nil || !strings.Contains(err.Error(), "multi-page TIFF") {
		t.Fatalf("expected multi-page TIFF error, got %v", err)
	}
}

func TestOutputCreatedDuringConversion(t *testing.T) {
	entered, resume := make(chan struct{}), make(chan struct{})
	image.RegisterFormat("png", "REVIEWRACE", func(r io.Reader) (image.Image, error) {
		close(entered)
		<-resume
		img := image.NewGray(image.Rect(0, 0, 10, 10))
		img.Set(0, 0, color.Black)
		return img, nil
	}, nil)
	d := t.TempDir()
	input := filepath.Join(d, "Piece p01.png")
	output := filepath.Join(d, "Piece.pdf")
	if err := os.WriteFile(input, []byte("REVIEWRACE"), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- run([]string{"--input-dir", d, output}) }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("conversion failed before decoding: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("conversion did not reach decoder")
	}
	original := []byte("existing PDF from another process")
	if err := os.WriteFile(output, original, 0600); err != nil {
		close(resume)
		t.Fatal(err)
	}
	close(resume)
	err := <-done
	if err == nil || !strings.Contains(err.Error(), "output already exists") {
		t.Fatalf("expected collision error, got %v", err)
	}
	got, readErr := os.ReadFile(output)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("output overwritten without --force; conversion error: %v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(d, ".score2pdf-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary output was not cleaned up: %v, %v", leftovers, err)
	}
}

func TestPublishPDFReplacement(t *testing.T) {
	d := t.TempDir()
	dest := filepath.Join(d, "existing.pdf")
	source := filepath.Join(d, "completed.tmp")
	if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	// Failure to replace must not remove the previous PDF.
	if err := publishPDF(source, dest, true); err == nil {
		t.Fatal("expected missing source error")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "original" {
		t.Fatalf("original lost: %q, %v", got, err)
	}
	if err := os.WriteFile(source, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishPDF(source, dest, false); err == nil {
		t.Fatal("overwrote without force")
	}
	if err := publishPDF(source, dest, true); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(dest)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("replacement failed: %q, %v", got, err)
	}
}

func TestBigTIFFWithPNGExtension(t *testing.T) {
	for _, header := range []string{"II\x2b\x00\x08\x00\x00\x00", "MM\x00\x2b\x00\x08\x00\x00"} {
		path := filepath.Join(t.TempDir(), "Piece p01.png")
		if err := os.WriteFile(path, []byte(header), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadAndTrimImage(path, 242); err == nil || !strings.Contains(err.Error(), "BigTIFF") {
			t.Fatalf("expected BigTIFF error, got %v", err)
		}
	}
}

func TestSinglePageTIFFWithPNGExtension(t *testing.T) {
	var b bytes.Buffer
	if err := tiff.Encode(&b, image.NewGray(image.Rect(0, 0, 12, 18)), nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Piece p01.png")
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	img, err := loadAndTrimImage(path, 242)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 12, 18) {
		t.Fatalf("wrong bounds: %v", img.Bounds())
	}
}
