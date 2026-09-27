# score2pdf

`score2pdf` turns a numbered set of score-page images into a print-ready PDF.

It is designed for a simple workflow: manually remove scanner/book-edge noise and correct any obvious rotation first, then let `score2pdf` trim the remaining white border, fit each page proportionally onto a PDF page, and write the pages in numeric order.

The program is written in pure Go and has no runtime dependency on ImageMagick, Python, `img2pdf`, or any external PDF library. JPEG and PNG use Go's standard image decoders; TIFF and BMP use the pure-Go `golang.org/x/image` decoders, compiled into the executable.

## Input filenames

Use one image file per final PDF page:

```text
Jingle Bells p01.png
Jingle Bells p02.jpg
Jingle Bells p03.tif
...
```

The `p` prefix is required. Page numbers are sorted numerically, so `p2` correctly comes before `p10`; zero-padding (`p01`, `p02`, ...) is still recommended because it also sorts naturally in Finder and Explorer.

If an original scan contains two facing pages, split it into two image files before running `score2pdf`.


## Supported image formats

The following input formats are supported:

- **PNG** (`.png`) — recommended for most scanned scores because it is lossless.
- **JPEG** (`.jpg`, `.jpeg`) — useful for scanner/camera output, though lossy compression can soften fine notation.
- **TIFF** (`.tif`, `.tiff`) — useful for archival and scanning workflows. Standard single-page TIFF is supported.
- **BMP** (`.bmp`) — supported for compatibility, particularly with Windows-based workflows.

Formats can be mixed within the same score:

```text
Jingle Bells p01.png
Jingle Bells p02.jpg
Jingle Bells p03.tif
Jingle Bells p04.bmp
```

The page number, not the image extension, determines ordering. A duplicate page number is an error even if the two files use different formats.

The program deliberately follows a **one image file = one PDF page** rule. Multi-page TIFF files are rejected with an explanatory error; split them into separate `pNN` files first. BigTIFF is not supported.

## Quick start

See [INSTALL.md](INSTALL.md) for downloads, checksum verification, installation
and first-run instructions on macOS, Windows and Linux.

With files such as `Jingle Bells p01.png`, `Jingle Bells p02.jpg`, etc. in the current directory:

```sh
score2pdf "Jingle Bells.pdf"
```

The input prefix defaults to the output PDF basename, so the command above looks for supported files named `Jingle Bells pNN.<ext>`.

To use a different output filename:

```sh
score2pdf --prefix "Jingle Bells" "Jingle Bells Print.pdf"
```

## Defaults

- PDF page size: **A4 portrait** (`210 x 297 mm`)
- Minimum margin: **12.7 mm (0.5 inch)** on all sides
- Horizontal alignment: **centred**
- Vertical alignment: **top**
- White-trim threshold: **242/255** per RGB channel

Each cropped image is enlarged as much as possible inside the available page area while preserving its aspect ratio. The raster image itself is not resampled; the PDF scales it at display/print time.

Because aspect ratio is preserved, one pair of margins may be larger than the specified minimum margin.

## Vertical alignment

Top alignment is the default because score page numbers and headings are commonly near the top:

```sh
score2pdf --align top "Jingle Bells.pdf"
```

Other options are:

```sh
score2pdf --align middle "Jingle Bells.pdf"
score2pdf --align bottom "Jingle Bells.pdf"
```

Horizontal alignment is always centred within the available area between the left and right margins.

## Page sizes

Named sizes:

```sh
score2pdf --page A4 "Jingle Bells.pdf"
score2pdf --page A3 "Jingle Bells.pdf"
score2pdf --page A5 "Jingle Bells.pdf"
score2pdf --page Letter "Jingle Bells.pdf"
score2pdf --page Legal "Jingle Bells.pdf"
```

Custom sizes accept `mm`, `cm`, `in`, or `pt`:

```sh
score2pdf --page 210x297mm "Jingle Bells.pdf"
score2pdf --page 8.5x11in "Jingle Bells.pdf"
```

Swap the dimensions for landscape output, for example `297x210mm`.

## Margins

Set all four margins together:

```sh
score2pdf --margin 15mm "Jingle Bells.pdf"
```

Or override individual margins:

```sh
score2pdf \
  --margin 12.7mm \
  --margin-top 15mm \
  --margin-bottom 15mm \
  --margin-left 18mm \
  --margin-right 12mm \
  "Jingle Bells.pdf"
```

Individual margin options override the base `--margin` value.

## Mirrored margins for double-sided printing

