#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 <tag> <arch> <go-binary> <out-dir>" >&2
  exit 2
fi

tag="$1"
arch="$2"
go_binary="$3"
out_dir="$4"

case "$arch" in
  arm64)
    swift_arch="arm64"
    go_arch="arm64"
    ;;
  amd64 | x86_64)
    swift_arch="x86_64"
    go_arch="amd64"
    ;;
  *)
    echo "unsupported arch: $arch (expected arm64, amd64, or x86_64)" >&2
    exit 2
    ;;
esac

if [[ ! -f "$go_binary" ]]; then
  echo "go binary not found: $go_binary" >&2
  exit 1
fi

bundle_version="${tag#v}"
bundle_version="${bundle_version%%-rc.*}"
if [[ ! "$bundle_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "cannot derive numeric bundle version from tag: $tag" >&2
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# P0 8.11: Command Line Tools 27.2 ships MacOSX27.2.sdk without Xcode's SwiftUIMacros plugin,
# which fails PermissionFlow's @State macro expansion. Fall back to CLT's MacOSX26.5.sdk when
# SDKROOT is unset and no Xcode developer directory is selected.
if [[ -z "${SDKROOT:-}" ]] && [[ "$(xcode-select -p 2>/dev/null || true)" == "/Library/Developer/CommandLineTools" ]]; then
  clt_sdk="/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk"
  if [[ -d "$clt_sdk" ]] && [[ -z "$(find /Library/Developer/CommandLineTools -name '*SwiftUIMacros*' -print -quit 2>/dev/null || true)" ]]; then
    export SDKROOT="$clt_sdk"
  fi
fi

(
  cd "$script_dir"
  swift build -c release --arch "$swift_arch"
)

bin_path="$(
  cd "$script_dir"
  swift build -c release --arch "$swift_arch" --show-bin-path
)"

helper_bin="$bin_path/SoloDawnPermissions"
res_bundle="$bin_path/PermissionFlow_PermissionFlow.bundle"

if [[ ! -x "$helper_bin" ]]; then
  echo "built helper not found: $helper_bin" >&2
  exit 1
fi
if [[ ! -d "$res_bundle" ]]; then
  echo "PermissionFlow resource bundle not found: $res_bundle" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

iconset="$tmp_dir/SoloDawn.iconset"
mkdir -p "$iconset"
src_png="$script_dir/bundle/AppIcon.png"

render_icon() {
  local size="$1"
  local name="$2"
  sips -z "$size" "$size" "$src_png" --out "$iconset/$name" >/dev/null
}

render_icon 16 icon_16x16.png
render_icon 32 icon_16x16@2x.png
render_icon 32 icon_32x32.png
render_icon 64 icon_32x32@2x.png
render_icon 128 icon_128x128.png
render_icon 256 icon_128x128@2x.png
render_icon 256 icon_256x256.png
render_icon 512 icon_256x256@2x.png
render_icon 512 icon_512x512.png
cp "$src_png" "$iconset/icon_512x512@2x.png"

app="$tmp_dir/SoloDawn.app"
contents="$app/Contents"
macos_dir="$contents/MacOS"
resources="$contents/Resources"

mkdir -p "$macos_dir" "$resources/en.lproj" "$resources/zh-Hans.lproj"

iconutil -c icns "$iconset" -o "$resources/SoloDawn.icns"

cp "$script_dir/bundle/Info.plist" "$contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $bundle_version" "$contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $bundle_version" "$contents/Info.plist"

cp "$script_dir/bundle/en.lproj/InfoPlist.strings" "$resources/en.lproj/InfoPlist.strings"
cp "$script_dir/bundle/zh-Hans.lproj/InfoPlist.strings" "$resources/zh-Hans.lproj/InfoPlist.strings"
cp "$script_dir/THIRD-PARTY-NOTICES.txt" "$resources/THIRD-PARTY-NOTICES.txt"
cp -R "$res_bundle" "$resources/PermissionFlow_PermissionFlow.bundle"

cp "$go_binary" "$macos_dir/solodawn"
cp "$helper_bin" "$macos_dir/SoloDawnPermissions"
chmod 0755 "$macos_dir/solodawn" "$macos_dir/SoloDawnPermissions"

codesign -s - -f "$macos_dir/SoloDawnPermissions"
codesign -s - -f "$app"

codesign --verify --deep --strict "$app"
codesign -dv "$app" 2>&1 | grep -Fx "Identifier=org.solodawn.node"
plutil -lint \
  "$contents/Info.plist" \
  "$resources/en.lproj/InfoPlist.strings" \
  "$resources/zh-Hans.lproj/InfoPlist.strings"

if [[ "$(uname -m)" == "$swift_arch" ]] || arch -"$swift_arch" /usr/bin/true 2>/dev/null; then
  got_version="$("$macos_dir/solodawn" version)"
  if [[ "$got_version" != "$tag" ]]; then
    echo "expected solodawn version '$tag', got '$got_version'" >&2
    exit 1
  fi
  "$macos_dir/SoloDawnPermissions" --verify-resources
fi

mkdir -p "$out_dir"
zip_path="$out_dir/solodawn_${tag}_darwin_${go_arch}.zip"
rm -f "$zip_path"
ditto -c -k --norsrc --noextattr --keepParent "$app" "$zip_path"
