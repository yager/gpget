#!/usr/bin/env bash
# Build the release binaries for every supported target.
#
#   ./scripts/build-release.sh v0.1.0
#   ./scripts/build-release.sh            # version from `git describe`
#
# Output: dist/gpget-<os>-<arch>[.exe] and dist/SHA256SUMS
#
# Must run on macOS: the darwin build needs cgo (UserNotifications + IOKit),
# and cgo cannot be cross-compiled from another OS.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT=dist

# Pin the macOS deployment target. Without it, clang defaults to the version of
# the machine doing the build, so upgrading this Mac would silently raise the
# minimum macOS the release requires -- the build would still succeed and every
# check would still pass. Measured 2026-09-05: a macOS 15.7.7 host produced
# minos 15.0, a macOS 26.6.2 CI runner produced minos 26.0, with the same SDK
# major. Keep this in step with the supported-macOS line in the README.
export MACOSX_DEPLOYMENT_TARGET=15.0

if [ "$(uname -s)" != "Darwin" ]; then
  echo "error: run this on macOS. The darwin build needs cgo and cannot be" >&2
  echo "       cross-compiled; a non-macOS build would silently ship a" >&2
  echo "       darwin binary with no notifications and no USB watching." >&2
  exit 1
fi

rm -rf "$OUT"
mkdir -p "$OUT"

echo "gpget $VERSION"
echo

build() {
  local goos=$1 goarch=$2 cgo=$3 name=$4
  printf '  %-24s ' "$name"
  CGO_ENABLED=$cgo GOOS=$goos GOARCH=$goarch \
    go build -trimpath -ldflags "-X main.version=$VERSION" -o "$OUT/$name" .
  printf 'ok  %s\n' "$(du -h "$OUT/$name" | cut -f1 | tr -d ' ')"
}

# macOS: cgo REQUIRED. Setting GOARCH alone flips CGO_ENABLED to 0, which
# builds a binary with no UserNotifications and no IOKit -- it installs, runs,
# and silently never notifies or detects the camera. Always pass CGO_ENABLED=1.
build darwin  arm64 1 gpget-darwin-arm64

# Linux and Windows use no cgo. CGO_ENABLED=0 keeps them fully static, so one
# Linux binary runs on glibc and musl alike.
build linux   amd64 0 gpget-linux-amd64
build linux   arm64 0 gpget-linux-arm64
build windows amd64 0 gpget-windows-amd64.exe

echo
echo "checks:"

# The darwin binary is the one that can be silently wrong. Prove it linked the
# frameworks rather than trusting that CGO_ENABLED=1 was honoured.
missing=0
for fw in UserNotifications IOKit; do
  if otool -L "$OUT/gpget-darwin-arm64" | grep -q "/$fw.framework/"; then
    echo "  darwin arm64 links $fw"
  else
    echo "  ERROR: darwin arm64 does NOT link $fw" >&2
    missing=1
  fi
done
[ "$missing" -eq 0 ] || { echo "aborting: the macOS build is not usable" >&2; exit 1; }

# Confirm the pin took. A mismatch means the release would demand a different
# macOS than the docs promise.
minos=$(vtool -show-build "$OUT/gpget-darwin-arm64" 2>/dev/null | awk '/minos/{print $2}')
echo "  darwin arm64 requires macOS $minos or newer"
if [ "$minos" != "$MACOSX_DEPLOYMENT_TARGET" ]; then
  echo "  ERROR: expected minos $MACOSX_DEPLOYMENT_TARGET, got $minos" >&2
  exit 1
fi

got=$("$OUT/gpget-darwin-arm64" version)
echo "  version reports: $got"
[ "$got" = "gpget $VERSION" ] || { echo "  ERROR: version stamp missing" >&2; exit 1; }

( cd "$OUT" && shasum -a 256 gpget-* > SHA256SUMS )
echo
echo "dist/:"
ls -1 "$OUT" | sed 's/^/  /'
