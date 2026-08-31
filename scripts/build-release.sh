#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${1:-$ROOT/dist}"
REVISION="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || printf 'unknown')"

mkdir -p "$DIST"

for arch in amd64 arm64; do
  bundle="$DIST/stagecore-linux-$arch"
  archive="$DIST/stagecore-linux-$arch.tar.gz"
  rm -rf "$bundle" "$archive"
  mkdir -p "$bundle"

  echo "Building StageCore linux/$arch release bundle..."
  (
    cd "$ROOT"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-hub" ./cmd/stagecore-hub
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-osc-plugin" ./cmd/stagecore-osc-plugin
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-pairing" ./cmd/stagecore-pairing
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-setup" ./cmd/stagecore-setup
  )

  cp "$ROOT/deployment/install.sh" "$bundle/install.sh"
  chmod 0755 "$bundle/install.sh" "$bundle"/stagecore-*
  printf '%s\n' "$REVISION" > "$bundle/RELEASE_REVISION"

  (
    cd "$bundle"
    sha256sum stagecore-hub stagecore-osc-plugin stagecore-pairing stagecore-setup > SHA256SUMS
  )

  tar -C "$DIST" -czf "$archive" "$(basename "$bundle")"
  echo "Created $archive"
done
