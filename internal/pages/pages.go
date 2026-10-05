package pages

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

type Page struct {
	Number int
	Path   string
	Name   string
}

func Discover(dir, prefix string) ([]Page, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read input directory: %w", err)
	}

	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(prefix) + ` p([0-9]+)\.(png|jpe?g|tiff?|bmp)$`)
	seen := map[int]string{}
	var pages []Page

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
		pages = append(pages, Page{Number: n, Path: filepath.Join(dir, entry.Name()), Name: entry.Name()})
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("no supported image files matching %q pNN.<ext> in %s (PNG, JPEG, TIFF, BMP)", prefix, dir)
	}

	sort.Slice(pages, func(i, j int) bool { return pages[i].Number < pages[j].Number })
	return pages, nil
}
