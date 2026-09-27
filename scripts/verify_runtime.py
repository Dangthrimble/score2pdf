"""Unpack and execute a release package, and independently inspect its PDFs."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import platform
import shutil
import struct
import subprocess
import tarfile
import zipfile

from pypdf import PdfReader
from build_package import ROOT, TARGETS


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def check_pdf(path, align, binding):
    reader = PdfReader(path, strict=True)
    assert len(reader.pages) == 4, "wrong page count"
    for i, (page, size) in enumerate(zip(reader.pages, [(40, 50), (30, 70), (50, 30), (60, 40)])):
        width, height = size
        pw, ph = float(page.mediabox.width), float(page.mediabox.height)
        assert math.isclose(pw, 210 * 72 / 25.4, abs_tol=.001)
        assert math.isclose(ph, 297 * 72 / 25.4, abs_tol=.001)
        images = page["/Resources"]["/XObject"].get_object()
        assert len(images) == 1
        image = next(iter(images.values())).get_object()
        assert image["/Subtype"] == "/Image"
        assert (image["/Width"], image["/Height"]) == size, "wrong ordering or crop"
        assert image["/ColorSpace"] == "/DeviceRGB" and image["/BitsPerComponent"] == 8
        pixels = image.get_data()  # pypdf decodes the image stream independently.
        assert len(pixels) == width * height * 3
        red = pixels[(7 * width + 7) * 3: (7 * width + 7) * 3 + 3]
        assert red[0] > 230 and red[1] < 25 and red[2] < 25, "image pixels corrupted"
        inner_left = (i % 2 == 0) == (binding == "left")
        left, right = ((18, 12) if inner_left else (12, 18))
        left, right = left * 72 / 25.4, right * 72 / 25.4
        available_w, available_h = pw - left - right, ph - 72
        scale = min(available_w / width, available_h / height)
        w, h = width * scale, height * scale
        x = left + (available_w - w) / 2
        y = {"top": ph - 36 - h, "middle": 36 + (available_h - h) / 2, "bottom": 36}[align]
        transforms = [list(map(float, operands)) for operands, op in page.get_contents().operations if op == b"cm"]
        assert len(transforms) == 1
        assert all(math.isclose(a, b, abs_tol=.002) for a, b in zip(transforms[0], [w, 0, 0, h, x, y])), "wrong placement"


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--target", choices=TARGETS, required=True)
    p.add_argument("--archive", type=Path, required=True)
    p.add_argument("--version", required=True)
    p.add_argument("--commit", required=True)
    p.add_argument("--output", type=Path, required=True)
    args = p.parse_args()
    goos, arch, _ = TARGETS[args.target]
    host_os = {"Darwin": "darwin", "Windows": "windows", "Linux": "linux"}[platform.system()]
    host_arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}[platform.machine().lower()]
    assert (host_os, host_arch) == (goos, arch), "verification must run natively"
    archive = args.archive.resolve()
    expected_checksum = archive.with_name(archive.name + ".sha256").read_text().split()[0]
    assert digest(archive) == expected_checksum, "archive checksum mismatch"
    binary_name = "score2pdf.exe" if goos == "windows" else "score2pdf"
    expected = {binary_name, "README.md", "LICENSE", "THIRD_PARTY_NOTICES.txt"}
    if archive.name.endswith(".zip"):
        with zipfile.ZipFile(archive) as z:
            assert len(z.namelist()) == 4 and set(z.namelist()) == expected
            assert z.testzip() is None
            data = {name: z.read(name) for name in expected}
            if goos != "windows":
                assert z.getinfo(binary_name).external_attr >> 16 & 0o111
    else:
        with tarfile.open(archive, "r:gz") as t:
            assert len(t.getmembers()) == 4 and {m.name for m in t.getmembers()} == expected
            assert all(m.isfile() for m in t.getmembers())
            assert t.getmember(binary_name).mode & 0o111
            data = {name: t.extractfile(name).read() for name in expected}
    for name in expected - {binary_name}:
        assert data[name] == (ROOT / name).read_bytes(), f"package {name} differs from source"
    assert b"MIT License" in data["LICENSE"]
    assert b"Copyright (c) 2026 Dangthrimble" in data["LICENSE"]
    work = args.output.resolve()
    work.mkdir(parents=True, exist_ok=False)
    package = work / "unpacked"
    package.mkdir()
    for name, content in data.items():
        (package / name).write_bytes(content)
    binary = package / binary_name
    binary.chmod(0o755)
    info = subprocess.check_output(["go", "version", "-m", str(binary)], text=True)
    assert f"GOOS={goos}" in info and f"GOARCH={arch}" in info and "CGO_ENABLED=0" in info
    checks = []

    def run(arguments, error=None):
        result = subprocess.run([str(binary), *map(str, arguments)], capture_output=True, text=True, timeout=60)
        if error is None:
            assert result.returncode == 0, result.stdout + result.stderr
        else:
            assert result.returncode != 0 and error in result.stderr, result.stdout + result.stderr
        return result.stdout.strip()

    version = run(["--version"])
    assert version.startswith(f"score2pdf {args.version} (commit {args.commit}, built ")
    checks.append("archive checksum, licences, native architecture and version")
    inputs = work / "inputs"
    inputs.mkdir()
    for source in (ROOT / "testdata/runtime").iterdir():
        if source.suffix.lower() in (".png", ".jpg", ".tif", ".bmp"):
            shutil.copyfile(source, inputs / source.name)
    original = {p.name: digest(p) for p in inputs.iterdir()}
    options = ["--input-dir", inputs, "--prefix", "Runtime", "--mirror-margins", "--margin-inner", "18mm", "--margin-outer", "12mm"]
    for align, binding in [("top", "left"), ("middle", "right"), ("bottom", "left")]:
        output = work / f"{align}-{binding}.pdf"
        run([*options, "--align", align, "--binding", binding, output])
        check_pdf(output, align, binding)
    checks.append("PNG/JPEG/TIFF/BMP, numeric ordering, trimming, A4, all alignments, both bindings")
    output = work / "top-left.pdf"
    before = digest(output)
    run([*options, output], error="output already exists")
    assert digest(output) == before
    run([*options, "--force", output])
    check_pdf(output, "top", "left")
    # Conversion failure with --force must retain the previous complete PDF.
    before = digest(output)
    broken = inputs / "Runtime p11.png"
    broken.write_bytes(b"not an image")
    run([*options, "--force", output], error="Runtime p11.png")
    assert digest(output) == before
    broken.unlink()
    duplicate = inputs / "Runtime p1.bmp"
    shutil.copyfile(inputs / "Runtime p10.bmp", duplicate)
    run([*options, work / "duplicate.pdf"], error="duplicate page number")
    assert not (work / "duplicate.pdf").exists()
    duplicate.unlink()
    # A valid second TIFF IFD can share the first image's pixel data.
    first = (inputs / "Runtime p03.tif").read_bytes()
    order = "<" if first[:2] == b"II" else ">"
    offset = struct.unpack_from(order + "I", first, 4)[0]
    next_pos = offset + 2 + 12 * struct.unpack_from(order + "H", first, offset)[0]
    multi = bytearray(first + first)
    struct.pack_into(order + "I", multi, next_pos, len(first) + offset)
    disguised = inputs / "Multipage p01.png"
    disguised.write_bytes(multi)
    run(["--input-dir", inputs, "--prefix", "Multipage", work / "multipage.pdf"], error="multi-page TIFF")
    assert not (work / "multipage.pdf").exists()
    disguised.unlink()
    assert {p.name: digest(p) for p in inputs.iterdir()} == original
    assert not list(work.glob(".score2pdf-*.tmp"))
    checks.append("overwrite refusal, force replacement, failure preservation, duplicates, disguised multi-page TIFF, unchanged sources")
    report = {"target": args.target, "host": platform.platform(), "machine": platform.machine(),
              "commit": args.commit, "version": version, "archive": archive.name,
              "sha256": expected_checksum, "checks": checks, "result": "passed"}
    (work / "verification.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
