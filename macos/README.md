---
description: 'The macOS SoloDawn.app bundle, icon, localized permission prompts, build script, and SoloDawnPermissions Full Disk Access helper.'
kind: reference
---

# macOS App Bundle and Permission Helper

## Summary

`macos/` packages the Go `solodawn` executable into `SoloDawn.app` (`org.solodawn.node`) and builds the native Swift helper `SoloDawnPermissions`, which opens PermissionFlow's floating drag panel for Full Disk Access.

## Files

| Path | Purpose |
| --- | --- |
| `Package.swift`, `Package.resolved` | Swift 6.1 package (`macOS >= 13`) pinning PermissionFlow `2.11.4` |
| `Sources/SoloDawnPermissions/` | CLI argument parsing (`--lang`) and AppKit entry point for `PermissionFlow.makeController` |
| `Tests/SoloDawnPermissionsTests/` | Swift Testing suite covering `--lang` mapping, Full Disk Access URL, and `zh-Hans` resources |
| `bundle/Info.plist` | App metadata and English permission usage descriptions |
| `bundle/en.lproj/InfoPlist.strings`, `bundle/zh-Hans.lproj/InfoPlist.strings` | Localized macOS system permission descriptions |
| `bundle/AppIcon.svg`, `bundle/AppIcon.png` | 1024×1024 app icon source and transparent raster PNG |
| `build-app.sh` | Builds `SoloDawnPermissions`, compiles `.icns`, assembles and signs `SoloDawn.app`, verifies and zips |
| `THIRD-PARTY-NOTICES.txt` | Third-party copyright and licence notice copied into `Contents/Resources/` |

## Third-party notices

- **PermissionFlow** `2.11.4` (`https://github.com/jaywcjlove/PermissionFlow`), **MIT License**. Its copyright and permission notice are in [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt) and bundled into `SoloDawn.app/Contents/Resources/THIRD-PARTY-NOTICES.txt`.

## Building

```sh
./macos/build-app.sh <tag> <arch> <go-binary> <out-dir>
```

On Command Line Tools–only Macs where the default SDK lacks `SwiftUIMacros`, `build-app.sh` falls back to `/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk` when `SDKROOT` is unset.
