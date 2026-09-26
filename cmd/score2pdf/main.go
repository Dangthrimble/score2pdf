package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const pointsPerInch = 72.0

type pageFile struct {
	Number int
	Path   string
	Name   string
}

type pageSize struct {
	Width  float64
	Height float64
}

type marginConfig struct {
	Top     float64
	Bottom  float64
	Left    float64
	Right   float64
	Inner   float64
	Outer   float64
	Mirror  bool
	Binding string
}

type placement struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type encodedImage struct {
	Width      int
	Height     int
	ColorSpace string
	Data       []byte
}

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

	ps, err := parsePageSize(*pageSpec)
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

	mc, err := parseMargins(*margin, *marginTop, *marginBottom, *marginLeft, *marginRight, *marginInner, *marginOuter, *mirror, *binding)
	if err != nil {
		return err
	}

	alignment := strings.ToLower(*align)
	if alignment != "top" && alignment != "middle" && alignment != "bottom" {
		return fmt.Errorf("--align must be top, middle, or bottom")
	}

	pages, err := discoverPages(*inputDir, *prefix)
	if err != nil {
		return err
	}

	fmt.Printf("Input prefix: %s\n", *prefix)
	fmt.Printf("Page size: %s (%.2f x %.2f mm)\n", *pageSpec, pointsToMM(ps.Width), pointsToMM(ps.Height))
	fmt.Printf("Pages: %d\n", len(pages))
	fmt.Printf("Vertical alignment: %s\n", alignment)
	if mc.Mirror {
		fmt.Printf("Margins: top %.2f mm, bottom %.2f mm, inner %.2f mm, outer %.2f mm, %s binding\n",
			pointsToMM(mc.Top), pointsToMM(mc.Bottom), pointsToMM(mc.Inner), pointsToMM(mc.Outer), mc.Binding)
	} else {
		fmt.Printf("Margins: top %.2f mm, bottom %.2f mm, left %.2f mm, right %.2f mm\n",
			pointsToMM(mc.Top), pointsToMM(mc.Bottom), pointsToMM(mc.Left), pointsToMM(mc.Right))
	}

	return createPDF(output, pages, ps, mc, alignment, uint8(*trimThreshold), *force)
}

func discoverPages(dir, prefix string) ([]pageFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read input directory: %w", err)
	}

	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(prefix) + ` p([0-9]+)\.(png|jpe?g|tiff?|bmp)$`)
	seen := map[int]string{}
	var pages []pageFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := re.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			continue
		}
		if prev, ok := seen[n]; ok {
			return nil, fmt.Errorf("duplicate page number p%d: %q and %q", n, prev, entry.Name())
		}
		seen[n] = entry.Name()
		pages = append(pages, pageFile{Number: n, Path: filepath.Join(dir, entry.Name()), Name: entry.Name()})
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("no supported image files matching %q pNN.<ext> in %s (PNG, JPEG, TIFF, BMP)", prefix, dir)
	}

	sort.Slice(pages, func(i, j int) bool { return pages[i].Number < pages[j].Number })
	return pages, nil
}

func parsePageSize(s string) (pageSize, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a3":
		return pageSize{mmToPoints(297), mmToPoints(420)}, nil
	case "a4":
		return pageSize{mmToPoints(210), mmToPoints(297)}, nil
	case "a5":
		return pageSize{mmToPoints(148), mmToPoints(210)}, nil
	case "letter":
		return pageSize{8.5 * pointsPerInch, 11 * pointsPerInch}, nil
	case "legal":
		return pageSize{8.5 * pointsPerInch, 14 * pointsPerInch}, nil
	}

	re := regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)x([0-9]+(?:\.[0-9]+)?)(mm|cm|in|pt)\s*$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return pageSize{}, fmt.Errorf("invalid --page %q; use A4, Letter, or e.g. 210x297mm", s)
	}
	w, _ := strconv.ParseFloat(m[1], 64)
	h, _ := strconv.ParseFloat(m[2], 64)
	factor, err := unitToPoints(m[3])
	if err != nil {
		return pageSize{}, err
	}
	if w <= 0 || h <= 0 {
		return pageSize{}, errors.New("page dimensions must be positive")
	}
	return pageSize{w * factor, h * factor}, nil
}

func parseLength(s string) (float64, error) {
	re := regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)(mm|cm|in|pt)\s*$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid length %q; examples: 12.7mm, 1.5cm, 0.5in", s)
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	factor, err := unitToPoints(m[2])
	if err != nil {
		return 0, err
	}
	return v * factor, nil
}

