package pdf

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Dangthrimble/score2pdf/internal/geometry"
	"github.com/Dangthrimble/score2pdf/internal/imageutil"
	"github.com/Dangthrimble/score2pdf/internal/pages"
)

type countingWriter struct {
	w      io.Writer
	offset int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.offset += int64(n)
	return n, err
}

type writer struct {
	cw      *countingWriter
	offsets []int64
}

func Create(output string, pageList []pages.Page, ps geometry.PageSize, mc geometry.MarginConfig, align string, threshold uint8, force bool) error {
	outDir := filepath.Dir(output)
	if outDir == "" {
		outDir = "."
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(outDir, ".score2pdf-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	pw := newWriter(tmp, 2+3*len(pageList))
	if err := pw.writeHeader(); err != nil {
		return err
	}
	if err := pw.writeCatalog(); err != nil {
		return err
	}
	if err := pw.writePagesObject(len(pageList)); err != nil {
		return err
	}

	for i, pf := range pageList {
		img, err := imageutil.LoadAndTrim(pf.Path, threshold)
		if err != nil {
			return fmt.Errorf("%s: %w", pf.Name, err)
		}
		enc, err := imageutil.Encode(img)
		if err != nil {
			return fmt.Errorf("%s: %w", pf.Name, err)
		}

		top, bottom, left, right := mc.ForPage(i + 1)
		if left+right >= ps.Width || top+bottom >= ps.Height {
			return fmt.Errorf("page %d margins leave no printable area", i+1)
		}
		place := geometry.FitPlacement(enc.Width, enc.Height, ps, top, bottom, left, right, align)

		fmt.Printf("p%02d  %-40s crop %dx%d  placed %.1f x %.1f mm\n",
			i+1, pf.Name, enc.Width, enc.Height, geometry.PointsToMM(place.Width), geometry.PointsToMM(place.Height))

		if err := pw.writePage(i, ps, place, enc); err != nil {
			return err
		}
	}

	if err := pw.finish(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := Publish(tmpName, output, force); err != nil {
		return err
	}
	fmt.Printf("Created %s\n", output)
	return nil
}

// Publish installs a completed file without a check-then-rename race.
// A hard link creates the destination only if it does not already exist.
// On filesystems without hard-link support, fail safely instead of overwriting.
func Publish(temp, output string, force bool) error {
	if force {
		// Do not delete the previous output before attempting replacement.
		return os.Rename(temp, output)
	}
	if err := os.Link(temp, output); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output already exists: %s (use --force to replace it)", output)
		}
		return fmt.Errorf("publish PDF without overwriting (requires hard-link support): %w", err)
	}
	return nil
}

func newWriter(w io.Writer, totalObjects int) *writer {
	return &writer{cw: &countingWriter{w: w}, offsets: make([]int64, totalObjects+1)}
}

func (p *writer) writeHeader() error {
	_, err := fmt.Fprint(p.cw, "%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	return err
}

func (p *writer) beginObject(id int) error {
	p.offsets[id] = p.cw.offset
	_, err := fmt.Fprintf(p.cw, "%d 0 obj\n", id)
	return err
}

func (p *writer) endObject() error {
	_, err := fmt.Fprint(p.cw, "endobj\n")
	return err
}

func (p *writer) writeCatalog() error {
	if err := p.beginObject(1); err != nil {
		return err
	}
	if _, err := fmt.Fprint(p.cw, "<< /Type /Catalog /Pages 2 0 R >>\n"); err != nil {
		return err
	}
	return p.endObject()
}

func (p *writer) writePagesObject(count int) error {
	if err := p.beginObject(2); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(p.cw, "<< /Type /Pages /Count %d /Kids [", count); err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		if _, err := fmt.Fprintf(p.cw, "%d 0 R ", 3+3*i); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(p.cw, "] >>\n"); err != nil {
		return err
	}
	return p.endObject()
}

func (p *writer) writePage(index int, ps geometry.PageSize, place geometry.Placement, img imageutil.EncodedImage) error {
	pageID := 3 + 3*index
	contentID := pageID + 1
	imageID := pageID + 2

	if err := p.beginObject(pageID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(p.cw,
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.4f %.4f] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>\n",
		ps.Width, ps.Height, imageID, contentID); err != nil {
		return err
	}
	if err := p.endObject(); err != nil {
		return err
	}

	content := fmt.Sprintf("q\n%.4f 0 0 %.4f %.4f %.4f cm\n/Im0 Do\nQ\n", place.Width, place.Height, place.X, place.Y)
	if err := p.beginObject(contentID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(p.cw, "<< /Length %d >>\nstream\n", len(content)); err != nil {
		return err
	}
	if _, err := io.WriteString(p.cw, content); err != nil {
		return err
	}
	if _, err := fmt.Fprint(p.cw, "endstream\n"); err != nil {
		return err
	}
	if err := p.endObject(); err != nil {
		return err
	}

	if err := p.beginObject(imageID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(p.cw,
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n",
		img.Width, img.Height, img.ColorSpace, len(img.Data)); err != nil {
		return err
	}
	if _, err := p.cw.Write(img.Data); err != nil {
		return err
	}
	if _, err := fmt.Fprint(p.cw, "\nendstream\n"); err != nil {
		return err
	}
	return p.endObject()
}

func (p *writer) finish() error {
	xref := p.cw.offset
	if _, err := fmt.Fprintf(p.cw, "xref\n0 %d\n", len(p.offsets)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(p.cw, "0000000000 65535 f \n"); err != nil {
		return err
	}
	for i := 1; i < len(p.offsets); i++ {
		if _, err := fmt.Fprintf(p.cw, "%010d 00000 n \n", p.offsets[i]); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(p.cw, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(p.offsets), xref); err != nil {
		return err
	}
	return nil
}
