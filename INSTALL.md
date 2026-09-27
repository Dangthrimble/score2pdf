# Installing score2pdf

score2pdf is a command-line program. Use Terminal on macOS/Linux or PowerShell
on Windows; it does not open a graphical window. The downloaded executable is
standalone: you do not need Go, Python, ImageMagick or any other runtime.

## Choose a download

Download the archive for your computer and `SHA256SUMS.txt` from the same
[GitHub release](https://github.com/Dangthrimble/score2pdf/releases).
Choose a packaged executable rather than GitHub's automatically generated
"Source code" archives.

- macOS with an Intel processor: `score2pdf-macos-intel.zip`.
- macOS with an Apple M-series chip: `score2pdf-macos-apple-silicon.zip`.
- Windows with an Intel/AMD 64-bit processor: `score2pdf-windows-x64.zip`.
- Windows with an ARM64 processor: `score2pdf-windows-arm64.zip`.
- Linux on an Intel/AMD 64-bit system: `score2pdf-linux-x64.tar.gz`.
- Linux on a 64-bit ARM system: `score2pdf-linux-arm64.tar.gz`.

On macOS, check **Apple menu > About This Mac** for the chip or processor.
On Windows, check **Settings > System > About > System type**. On Linux,
`uname -m` normally reports `x86_64` for x64 or `aarch64` for ARM64.
A Raspberry Pi needs a 64-bit operating system for the ARM64 build.

Each archive contains the executable, `README.md`, this `INSTALL.md`, `LICENSE`
and `THIRD_PARTY_NOTICES.txt`. Keep the documents with the executable when
redistributing it.

## Verify your download

Work in the directory containing the downloaded archive. Compute its SHA-256
hash and compare all 64 hexadecimal characters with the line for that exact
filename in `SHA256SUMS.txt`. Use the checksum file from the same release.
Uppercase and lowercase hexadecimal letters represent the same hash.

macOS, for example:

```sh
shasum -a 256 score2pdf-macos-apple-silicon.zip
```

Windows PowerShell, for example:

```powershell
Get-FileHash -Algorithm SHA256 .\score2pdf-windows-x64.zip
```

Linux, for example:

```sh
sha256sum score2pdf-linux-x64.tar.gz
```

Substitute your archive's name. If the hashes differ, do not run the executable;
download the archive and checksum file again from the release page. A matching
checksum checks the downloaded bytes; it is not a developer signature.

GitHub Actions development artifacts are not published releases. Their outer
artifact ZIP contains a platform archive and its `.sha256` file. Unpack the
outer ZIP first, verify the inner archive against that checksum, then follow
the installation steps below for the inner archive.

## macOS

In Terminal, change to the directory containing your downloaded archive. For
Apple Silicon:

```sh
unzip score2pdf-macos-apple-silicon.zip -d score2pdf-release
cd score2pdf-release
./score2pdf --version
```

For Intel, substitute `score2pdf-macos-intel.zip`. Choose a new extraction
folder when upgrading so an existing installation is not overwritten.

The archive preserves executable permissions. If your extraction tool loses
them, run `chmod u+x score2pdf` inside the extracted folder, then retry.

These builds are not Developer ID-signed or notarised. A browser download may
therefore trigger a Gatekeeper warning even though hosted runtime checks pass.
If you trust the source and have checked the checksum, Apple's per-app
approval may be available under **System Settings > Privacy & Security >
Open Anyway**, after an attempted launch. Follow
[Apple's guidance](https://support.apple.com/en-gb/102445); options can depend on
system policy. Do not disable Gatekeeper globally. A malware or damaged-file
alert warrants investigation, not an automatic override.

## Windows

Open PowerShell in the download directory. For x64:

```powershell
Expand-Archive -LiteralPath .\score2pdf-windows-x64.zip -DestinationPath .\score2pdf-release
Set-Location .\score2pdf-release
.\score2pdf.exe --version
```

For ARM64, substitute `score2pdf-windows-arm64.zip`. Use a new destination
folder for an upgrade. Run the extracted executable, not the copy shown inside
Explorer's ZIP view.

These builds are not Authenticode-signed. SmartScreen may report an unknown
publisher or unrecognised app. Only proceed if you trust the download and its
checksum matches. Windows policy or Smart App Control may prevent execution;
there is not always an individual approval option. See
[Microsoft's guidance](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation).
Do not disable system-wide protection to install this tool; consult your
administrator if a managed computer blocks it.

## Linux

In a terminal in the download directory, for x64:

```sh
mkdir score2pdf-release
tar -xzf score2pdf-linux-x64.tar.gz -C score2pdf-release
cd score2pdf-release
./score2pdf --version
```

For ARM64, substitute `score2pdf-linux-arm64.tar.gz`. These binaries are
statically linked. Extraction preserves executable permissions; no root access
is needed. If execution is denied despite correct permissions, use a location
where execution is permitted (some mounted drives use `noexec`).

## First conversion

Manually crop scanner/book-edge noise and correct rotation before conversion.
Use one image per page, named for example:

```text
Jingle Bells p01.png
Jingle Bells p02.jpg
Jingle Bells p03.tif
```

From the extracted program folder, point to the folder containing those images.
For macOS/Linux, replace `/path/to/scans` with your actual folder:

```sh
./score2pdf --input-dir "/path/to/scans" "Jingle Bells.pdf"
```

In PowerShell, replace the example folder:

```powershell
.\score2pdf.exe --input-dir "C:\path\to\scans" "Jingle Bells.pdf"
```

The PDF is written in the current directory. Defaults are A4 portrait, 12.7 mm
minimum margins and top alignment. Image files are not modified. Existing PDFs
are refused unless you explicitly use `--force`.

Safe output creation requires a filesystem with hard-link support, such as
APFS, NTFS or ext4. For an exFAT/FAT destination, create the PDF on a supported
local disk, then copy it. Do not use `--force` merely to work around this
restriction, because it permits replacing an existing PDF.

Before a large printing run, open a representative result in your PDF reader
and print a sample, including duplex pages if you use mirrored margins. See
`README.md` for page sizes, margins, alignment and supported image formats.

## Running from other folders

You can always invoke the executable by its full path. If the path contains
spaces, quote it. In PowerShell, use the call operator, for example:

```powershell
& "C:\path with spaces\score2pdf.exe" --version
```

Optionally add the extracted program folder to your user `PATH` using your
normal shell or system settings, then open a new terminal. After that,
`score2pdf --version` works without a folder prefix. Adding it to `PATH` is not
required, and administrator privileges are not needed for normal use.

## Compatibility and updates

Hosted native runtime verification currently covers macOS 15 on Intel and
Apple Silicon, Windows Server 2025 x64, Windows 11 ARM64, and Ubuntu 24.04 x64
and ARM64. These are the verification environments, not established minimum
OS versions. Compatibility with older systems, other Linux distributions and
other Windows versions has not been established by those checks.

For an update, download and verify the new archive, extract it into a new
folder, and check `--version`. If you use `PATH`, update that folder entry as
needed. Keep your scans and output PDFs separately from the program folder.
To uninstall, remove the program folder and any `PATH` entry you added.

score2pdf is provided under the MIT licence in `LICENSE`. Third-party notices
remain in `THIRD_PARTY_NOTICES.txt`.
