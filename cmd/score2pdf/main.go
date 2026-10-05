package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dangthrimble/score2pdf/internal/geometry"
	"github.com/Dangthrimble/score2pdf/internal/pages"
	"github.com/Dangthrimble/score2pdf/internal/pdf"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

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

	return pdf.Create(output, pageList, ps, mc, alignment, uint8(*trimThreshold), *force)
}
