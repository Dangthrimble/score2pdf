package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dangthrimble/score2pdf/internal/geometry"
	"github.com/Dangthrimble/score2pdf/internal/imageutil"
	"github.com/Dangthrimble/score2pdf/internal/pages"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
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

type pdfWriter struct {
	cw      *countingWriter
	offsets []int64
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "score2pdf: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("score2pdf", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	prefix := fs.String("prefix", "", "input filename prefix (default: output PDF basename)")
	inputDir := fs.String("input-dir", ".", "directory containing input image files")
	pageSpec := fs.String("page", "A4", "page size: A4, A3, A5, Letter, Legal, or WIDTHxHEIGHTunit")
	margin := fs.String("margin", "12.7mm", "base margin on all sides")
	marginTop := fs.String("margin-top", "", "override top margin")
	marginBottom := fs.String("margin-bottom", "", "override bottom margin")
	marginLeft := fs.String("margin-left", "", "override left margin")
	marginRight := fs.String("margin-right", "", "override right margin")
	mirror := fs.Bool("mirror-margins", false, "alternate inner/outer margins for duplex printing")
	marginInner := fs.String("margin-inner", "", "inner margin when --mirror-margins is enabled")
	marginOuter := fs.String("margin-outer", "", "outer margin when --mirror-margins is enabled")
	binding := fs.String("binding", "left", "binding edge for mirrored margins: left or right")
	align := fs.String("align", "top", "vertical alignment within margins: top, middle, or bottom")
	trimThreshold := fs.Int("trim-threshold", 242, "0-255 threshold; pixels at or above this in RGB are treated as white")
	force := fs.Bool("force", false, "replace an existing output PDF")
	showVersion := fs.Bool("version", false, "print version information")

	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: score2pdf [options] OUTPUT.pdf")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "Input files are named <prefix> pNN.<ext>, for example:")
		fmt.Fprintln(fs.Output(), "  Jingle Bells p01.png")
		fmt.Fprintln(fs.Output(), "  Jingle Bells p02.jpg")
		fmt.Fprintln(fs.Output(), "  Jingle Bells p03.tif")
		fmt.Fprintln(fs.Output(), "Supported: PNG, JPEG, TIFF, BMP")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("score2pdf %s (commit %s, built %s)\n", version, commit, date)
		return nil
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("exactly one output PDF is required")
	}

	output := fs.Arg(0)
	if !strings.EqualFold(filepath.Ext(output), ".pdf") {
		return fmt.Errorf("output must have a .pdf extension: %s", output)
	}

	if *trimThreshold < 0 || *trimThreshold > 255 {
		return fmt.Errorf("--trim-threshold must be between 0 and 255")
	}

	ps, err := geometry.ParsePageSize(*pageSpec)
	if err != nil {
		return err
	}

	if *prefix == "" {
		base := filepath.Base(output)
		*prefix = strings.TrimSuffix(base, filepath.Ext(base))
	}

	if _, err := os.Stat(output); err == nil && !*force {
		return fmt.Errorf("output already exists: %s (use --force to replace it)", output)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	mc, err := geometry.ParseMargins(*margin, *marginTop, *marginBottom, *marginLeft, *marginRight, *marginInner, *marginOuter, *mirror, *binding)
	if err != nil {
		return err
	}

	alignment := strings.ToLower(*align)
	if alignment != "top" && alignment != "middle" && alignment != "bottom" {
		return fmt.Errorf("--align must be top, middle, or bottom")
	}

	pageList, err := pages.Discover(*inputDir, *prefix)
	if err != nil {
		return err
	}

	fmt.Printf("Input prefix: %s\n", *prefix)
	fmt.Printf("Page size: %s (%.2f x %.2f mm)\n", *pageSpec, geometry.PointsToMM(ps.Width), geometry.PointsToMM(ps.Height))
	fmt.Printf("Pages: %d\n", len(pageList))
	fmt.Printf("Vertical alignment: %s\n", alignment)
	if mc.Mirror {
		fmt.Printf("Margins: top %.2f mm, bottom %.2f mm, inner %.2f mm, outer %.2f mm, %s binding\n",
			geometry.PointsToMM(mc.Top), geometry.PointsToMM(mc.Bottom), geometry.PointsToMM(mc.Inner), geometry.PointsToMM(mc.Outer), mc.Binding)
	} else {
		fmt.Printf("Margins: top %.2f mm, bottom %.2f mm, left %.2f mm, right %.2f mm\n",
			geometry.PointsToMM(mc.Top), geometry.PointsToMM(mc.Bottom), geometry.PointsToMM(mc.Left), geometry.PointsToMM(mc.Right))
	}

	return createPDF(output, pageList, ps, mc, alignment, uint8(*trimThreshold), *force)
}

func createPDF(output string, pageList []pages.Page, ps geometry.PageSize, mc geometry.MarginConfig, align string, threshold uint8, force bool) error {
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

	pw := newPDFWriter(tmp, 2+3*len(pageList))
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

	if err := publishPDF(tmpName, output, force); err != nil {
		return err
	}
	fmt.Printf("Created %s\n", output)
	return nil
}

// publishPDF installs a completed file without a check-then-rename race.
// A hard link creates the destination only if it does not already exist.
// On filesystems without hard-link support, fail safely instead of overwriting.
func publishPDF(temp, output string, force bool) error {
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

func newPDFWriter(w io.Writer, totalObjects int) *pdfWriter {
	return &pdfWriter{cw: &countingWriter{w: w}, offsets: make([]int64, totalObjects+1)}
}

func (p *pdfWriter) writeHeader() error {
	_, err := fmt.Fprint(p.cw, "%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	return err
}

func (p *pdfWriter) beginObject(id int) error {
	p.offsets[id] = p.cw.offset
	_, err := fmt.Fprintf(p.cw, "%d 0 obj\n", id)
	return err
}

func (p *pdfWriter) endObject() error {
	_, err := fmt.Fprint(p.cw, "endobj\n")
	return err
}

func (p *pdfWriter) writeCatalog() error {
	if err := p.beginObject(1); err != nil {
		return err
	}
	if _, err := fmt.Fprint(p.cw, "<< /Type /Catalog /Pages 2 0 R >>\n"); err != nil {
		return err
	}
	return p.endObject()
}

func (p *pdfWriter) writePagesObject(count int) error {
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

func (p *pdfWriter) writePage(index int, ps geometry.PageSize, place geometry.Placement, img imageutil.EncodedImage) error {
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

func (p *pdfWriter) finish() error {
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
