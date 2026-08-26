#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/dist"
rm -rf "$DIST"
mkdir -p "$DIST"
for arch in amd64 arm64; do
  out="$DIST/stagecore-hub-linux-$arch"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$out" "$ROOT/cmd/stagecore-hub"
  sha256sum "$out" > "$out.sha256"
done
file "$DIST"/stagecore-hub-linux-*
