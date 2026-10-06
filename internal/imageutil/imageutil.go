package imageutil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

type EncodedImage struct {
	Width      int
	Height     int
	ColorSpace string
	Bits       int
	Predictor  bool
	Data       []byte
}

func LoadAndTrim(path string, threshold uint8) (image.Image, error) {
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

// EnsureSinglePageTIFF enforces score2pdf's one-file-per-page rule. Classic
// TIFF stores a pointer to the next image file directory (IFD) after each IFD.
// A non-zero next-IFD pointer means that the TIFF contains another page/image.
// BigTIFF is deliberately rejected because the decoder used by score2pdf does
// not support it.
func EnsureSinglePageTIFF(path string) error {
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

func Encode(img image.Image) (EncodedImage, error) {
	return EncodeMode(img, "legacy")
}

// EncodeMode encodes with "legacy" (fast Flate) or "lossless" (optional size optimisation).
func EncodeMode(img image.Image, mode string) (EncodedImage, error) {
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

	enc := EncodedImage{Width: w, Height: h, ColorSpace: space, Bits: 8}
	data, err := deflate(raw.Bytes())
	if err != nil {
		return EncodedImage{}, err
	}
	enc.Data = data
	if mode == "legacy" {
		return enc, nil
	}
	return optimiseImage(raw.Bytes(), channels, enc)
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
