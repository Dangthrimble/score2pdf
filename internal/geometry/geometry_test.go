package geometry

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestParseLength(t *testing.T) {
	got, err := ParseLength("12.7mm")
	if err != nil {
		t.Fatal(err)
	}
	if !near(got, 36.0) {
		t.Fatalf("12.7mm = %f pt, want 36", got)
	}

	got, err = ParseLength("0.5in")
	if err != nil {
		t.Fatal(err)
	}
	if !near(got, 36.0) {
		t.Fatalf("0.5in = %f pt, want 36", got)
	}
}

func TestParsePageSize(t *testing.T) {
	a4, err := ParsePageSize("A4")
	if err != nil {
		t.Fatal(err)
	}
	if !near(PointsToMM(a4.Width), 210) || !near(PointsToMM(a4.Height), 297) {
		t.Fatalf("unexpected A4 size: %.2f x %.2f mm", PointsToMM(a4.Width), PointsToMM(a4.Height))
	}

	custom, err := ParsePageSize("8.5x11in")
	if err != nil {
		t.Fatal(err)
	}
	if !near(custom.Width, 612) || !near(custom.Height, 792) {
		t.Fatalf("unexpected custom size: %f x %f", custom.Width, custom.Height)
	}
}

func TestMirroredMarginsLeftBinding(t *testing.T) {
	mc := MarginConfig{Top: 10, Bottom: 11, Inner: 20, Outer: 12, Mirror: true, Binding: "left"}
	_, _, l1, r1 := mc.ForPage(1)
	_, _, l2, r2 := mc.ForPage(2)
	if l1 != 20 || r1 != 12 {
		t.Fatalf("odd page margins = %v/%v", l1, r1)
	}
	if l2 != 12 || r2 != 20 {
		t.Fatalf("even page margins = %v/%v", l2, r2)
	}
}

func TestMirroredMarginsRightBinding(t *testing.T) {
	mc := MarginConfig{Top: 10, Bottom: 11, Inner: 20, Outer: 12, Mirror: true, Binding: "right"}
	_, _, l1, r1 := mc.ForPage(1)
	_, _, l2, r2 := mc.ForPage(2)
	if l1 != 12 || r1 != 20 {
		t.Fatalf("odd page margins = %v/%v", l1, r1)
	}
	if l2 != 20 || r2 != 12 {
		t.Fatalf("even page margins = %v/%v", l2, r2)
	}
}

func TestFitPlacement(t *testing.T) {
	ps := PageSize{Width: 600, Height: 800}
	p := FitPlacement(1000, 1200, ps, 50, 50, 50, 50, "top")
	if p.X < 50 || p.Y < 50 || p.X+p.Width > 550.01 || p.Y+p.Height > 750.01 {
		t.Fatalf("placement outside printable area: %+v", p)
	}
	if !near(p.X, 50) {
		t.Fatalf("expected width-limited image at left margin, got x=%f", p.X)
	}
	if !near(p.Y+p.Height, 750) {
		t.Fatalf("expected top alignment, got top=%f", p.Y+p.Height)
	}
}
