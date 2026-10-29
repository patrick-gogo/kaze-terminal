#!/bin/sh
set -eu

if [ "$(uname -s)" != Darwin ]; then
    echo "package-macos.sh requires macOS" >&2
    exit 1
fi

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
bundle="$repo_root/dist/Kaze.app"
go_bin=${KAZE_GO_BIN:-go}

if ! command -v "$go_bin" >/dev/null 2>&1; then
    echo "Go 1.27 or newer is required; set KAZE_GO_BIN to its executable path" >&2
    exit 1
fi

mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cd "$repo_root"
"$go_bin" build -o "$bundle/Contents/MacOS/kaze" ./cmd/kaze
cp assets/branding/kaze.icns "$bundle/Contents/Resources/Kaze.icns"
cp assets/branding/macos/Info.plist "$bundle/Contents/Info.plist"
codesign --force --deep --sign - "$bundle"

echo "Built $bundle"
