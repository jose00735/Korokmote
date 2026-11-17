#!/usr/bin/env bash
# Builds the Windows daemon (x64 + ARM64), the installer and portable zips into dist/.
# Usage: ./build.sh [version]     Requires: Go, makensis (NSIS 3), zip.
set -euo pipefail
VERSION="${1:-$(git describe --tags --always 2>/dev/null | sed 's/^v//' || echo dev)}"
ROOT="$(cd "$(dirname "$0")" && pwd)"
DIST="$ROOT/dist"
rm -rf "$DIST" && mkdir -p "$DIST"
cd "$ROOT/daemon"
for arch in amd64 arm64; do
  out="$DIST/windows-$arch"
  GOOS=windows GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=$VERSION" -o "$out/korokmoted.exe" ./cmd/korokmoted
  GOOS=windows GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$out/korokctl.exe" ./cmd/korokctl
  cp "$ROOT/LICENSE" "$out/LICENSE.txt"
  (cd "$out" && zip -q -9 "$DIST/korokmote-$VERSION-windows-$arch-portable.zip" korokmoted.exe korokctl.exe LICENSE.txt)
done
NSIS_VERSION="$(echo "$VERSION" | grep -oE '^[0-9]+\.[0-9]+\.[0-9]+' || echo 0.0.0)"
(cd "$ROOT/installer" && makensis -V2 -DVERSION="$NSIS_VERSION" -DBINDIR="$DIST/windows-amd64" -DOUTDIR="$DIST" korokmote.nsi)
[ "$NSIS_VERSION" != "$VERSION" ] && mv "$DIST/korokmote-setup-$NSIS_VERSION-x64.exe" "$DIST/korokmote-setup-$VERSION-x64.exe" || true
(cd "$DIST" && sha256sum *.exe *.zip > SHA256SUMS.txt)
ls -la "$DIST"