func unitToPoints(unit string) (float64, error) {
	switch strings.ToLower(unit) {
	case "mm":
		return pointsPerInch / 25.4, nil
	case "cm":
		return pointsPerInch / 2.54, nil
	case "in":
		return pointsPerInch, nil
	case "pt":
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
}

func parseMargins(base, top, bottom, left, right, inner, outer string, mirror bool, binding string) (marginConfig, error) {
	basePt, err := parseLength(base)
	if err != nil {
		return marginConfig{}, fmt.Errorf("--margin: %w", err)
	}
	mc := marginConfig{Top: basePt, Bottom: basePt, Left: basePt, Right: basePt, Inner: basePt, Outer: basePt, Mirror: mirror, Binding: strings.ToLower(binding)}

	apply := func(name, raw string, target *float64) error {
		if raw == "" {
			return nil
		}
		v, err := parseLength(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		*target = v
		return nil
	}

	if err := apply("--margin-top", top, &mc.Top); err != nil {
		return marginConfig{}, err
	}
	if err := apply("--margin-bottom", bottom, &mc.Bottom); err != nil {
		return marginConfig{}, err
	}
	if err := apply("--margin-left", left, &mc.Left); err != nil {
		return marginConfig{}, err
	}
	if err := apply("--margin-right", right, &mc.Right); err != nil {
		return marginConfig{}, err
	}
	if err := apply("--margin-inner", inner, &mc.Inner); err != nil {
		return marginConfig{}, err
	}
	if err := apply("--margin-outer", outer, &mc.Outer); err != nil {
		return marginConfig{}, err
	}

	if mc.Binding != "left" && mc.Binding != "right" {
		return marginConfig{}, fmt.Errorf("--binding must be left or right")
	}
	if mirror && (left != "" || right != "") {
		return marginConfig{}, errors.New("with --mirror-margins, use --margin-inner/--margin-outer instead of --margin-left/--margin-right")
	}
	if !mirror && (inner != "" || outer != "") {
		return marginConfig{}, errors.New("--margin-inner and --margin-outer require --mirror-margins")
	}
	return mc, nil
}

func (mc marginConfig) forPage(pageNumber int) (top, bottom, left, right float64) {
	if !mc.Mirror {
		return mc.Top, mc.Bottom, mc.Left, mc.Right
	}
	odd := pageNumber%2 == 1
	if mc.Binding == "left" {
		if odd {
			return mc.Top, mc.Bottom, mc.Inner, mc.Outer
		}
		return mc.Top, mc.Bottom, mc.Outer, mc.Inner
	}
	if odd {
		return mc.Top, mc.Bottom, mc.Outer, mc.Inner
	}
	return mc.Top, mc.Bottom, mc.Inner, mc.Outer
}

func createPDF(output string, pages []pageFile, ps pageSize, mc marginConfig, align string, threshold uint8, force bool) error {
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

	pw := newPDFWriter(tmp, 2+3*len(pages))
	if err := pw.writeHeader(); err != nil {
		return err
	}
	if err := pw.writeCatalog(); err != nil {
		return err
	}
	if err := pw.writePagesObject(len(pages)); err != nil {
		return err
	}

	for i, pf := range pages {
		img, err := loadAndTrimImage(pf.Path, threshold)
		if err != nil {
			return fmt.Errorf("%s: %w", pf.Name, err)
		}
		enc, err := encodeImage(img)
		if err != nil {
			return fmt.Errorf("%s: %w", pf.Name, err)
		}

		top, bottom, left, right := mc.forPage(i + 1)
		if left+right >= ps.Width || top+bottom >= ps.Height {
			return fmt.Errorf("page %d margins leave no printable area", i+1)
		}
		place := fitPlacement(enc.Width, enc.Height, ps, top, bottom, left, right, align)

		fmt.Printf("p%02d  %-40s crop %dx%d  placed %.1f x %.1f mm\n",
			i+1, pf.Name, enc.Width, enc.Height, pointsToMM(place.Width), pointsToMM(place.Height))

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

func loadAndTrimImage(path string, threshold uint8) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Inspect the bytes, not the extension: image.Decode also detects formats
	// by content. Validate the same open file that will subsequently be decoded.
	var signature [4]byte
	if _, err := f.ReadAt(signature[:], 0); err != nil {
		return nil, err
	}
	switch string(signature[:]) {
	case "II\x2a\x00", "MM\x00\x2a", "II\x2b\x00", "MM\x00\x2b":
		if err := validateSinglePageTIFF(f); err != nil {
			return nil, err
		}
	}

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if !supportedDecodedFormat(format) {
		return nil, fmt.Errorf("unsupported decoded image format %q", format)
	}

	r, ok := trimBounds(img, threshold)
	if !ok {
		return nil, errors.New("image appears blank after trimming")
	}
	return croppedImage{Image: img, rect: r}, nil
}

func supportedDecodedFormat(format string) bool {
	switch strings.ToLower(format) {
	case "png", "jpeg", "tiff", "bmp":
		return true
	default:
		return false
	}
}

// ensureSinglePageTIFF enforces score2pdf's one-file-per-page rule. Classic
// TIFF stores a pointer to the next image file directory (IFD) after each IFD.
// A non-zero next-IFD pointer means that the TIFF contains another page/image.
// BigTIFF is deliberately rejected because the decoder used by score2pdf does
// not support it.
func ensureSinglePageTIFF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return validateSinglePageTIFF(f)
}

func validateSinglePageTIFF(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < 8 {
		return errors.New("invalid TIFF header")
	}

	header := make([]byte, 8)
	if _, err := f.ReadAt(header, 0); err != nil {
		return err
	}

	var order binary.ByteOrder
	switch string(header[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return errors.New("invalid TIFF byte order")
	}

	magic := order.Uint16(header[2:4])
	if magic == 43 {
		return errors.New("BigTIFF is not supported; use a standard single-page TIFF")
	}
	if magic != 42 {
		return errors.New("invalid TIFF header")
	}

	ifdOffset := int64(order.Uint32(header[4:8]))
	if ifdOffset <= 0 || ifdOffset+2 > info.Size() {
		return errors.New("invalid TIFF IFD offset")
	}

	var countBuf [2]byte
	if _, err := f.ReadAt(countBuf[:], ifdOffset); err != nil {
		return fmt.Errorf("read TIFF IFD: %w", err)
	}
	entryCount := int64(order.Uint16(countBuf[:]))

	nextPos := ifdOffset + 2 + entryCount*12
	if nextPos < ifdOffset || nextPos+4 > info.Size() {
		return errors.New("invalid TIFF IFD")
	}

	var nextBuf [4]byte
	if _, err := f.ReadAt(nextBuf[:], nextPos); err != nil {
		return fmt.Errorf("read TIFF next IFD: %w", err)
	}
	if order.Uint32(nextBuf[:]) != 0 {
		return errors.New("multi-page TIFF is not supported; use one image file per score page")
	}
	return nil
}

type croppedImage struct {
	image.Image
	rect image.Rectangle
}

func (c croppedImage) Bounds() image.Rectangle {
	return image.Rect(0, 0, c.rect.Dx(), c.rect.Dy())
}

func (c croppedImage) At(x, y int) color.Color {
	return c.Image.At(c.rect.Min.X+x, c.rect.Min.Y+y)
}

func trimBounds(img image.Image, threshold uint8) (image.Rectangle, bool) {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if !isWhite(img.At(x, y), threshold) {
				found = true
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if !found {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

func isWhite(c color.Color, threshold uint8) bool {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	if n.A == 0 {
		return true
	}
	composite := func(v uint8) uint8 {
		return uint8((uint32(v)*uint32(n.A) + 255*uint32(255-n.A)) / 255)
	}
	return composite(n.R) >= threshold && composite(n.G) >= threshold && composite(n.B) >= threshold
}

func encodeImage(img image.Image) (encodedImage, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	gray := true
	for y := b.Min.Y; y < b.Max.Y && gray; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl := compositeRGB(img.At(x, y))
			if r != g || g != bl {
				gray = false
				break
			}
		}
	}

	var raw bytes.Buffer
	channels := 3
	space := "/DeviceRGB"
	if gray {
		channels = 1
		space = "/DeviceGray"
	}
	raw.Grow(w * h * channels)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl := compositeRGB(img.At(x, y))
			if gray {
				raw.WriteByte(r)
			} else {
				raw.WriteByte(r)
				raw.WriteByte(g)
				raw.WriteByte(bl)
			}
		}
	}

	var compressed bytes.Buffer
	zw, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		return encodedImage{}, err
	}
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return encodedImage{}, err
	}
	if err := zw.Close(); err != nil {
		return encodedImage{}, err
	}
	return encodedImage{Width: w, Height: h, ColorSpace: space, Data: compressed.Bytes()}, nil
}

