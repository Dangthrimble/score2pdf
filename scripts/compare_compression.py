"""Compare legacy and lossless PDFs, verifying every decoded pixel and placement.

Requires only pypdf (verification dependency). With no input directory, uses
repository fixtures plus deterministic synthetic pages, not real scanned scores.
Outputs go into a new directory; input images and existing PDFs are not modified.
"""
import argparse
import hashlib
import json
from pathlib import Path
import random
import struct
import subprocess
import time
import zlib

from pypdf import PdfReader


def image_pixels(image):
    width, height = int(image['/Width']), int(image['/Height'])
    raw = image.get_data()
    space, bits = image['/ColorSpace'], int(image['/BitsPerComponent'])
    channels = {'/DeviceGray': 1, '/DeviceRGB': 3}[space]
    if bits == 1:
        assert channels == 1
        stride = (width + 7) // 8
        assert len(raw) == stride * height
        raw = bytes(255 if raw[y * stride + x // 8] & (128 >> (x % 8)) else 0
                    for y in range(height) for x in range(width))
    else:
        assert bits == 8 and len(raw) == width * height * channels
    return width, height, space, raw


def compare(binary, inputs, prefix, output):
    output.mkdir(parents=True, exist_ok=False)
    times = {}
    for mode in ('legacy', 'lossless'):
        start = time.perf_counter()
        arguments = [str(binary), '--input-dir', str(inputs), '--prefix', prefix]
        if mode == 'lossless':
            arguments.append('--optimise')
        arguments.append(str(output / f'{mode}.pdf'))
        subprocess.run(arguments,
                       check=True, capture_output=True, text=True)
        times[mode] = round(time.perf_counter() - start, 3)
    old = PdfReader(output / 'legacy.pdf', strict=True)
    new = PdfReader(output / 'lossless.pdf', strict=True)
    assert len(old.pages) == len(new.pages)
    pages = []
    for index, (a, b) in enumerate(zip(old.pages, new.pages), 1):
        assert list(a.mediabox) == list(b.mediabox), 'page geometry changed'
        assert a.get_contents().get_data() == b.get_contents().get_data(), 'placement changed'
        ai = a['/Resources']['/XObject']['/Im0'].get_object()
        bi = b['/Resources']['/XObject']['/Im0'].get_object()
        before, after = image_pixels(ai), image_pixels(bi)
        assert before == after, f'page {index}: decoded pixels changed'
        pages.append({'page': index, 'width': after[0], 'height': after[1],
                      'bits': int(bi['/BitsPerComponent']),
                      'predictor': int(bi.get('/DecodeParms', {}).get('/Predictor', 1)),
                      'pixel_sha256': hashlib.sha256(after[3]).hexdigest()})
    old_size = (output / 'legacy.pdf').stat().st_size
    new_size = (output / 'lossless.pdf').stat().st_size
    assert new_size <= old_size, 'PDF grew'
    report = {'input_prefix': prefix, 'legacy_bytes': old_size, 'lossless_bytes': new_size,
              'saved_percent': round(100 * (old_size - new_size) / old_size, 2),
              'seconds': times, 'identical_pixels_and_placement': True, 'pages': pages}
    (output / 'comparison.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(f'{prefix}: {old_size:,} -> {new_size:,} bytes ({report["saved_percent"]}% saved); pixels identical')
    return report


def synthetic(directory, kind):
    """Generate score-like patterns with gradients/noise; no external imaging library."""
    directory.mkdir(parents=True)
    width, height = 1201, 1600  # Odd width exercises one-bit row padding.
    channels = 3 if kind == 'colour' else 1
    rng = random.Random(20260928)
    raw = bytearray()
    for y in range(height):
        raw.append(0)  # PNG filter None
        for x in range(width):
            interior = 40 <= x < width - 40 and 40 <= y < height - 40
            line = y % 140 in (40, 48, 56, 64, 72)
            note = ((x % 83 - 30) ** 2 / 49 + (y % 140 - 56) ** 2 / 16) <= 1
            stem = x % 83 in (36, 37) and 27 <= y % 140 <= 56
            ink = interior and (line or note or stem)
            v = 0 if ink else 255
            if kind != 'binary' and interior:
                v = 25 if ink else 220 + (x // 50 + y // 60) % 30
                if kind == 'noisy': v = max(0, min(255, v + rng.randrange(-12, 13)))
            if channels == 3:
                raw.extend((v, max(0, v-8), max(0, v-16)))
            else:
                raw.append(v)
    def chunk(kind, data):
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))
    header = struct.pack('>IIBBBBB', width, height, 8, 2 if channels == 3 else 0, 0, 0, 0)
    png = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', header) + chunk(b'IDAT', zlib.compress(raw)) + chunk(b'IEND', b'')
    (directory / f'{kind} p01.png').write_bytes(png)


def verify_suite(binary, output):
    results = [compare(binary, Path(__file__).resolve().parents[1] / 'testdata/runtime',
                       'Runtime', output / 'runtime')]
    for kind in ('binary', 'grey', 'colour', 'noisy'):
        inputs = output / 'inputs' / kind
        synthetic(inputs, kind)
        results.append(compare(binary, inputs, kind, output / kind))
    (output / 'summary.json').write_text(json.dumps(results, indent=2) + '\n', encoding='utf-8')
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path, help='new directory for comparison PDFs and JSON')
    parser.add_argument('--input-dir', type=Path)
    parser.add_argument('--prefix')
    args = parser.parse_args()
    if bool(args.input_dir) != bool(args.prefix):
        parser.error('--input-dir and --prefix must be supplied together')
    if args.output.exists():
        parser.error('--output must not already exist')
    if args.input_dir:
        compare(args.binary.resolve(), args.input_dir.resolve(), args.prefix, args.output.resolve())
    else:
        verify_suite(args.binary.resolve(), args.output.resolve())


if __name__ == '__main__':
    main()
