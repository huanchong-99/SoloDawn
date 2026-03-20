# GitCortex Windows Installer

Builds a lightweight `.exe` installer for Windows using [Inno Setup](https://jrsoftware.org/isinfo.php).

## What's Bundled

| Component | Purpose |
|-----------|---------|
| `gitcortex-server.exe` | Backend server with embedded frontend |
| `gitcortex-tray.exe` | System tray lifecycle manager |
| Scripts | Encryption key generator, post-install checks |
| `GitCortex.ico` | Application icon |

The installer does **not** bundle Node.js, Git, GitHub CLI, VC++ Runtime, or AI CLI packages. These are expected to be available on the target system.

## System Prerequisites (target machine)

- **Node.js** >= 18
- **Git**
- **npm** (ships with Node.js)

## Build Prerequisites (build machine)

- **Rust** nightly-2025-12-04
- **Node.js** >= 18 with pnpm
- **Inno Setup 6** ([download](https://jrsoftware.org/isdl.php))

## Build

```powershell
# One-click build (compiles + packages)
powershell -ExecutionPolicy Bypass -File build-installer.ps1

# Skip Rust rebuild (if binaries already compiled)
powershell -ExecutionPolicy Bypass -File build-installer.ps1 -SkipRustBuild
```

Output: `output/GitCortex-Setup-v{version}.exe`

## Directory Structure

```
installer/
├── gitcortex.iss           # Inno Setup main script
├── build-installer.ps1     # One-click build script
├── scripts/
│   ├── generate-key.ps1    # Encryption key generator
│   └── post-install-check.ps1  # Post-install verification
├── assets/
│   └── GitCortex.ico       # Application icon
├── build/                  # (gitignored) Build artifacts
└── output/                 # (gitignored) Built installer .exe
```

## Silent Install

```powershell
GitCortex-Setup-v0.0.153.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART
```

## Code Signing

The installer is not currently code-signed. Windows SmartScreen will show a warning on first run. To bypass: click "More info" → "Run anyway".

For production releases, sign with a code-signing certificate:
```powershell
signtool sign /f cert.pfx /p password /tr http://timestamp.digicert.com /td sha256 /fd sha256 GitCortex-Setup-v0.0.153.exe
```