func compositeRGB(c color.Color) (uint8, uint8, uint8) {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	if n.A == 255 {
		return n.R, n.G, n.B
	}
	if n.A == 0 {
		return 255, 255, 255
	}
	comp := func(v uint8) uint8 {
		return uint8((uint32(v)*uint32(n.A) + 255*uint32(255-n.A)) / 255)
	}
	return comp(n.R), comp(n.G), comp(n.B)
}

func fitPlacement(pixelW, pixelH int, ps pageSize, top, bottom, left, right float64, align string) placement {
	availW := ps.Width - left - right
	availH := ps.Height - top - bottom
	scaleW := availW / float64(pixelW)
	scaleH := availH / float64(pixelH)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	w := float64(pixelW) * scale
	h := float64(pixelH) * scale
	x := left + (availW-w)/2
	y := bottom
	switch align {
	case "top":
		y = bottom + availH - h
	case "middle":
		y = bottom + (availH-h)/2
	case "bottom":
		y = bottom
	}
	return placement{X: x, Y: y, Width: w, Height: h}
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

func (p *pdfWriter) writePage(index int, ps pageSize, place placement, img encodedImage) error {
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

func mmToPoints(mm float64) float64 { return mm * pointsPerInch / 25.4 }
func pointsToMM(pt float64) float64 { return pt * 25.4 / pointsPerInch }
