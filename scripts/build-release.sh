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
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore" ./cmd/stagecore
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-hub" ./cmd/stagecore-hub
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-osc-plugin" ./cmd/stagecore-osc-plugin
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-pairing" ./cmd/stagecore-pairing
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$bundle/stagecore-setup" ./cmd/stagecore-setup
  )

  cp "$ROOT/deployment/install.sh" "$bundle/install.sh"
  chmod 0755 "$bundle/install.sh" "$bundle/stagecore" "$bundle"/stagecore-*
  printf '%s\n' "$REVISION" > "$bundle/RELEASE_REVISION"

  (
    cd "$bundle"
    sha256sum stagecore stagecore-hub stagecore-osc-plugin stagecore-pairing stagecore-setup > SHA256SUMS
  )

  tar -C "$DIST" -czf "$archive" "$(basename "$bundle")"
  echo "Created $archive"
done

media="$DIST/stagecore-offline-media"
media_archive="$DIST/stagecore-offline-media.tar.gz"
rm -rf "$media" "$media_archive"
mkdir -p "$media/bundles"

for arch in amd64 arm64; do
  cp -a "$DIST/stagecore-linux-$arch" "$media/bundles/"
done

cp "$ROOT/deployment/offline.sh" "$media/stagecore-offline"
chmod 0755 "$media/stagecore-offline"
printf '%s\n' "$REVISION" > "$media/RELEASE_REVISION"
cat > "$media/MEDIA_CATALOG" <<EOF
format=stagecore-offline-media-v1
revision=$REVISION
bundle.linux.amd64=bundles/stagecore-linux-amd64
bundle.linux.arm64=bundles/stagecore-linux-arm64
EOF
cat > "$media/README.txt" <<'EOF'
StageCore Offline Release Media

This directory contains complete Linux amd64 and arm64 StageCore release bundles.
No Internet connection, source checkout, Go compiler, package registry, or cloud account is required after this media is available on the target host.

Recommended path:
  ./stagecore-offline verify
  ./stagecore-offline info
  ./stagecore-offline install

For an existing StageCore installation, use the transactional F-010 update path:
  ./stagecore-offline update

The media SHA-256 manifests detect corruption/substitution relative to the supplied manifests. They do not authenticate the publisher; signed release trust is a separate future boundary unless explicitly added.
EOF

(
  cd "$media"
  {
    printf '%s\n' RELEASE_REVISION MEDIA_CATALOG README.txt stagecore-offline
    find bundles -type f -print | LC_ALL=C sort
  } | while IFS= read -r path; do
    sha256sum "$path"
  done > MEDIA_SHA256SUMS
)

tar -C "$DIST" -czf "$media_archive" "$(basename "$media")"
echo "Created $media_archive"
