# Install and pair SoloDawn

From the [SoloDawn releases](https://github.com/huanchong-99/SoloDawn/releases),
download `SHA256SUMS` and the binary for your computer **from the same release**.
Save both in an empty directory. Each binary is named
`solodawn_<version>_<os>_<arch>`, with an extra `.exe` on Windows.
`<version>` is the full stable release tag, including its leading `v`.
For example, a Linux x86-64 binary for `v1.10.2` is named
`solodawn_v1.10.2_linux_amd64`.

| System | `<os>` | `<arch>` |
| --- | --- | --- |
| Linux | `linux` | `amd64` for x86-64, `arm64` for AArch64 |
| macOS | `darwin` | `amd64` for Intel, `arm64` for Apple silicon |
| Windows | `windows` | `amd64` for x64, `arm64` for ARM64 |

## Verify and install

In the download directory, set `asset` to the downloaded binary's filename.
The commands below check the SHA-256 digest and filename against the manifest
before installing the executable.

**Linux and macOS** (Bash or Zsh)

```sh
asset='REPLACE_WITH_BINARY_FILENAME'
if [ "$(uname -s)" = Darwin ]; then
  shasum -a 256 "$asset"
else
  sha256sum "$asset"
fi | grep -Fxf SHA256SUMS &&
  mkdir -p "$HOME/.local/bin" &&
  install -m 755 "$asset" "$HOME/.local/bin/solodawn" &&
  if [ "$(uname -s)" = Darwin ]; then
    xattr -d com.apple.quarantine "$HOME/.local/bin/solodawn" 2>/dev/null || true
  fi
```

On success the matching line appears. On macOS the last step clears the
quarantine mark that a browser download carries, which would otherwise
stop the unsigned binary from running; it runs only after the checksum
matched. On macOS with the Application Firewall enabled, allow `solodawn`
when asked, or direct connections to it fail and only the relay works. Add
`$HOME/.local/bin` to your `PATH` if needed.

**Windows** (PowerShell)

```powershell
$asset = 'REPLACE_WITH_BINARY_FILENAME.exe'
$digest = (Get-FileHash -Algorithm SHA256 -LiteralPath $asset -ErrorAction Stop).Hash
if ((Get-Content -LiteralPath SHA256SUMS -ErrorAction Stop) -notcontains "$digest  $asset") {
    throw "SHA256SUMS has no matching digest and filename"
}
$bin = Join-Path $env:LOCALAPPDATA 'Programs\SoloDawn'
New-Item -ItemType Directory -Force -Path $bin | Out-Null
Copy-Item -LiteralPath $asset -Destination (Join-Path $bin 'solodawn.exe')
```

Add the installation directory to your user `Path` or run the executable
by its full path.

## Check Git identity

SoloDawn commits local task merges with your Git identity. Run
`git config --global user.email`; if it is empty, set your name and email
with `git config --global user.name "<name>"` and
`git config --global user.email "<email>"`.

## Pair with the browser

Run `solodawn version` and confirm it prints the tag you downloaded.
Then use the pairing secret from the
onboarding prompt within its ten-minute window. Replace the placeholders
with that secret and a label for this computer.

```text
solodawn pair <secret> --name "<label>"
```

Check the pairing result against the device label and fingerprint shown
in the browser.
