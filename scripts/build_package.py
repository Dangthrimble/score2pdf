"""Build one release-format package, including licences and a checksum."""
import argparse
import datetime
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

TARGETS = {
    "macos-intel": ("darwin", "amd64", ".zip"),
    "macos-apple-silicon": ("darwin", "arm64", ".zip"),
    "windows-x64": ("windows", "amd64", ".zip"),
    "windows-arm64": ("windows", "arm64", ".zip"),
    "linux-x64": ("linux", "amd64", ".tar.gz"),
    "linux-arm64": ("linux", "arm64", ".tar.gz"),
}
ROOT = Path(__file__).resolve().parents[1]


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--target", choices=TARGETS, required=True)
    p.add_argument("--version", required=True)
    p.add_argument("--commit", required=True)
    p.add_argument("--output", type=Path, default=ROOT / "dist" / "packages")
    args = p.parse_args()
    # Go splits -ldflags on whitespace; reject values that could change flags.
    for value in (args.version, args.commit):
        if not value or any(c.isspace() or c in "\"'" for c in value):
            p.error("version and commit must not contain whitespace or quotes")
    goos, arch, extension = TARGETS[args.target]
    args.output.mkdir(parents=True, exist_ok=True)
    archive = args.output / ("score2pdf-" + args.target + extension)
    if archive.exists():
        raise FileExistsError(archive)
    binary = "score2pdf.exe" if goos == "windows" else "score2pdf"
    date = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    with tempfile.TemporaryDirectory(prefix="score2pdf-package-") as temp:
        folder = Path(temp)
        env = os.environ | {"GOOS": goos, "GOARCH": arch, "CGO_ENABLED": "0", "GOFLAGS": "-mod=readonly"}
        subprocess.run([
            "go", "build", "-trimpath", "-ldflags",
            f"-s -w -X main.version={args.version} -X main.commit={args.commit} -X main.date={date}",
            "-o", str(folder / binary), "./cmd/score2pdf",
        ], cwd=ROOT, env=env, check=True)
        (folder / binary).chmod(0o755)
        for name in ("README.md", "LICENSE", "THIRD_PARTY_NOTICES.txt"):
            shutil.copyfile(ROOT / name, folder / name)
            (folder / name).chmod(0o644)
        if extension == ".zip":
            with zipfile.ZipFile(archive, "x", zipfile.ZIP_DEFLATED) as z:
                for path in sorted(folder.iterdir()):
                    z.write(path, path.name)
        else:
            with tarfile.open(archive, "x:gz", format=tarfile.PAX_FORMAT) as t:
                for path in sorted(folder.iterdir()):
                    t.add(path, arcname=path.name, recursive=False)
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    archive.with_name(archive.name + ".sha256").write_text(
        f"{digest}  {archive.name}\n", encoding="utf-8", newline="\n"
    )
    print(f"Built {archive}: {digest}")


if __name__ == "__main__":
    main()