For a conventionally left-bound score:

```sh
score2pdf \
  --mirror-margins \
  --margin 12.7mm \
  --margin-inner 18mm \
  --margin-outer 12.7mm \
  "Jingle Bells.pdf"
```

With left binding:

- odd PDF pages: inner margin on the left, outer margin on the right
- even PDF pages: outer margin on the left, inner margin on the right

For right binding:

```sh
score2pdf \
  --mirror-margins \
  --binding right \
  --margin-inner 18mm \
  --margin-outer 12.7mm \
  "Jingle Bells.pdf"
```

`--margin-left` and `--margin-right` are intentionally not allowed together with `--mirror-margins`; use `--margin-inner` and `--margin-outer` instead.

## Trimming

The program trims surrounding pixels that are sufficiently close to white. The default threshold is `242` on a 0-255 scale:

```sh
score2pdf --trim-threshold 242 "Jingle Bells.pdf"
```

Increase the value to require pixels to be closer to pure white before they are trimmed; decrease it to treat more light-grey pixels as background.

If an input page contains scanner edges, shadows, neighbouring pages, or other dark material outside the score, crop those manually first. `score2pdf` deliberately does not try to decide whether dark material is music or scanner noise.

## Other options

Use source images from another directory:

```sh
score2pdf --input-dir ./pages "Jingle Bells.pdf"
```

By default, an existing output is never replaced, including one created by another process during conversion. Safe publication requires hard-link support (such as APFS, NTFS or ext4); unsupported filesystems fail with an error. For an exFAT/FAT destination, create the PDF on a supported local disk and then copy it.

Replace an existing PDF:

```sh
score2pdf --force "Jingle Bells.pdf"
```

Show build information:

```sh
score2pdf --version
```

Show all options:

```sh
score2pdf -h
```

## Builds

Release builds are produced for six targets:

| Platform | Architecture | Release name |
| --- | --- | --- |
| macOS | Intel 64-bit (`amd64`) | `score2pdf-macos-intel.zip` |
| macOS | Apple Silicon (`arm64`) | `score2pdf-macos-apple-silicon.zip` |
| Windows | x64 (`amd64`) | `score2pdf-windows-x64.zip` |
| Windows | ARM64 (`arm64`) | `score2pdf-windows-arm64.zip` |
| Linux | x64 (`amd64`) | `score2pdf-linux-x64.tar.gz` |
| Linux | ARM64 (`arm64`) | `score2pdf-linux-arm64.tar.gz` |

The binaries are built with `CGO_ENABLED=0`, so no additional runtime libraries are required.

Linux archives preserve executable permissions. Extract the archive for your architecture and run the binary:

```sh
tar -xzf score2pdf-linux-x64.tar.gz
./score2pdf --version
```

Use `score2pdf-linux-arm64.tar.gz` on a 64-bit ARM system. CI runs the tests and standalone binary on both Linux architectures. `SHA256SUMS.txt` covers all six release archives.

## Build from source

Building from source requires **Go 1.26 or newer**. Release builds use Go 1.27.

```sh
go build ./cmd/score2pdf
```

Run the tests:

```sh
go test ./...
```

The GitHub Actions release workflow builds all six supported binaries when a tag beginning with `v` is pushed, for example `v0.1.0`.

Release archives include `THIRD_PARTY_NOTICES.txt` for the Go runtime and image decoders. Release publication waits for tests on Linux, macOS and Windows and native runtime verification of all six packaged executables. The release publishes those exact verified archives.

## Licence

score2pdf is licensed under the [MIT licence](LICENSE), copyright 2026 Dangthrimble.
All release packages include `LICENSE`. Third-party components retain their own
licences, reproduced separately in `THIRD_PARTY_NOTICES.txt`.

## Hosted runtime verification

CI builds, unpacks and executes the release-format package on each of the six
native operating-system/architecture combinations. A separate PDF reader checks
four-format image conversion, numeric page ordering, trimming, A4 geometry,
top/middle/bottom alignment and mirrored margins. The checks also exercise
version metadata, overwrite refusal, forced replacement, preservation of an
existing PDF after a failed conversion, duplicate page rejection and multi-page
TIFF rejection even when the filename extension is misleading.

Each successful job retains its package and SHA-256 checksum plus a JSON report
and sample PDFs as GitHub Actions artifacts. These CI artifacts are development
builds, not published releases. Verification uses Python and pypdf only on the
build/test hosts; the distributed score2pdf executable remains standalone.
