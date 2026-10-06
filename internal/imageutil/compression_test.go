package imageutil

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"testing"
)

// Use Go's independent PNG decoder to reverse the PDF's PNG predictor, rather
// than mirroring our filter implementation in the test.
func decodeEncoded(t *testing.T, enc EncodedImage) image.Image {
	t.Helper()
	if enc.Predictor {
		var buf bytes.Buffer
		buf.WriteString("\x89PNG\r\n\x1a\n")
		chunk := func(kind string, data []byte) {
			binary.Write(&buf, binary.BigEndian, uint32(len(data)))
			buf.WriteString(kind)
			buf.Write(data)
			binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(kind), data...)))
		}
		header := make([]byte, 13)
		binary.BigEndian.PutUint32(header, uint32(enc.Width))
		binary.BigEndian.PutUint32(header[4:], uint32(enc.Height))
		header[8] = byte(enc.Bits)
		if enc.ColorSpace == "/DeviceRGB" {
			header[9] = 2
		}
		chunk("IHDR", header)
		chunk("IDAT", enc.Data)
		chunk("IEND", nil)
		img, err := png.Decode(&buf)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	zr, err := zlib.NewReader(bytes.NewReader(enc.Data))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	zr.Close()
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, enc.Width, enc.Height))
	for y := 0; y < enc.Height; y++ {
		for x := 0; x < enc.Width; x++ {
			var r, g, b byte
			if enc.Bits == 1 {
				if raw[y*((enc.Width+7)/8)+x/8]&(1<<(7-x%8)) != 0 {
					r = 255
				}
				g, b = r, r
			} else if enc.ColorSpace == "/DeviceGray" {
				r = raw[y*enc.Width+x]
				g, b = r, r
			} else {
				i := (y*enc.Width + x) * 3
				r, g, b = raw[i], raw[i+1], raw[i+2]
			}
			img.SetNRGBA(x, y, color.NRGBA{r, g, b, 255})
		}
	}
	return img
}

func TestLosslessEncoding(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for _, kind := range []string{"binary", "gray", "rgb", "alpha", "noise", "gray16"} {
		for _, w := range []int{1, 7, 8, 9, 257} {
			t.Run(fmt.Sprintf("%s/%d", kind, w), func(t *testing.T) {
				bounds := image.Rect(3, 5, 3+w, 78)
				img := image.NewNRGBA64(bounds)
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						v := uint8((x*7 + y*3) % 256)
						c := color.NRGBA{v, v, v, 255}
						switch kind {
						case "binary":
							c.R = 0
							if (x+y)%13 < 8 {
								c.R = 255
							}
							c.G, c.B = c.R, c.R
						case "rgb":
							c.G, c.B = uint8(x), uint8(y)
						case "alpha":
							c.G, c.B, c.A = uint8(x), uint8(y), uint8(x*y)
						case "noise":
							c.R, c.G, c.B = uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256))
						}
						img.Set(x, y, c)
						if kind == "gray16" {
							v := uint16(x*321 + y*17)
							img.SetNRGBA64(x, y, color.NRGBA64{v, v, v, 65535})
						}
					}
				}
				enc, err := EncodeMode(img, "lossless")
				if err != nil {
					t.Fatal(err)
				}
				old, err := EncodeMode(img, "legacy")
				if err != nil {
					t.Fatal(err)
				}
				if len(enc.Dictionary())+len(enc.Data) > len(old.Dictionary())+len(old.Data) {
					t.Fatal("larger than legacy")
				}
				decoded := decodeEncoded(t, enc)
				for y := 0; y < bounds.Dy(); y++ {
					for x := 0; x < w; x++ {
						r, g, b := compositeRGB(img.At(bounds.Min.X+x, bounds.Min.Y+y))
						got := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA)
						if got != (color.NRGBA{r, g, b, 255}) {
							t.Fatalf("pixel %d,%d changed: %v versus %v", x, y, got, color.NRGBA{r, g, b, 255})
						}
					}
				}
			})
		}
	}
}

func TestPackedBinaryAndPredictor(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 1001, 400))
	rng := rand.New(rand.NewSource(17))
	for i := range img.Pix {
		if rng.Intn(2) == 1 {
			img.Pix[i] = 255
		}
	}
	enc, err := EncodeMode(img, "lossless")
	if err != nil {
		t.Fatal(err)
	}
	if enc.Bits != 1 {
		t.Fatal("expected packed binary")
	}
	decodeEncoded(t, enc)
	// A non-binary grey must never be thresholded into black or white.
	img.Pix[0] = 254
	enc, err = EncodeMode(img, "lossless")
	if err != nil {
		t.Fatal(err)
	}
	if enc.Bits != 8 {
		t.Fatal("near-white grey lost")
	}
	gradient := image.NewGray(image.Rect(0, 0, 1001, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 1001; x++ {
			gradient.Pix[y*1001+x] = byte(x + y)
		}
	}
	enc, err = EncodeMode(gradient, "lossless")
	if err != nil {
		t.Fatal(err)
	}
	if !enc.Predictor {
		t.Fatal("expected predictor")
	}
	decodeEncoded(t, enc)
}

func TestPredictorsWithIndependentDecoder(t *testing.T) {
	for _, bits := range []int{1, 8} {
		for _, channels := range []int{1, 3} {
			if bits == 1 && channels == 3 {
				continue
			}
			for _, width := range []int{1, 7, 8, 9, 65} {
				stride := (width*channels*bits + 7) / 8
				raw := make([]byte, stride*10)
				rand.New(rand.NewSource(int64(width))).Read(raw)
				space := "/DeviceGray"
				if channels == 3 {
					space = "/DeviceRGB"
				}
				base, err := deflate(raw)
				if err != nil {
					t.Fatal(err)
				}
				enc := EncodedImage{Width: width, Height: 10, ColorSpace: space, Bits: bits, Data: base}
				reference := decodeEncoded(t, enc)
				for _, adaptive := range []bool{false, true} {
					enc.Data, err = deflate(filterRows(raw, stride, channels, adaptive))
					if err != nil {
						t.Fatal(err)
					}
					enc.Predictor = true
					got := decodeEncoded(t, enc)
					for y := 0; y < 10; y++ {
						for x := 0; x < width; x++ {
							a := color.NRGBAModel.Convert(reference.At(x, y))
							b := color.NRGBAModel.Convert(got.At(x, y))
							if a != b {
								t.Fatalf("predictor mismatch bits=%d channels=%d width=%d at %d,%d", bits, channels, width, x, y)
							}
						}
					}
				}
			}
		}
	}
}
