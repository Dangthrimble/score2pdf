package imageutil

import (
	"bytes"
	"compress/zlib"
	"fmt"
)

func deflate(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (img EncodedImage) Dictionary() string {
	params := ""
	if img.Predictor {
		colors := 1
		if img.ColorSpace == "/DeviceRGB" {
			colors = 3
		}
		params = fmt.Sprintf(" /DecodeParms << /Predictor 15 /Colors %d /BitsPerComponent %d /Columns %d >>", colors, img.Bits, img.Width)
	}
	return fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent %d /Filter /FlateDecode%s /Length %d >>", img.Width, img.Height, img.ColorSpace, img.Bits, params, len(img.Data))
}

// Compare complete image objects, including predictor metadata. Keeping the
// legacy candidate ensures that optimisation never enlarges an image object.
func optimiseImage(raw []byte, channels int, best EncodedImage) (EncodedImage, error) {
	consider := func(data []byte, bits int, predictor bool) error {
		compressed, err := deflate(data)
		if err != nil {
			return err
		}
		candidate := best
		candidate.Data, candidate.Bits, candidate.Predictor = compressed, bits, predictor
		if len(candidate.Dictionary())+len(compressed) < len(best.Dictionary())+len(best.Data) {
			best = candidate
		}
		return nil
	}
	tryFilters := func(data []byte, stride, bpp, bits int) error {
		// Up often suits repeated score lines; adaptive filtering also handles
		// gradients and colour. Both are reversible PNG predictors supported by PDF.
		for _, adaptive := range []bool{false, true} {
			if err := consider(filterRows(data, stride, bpp, adaptive), bits, true); err != nil {
				return err
			}
		}
		return nil
	}
	if err := tryFilters(raw, best.Width*channels, channels, 8); err != nil {
		return EncodedImage{}, err
	}
	if channels == 1 {
		binary := true
		for _, v := range raw {
			if v != 0 && v != 255 {
				binary = false
				break
			}
		}
		if binary {
			stride := (best.Width + 7) / 8
			packed := make([]byte, stride*best.Height)
			for y := 0; y < best.Height; y++ {
				for x := 0; x < best.Width; x++ {
					if raw[y*best.Width+x] == 255 {
						packed[y*stride+x/8] |= 1 << (7 - x%8)
					}
				}
			}
			if err := consider(packed, 1, false); err != nil {
				return EncodedImage{}, err
			}
			if err := tryFilters(packed, stride, 1, 1); err != nil {
				return EncodedImage{}, err
			}
		}
	}
	return best, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	distance := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	da, db, dc := distance(p-int(a)), distance(p-int(b)), distance(p-int(c))
	if da <= db && da <= dc {
		return a
	}
	if db <= dc {
		return b
	}
	return c
}

func filterRows(raw []byte, stride, bpp int, adaptive bool) []byte {
	filtered := make([]byte, 0, len(raw)+len(raw)/stride)
	candidate, best := make([]byte, stride), make([]byte, stride)
	for offset := 0; offset < len(raw); offset += stride {
		first, last := byte(2), byte(2)
		if adaptive {
			first, last = 0, 4
		}
		bestScore, bestType := int(^uint(0)>>1), byte(0)
		for kind := first; kind <= last; kind++ {
			score := 0
			for x := 0; x < stride; x++ {
				var left, up, upperLeft, prediction byte
				if x >= bpp {
					left = raw[offset+x-bpp]
				}
				if offset > 0 {
					up = raw[offset+x-stride]
				}
				if offset > 0 && x >= bpp {
					upperLeft = raw[offset+x-stride-bpp]
				}
				switch kind {
				case 1:
					prediction = left
				case 2:
					prediction = up
				case 3:
					prediction = byte((int(left) + int(up)) / 2)
				case 4:
					prediction = paeth(left, up, upperLeft)
				}
				v := raw[offset+x] - prediction
				candidate[x] = v
				if v < 128 {
					score += int(v)
				} else {
					score += 256 - int(v)
				}
			}
			if score < bestScore {
				bestScore, bestType = score, kind
				copy(best, candidate)
			}
		}
		filtered = append(filtered, bestType)
		filtered = append(filtered, best...)
	}
	return filtered
}
