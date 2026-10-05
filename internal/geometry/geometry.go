package geometry

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const PointsPerInch = 72.0

type PageSize struct {
	Width  float64
	Height float64
}

type MarginConfig struct {
	Top     float64
	Bottom  float64
	Left    float64
	Right   float64
	Inner   float64
	Outer   float64
	Mirror  bool
	Binding string
}

type Placement struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func ParsePageSize(s string) (PageSize, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a3":
		return PageSize{MmToPoints(297), MmToPoints(420)}, nil
	case "a4":
		return PageSize{MmToPoints(210), MmToPoints(297)}, nil
	case "a5":
		return PageSize{MmToPoints(148), MmToPoints(210)}, nil
	case "letter":
		return PageSize{8.5 * PointsPerInch, 11 * PointsPerInch}, nil
	case "legal":
		return PageSize{8.5 * PointsPerInch, 14 * PointsPerInch}, nil
	}

	re := regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)x([0-9]+(?:\.[0-9]+)?)(mm|cm|in|pt)\s*$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return PageSize{}, fmt.Errorf("invalid --page %q; use A4, Letter, or e.g. 210x297mm", s)
	}
	w, _ := strconv.ParseFloat(m[1], 64)
	h, _ := strconv.ParseFloat(m[2], 64)
	factor, err := UnitToPoints(m[3])
	if err != nil {
		return PageSize{}, err
	}
	if w <= 0 || h <= 0 {
		return PageSize{}, errors.New("page dimensions must be positive")
	}
	return PageSize{w * factor, h * factor}, nil
}

func ParseLength(s string) (float64, error) {
	re := regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)(mm|cm|in|pt)\s*$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid length %q; examples: 12.7mm, 1.5cm, 0.5in", s)
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	factor, err := UnitToPoints(m[2])
	if err != nil {
		return 0, err
	}
	return v * factor, nil
}

func UnitToPoints(unit string) (float64, error) {
	switch strings.ToLower(unit) {
	case "mm":
		return PointsPerInch / 25.4, nil
	case "cm":
		return PointsPerInch / 2.54, nil
	case "in":
		return PointsPerInch, nil
	case "pt":
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
}

func ParseMargins(base, top, bottom, left, right, inner, outer string, mirror bool, binding string) (MarginConfig, error) {
	basePt, err := ParseLength(base)
	if err != nil {
		return MarginConfig{}, fmt.Errorf("--margin: %w", err)
	}
	mc := MarginConfig{Top: basePt, Bottom: basePt, Left: basePt, Right: basePt, Inner: basePt, Outer: basePt, Mirror: mirror, Binding: strings.ToLower(binding)}

	apply := func(name, raw string, target *float64) error {
		if raw == "" {
			return nil
		}
		v, err := ParseLength(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		*target = v
		return nil
	}

	if err := apply("--margin-top", top, &mc.Top); err != nil {
		return MarginConfig{}, err
	}
	if err := apply("--margin-bottom", bottom, &mc.Bottom); err != nil {
		return MarginConfig{}, err
	}
	if err := apply("--margin-left", left, &mc.Left); err != nil {
		return MarginConfig{}, err
	}
	if err := apply("--margin-right", right, &mc.Right); err != nil {
		return MarginConfig{}, err
	}
	if err := apply("--margin-inner", inner, &mc.Inner); err != nil {
		return MarginConfig{}, err
	}
	if err := apply("--margin-outer", outer, &mc.Outer); err != nil {
		return MarginConfig{}, err
	}

	if mc.Binding != "left" && mc.Binding != "right" {
		return MarginConfig{}, fmt.Errorf("--binding must be left or right")
	}
	if mirror && (left != "" || right != "") {
		return MarginConfig{}, errors.New("with --mirror-margins, use --margin-inner/--margin-outer instead of --margin-left/--margin-right")
	}
	if !mirror && (inner != "" || outer != "") {
		return MarginConfig{}, errors.New("--margin-inner and --margin-outer require --mirror-margins")
	}
	return mc, nil
}

func (mc MarginConfig) ForPage(pageNumber int) (top, bottom, left, right float64) {
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

func FitPlacement(pixelW, pixelH int, ps PageSize, top, bottom, left, right float64, align string) Placement {
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
	return Placement{X: x, Y: y, Width: w, Height: h}
}

func MmToPoints(mm float64) float64 { return mm * PointsPerInch / 25.4 }
func PointsToMM(pt float64) float64 { return pt * 25.4 / PointsPerInch }
