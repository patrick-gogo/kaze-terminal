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

cd "$repo_root"
# The app's version comes from internal/version, so Finder shows what kaze --version does.
version=$(sed -n 's/^const Number = "\(.*\)"$/\1/p' internal/version/version.go)
if [ -z "$version" ]; then
    echo "could not read the version from internal/version/version.go" >&2
    exit 1
fi

rm -rf "$bundle"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
exe="$bundle/Contents/MacOS/kaze"
if [ "${KAZE_UNIVERSAL:-}" = 1 ]; then
    # One app for Apple Silicon and Intel: build each half, then join them.
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    CGO_ENABLED=1 GOARCH=arm64 CC="clang -arch arm64" "$go_bin" build -trimpath -ldflags "-s -w" -o "$tmp/kaze-arm64" ./cmd/kaze
    CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64" "$go_bin" build -trimpath -ldflags "-s -w" -o "$tmp/kaze-amd64" ./cmd/kaze
    lipo -create -output "$exe" "$tmp/kaze-arm64" "$tmp/kaze-amd64"
else
    "$go_bin" build -trimpath -ldflags "-s -w" -o "$exe" ./cmd/kaze
fi
cp assets/branding/kaze.icns "$bundle/Contents/Resources/Kaze.icns"
cp assets/branding/macos/Info.plist "$bundle/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "$version" "$bundle/Contents/Info.plist"
codesign --force --deep --sign - "$bundle"

echo "Built $bundle (kaze $version)"
