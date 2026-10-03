#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="$SCRIPT_DIR/stagecore-camera-relay"
UNIT_SRC="$SCRIPT_DIR/camera-relay.service"
CHECKSUMS="$SCRIPT_DIR/SHA256SUMS"

INSTALL_ROOT="/opt/stagecore"
CONFIG_ROOT="/etc/stagecore"
BIN_DST="$INSTALL_ROOT/bin/stagecore-camera-relay"
ENV_DST="$CONFIG_ROOT/camera-relay.env"
UNIT_DST="/etc/systemd/system/stagecore-camera-relay.service"

SOURCE=""
FLASH_CONTROL=""
LISTEN=""
SOURCE_SET=0
FLASH_SET=0
LISTEN_SET=0

usage() {
  cat <<'EOF'
usage: install-camera-relay.sh [options]

One-time/persistent Camera Relay installer for the StageCore Pi.

Options:
  --source URL          Camera MJPEG URL, e.g. http://stagecam-xxxxxx.local:81/api/v0/stream
  --flash-control URL   Optional camera flash endpoint, e.g. http://stagecam-xxxxxx.local/api/v0/flash
  --listen ADDR         Relay listen address (fresh default: 0.0.0.0:9081)
  -h, --help            Show this help

If /etc/stagecore/camera-relay.env already exists, unspecified values are
preserved. Re-running the installer updates the binary/unit and restarts the
managed service without requiring any per-boot shell command.
EOF
}

if [ "$(id -u)" -ne 0 ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "Camera Relay installation requires root privileges and sudo is unavailable." >&2
    exit 1
  fi
  exec sudo "$0" "$@"
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
    --source)
      [ "$#" -ge 2 ] || { echo "--source requires a value" >&2; exit 2; }
      SOURCE="$2"; SOURCE_SET=1; shift 2 ;;
    --flash-control)
      [ "$#" -ge 2 ] || { echo "--flash-control requires a value" >&2; exit 2; }
      FLASH_CONTROL="$2"; FLASH_SET=1; shift 2 ;;
    --listen)
      [ "$#" -ge 2 ] || { echo "--listen requires a value" >&2; exit 2; }
      LISTEN="$2"; LISTEN_SET=1; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

for required in "$BIN_SRC" "$UNIT_SRC" "$CHECKSUMS"; do
  [ -f "$required" ] || { echo "Missing release asset: $required" >&2; exit 1; }
done
[ -x "$BIN_SRC" ] || { echo "Relay binary is not executable: $BIN_SRC" >&2; exit 1; }

checksum_line="$(grep -E '^[0-9a-fA-F]{64}[[:space:]]+\*?stagecore-camera-relay$' "$CHECKSUMS" | tail -n 1 || true)"
[ -n "$checksum_line" ] || { echo "SHA256SUMS has no stagecore-camera-relay entry" >&2; exit 1; }
expected="$(printf '%s\n' "$checksum_line" | awk '{print $1}')"
actual="$(sha256sum "$BIN_SRC" | awk '{print $1}')"
[ "$actual" = "$expected" ] || {
  echo "Camera Relay checksum mismatch: got $actual want $expected" >&2
  exit 1
}

get_existing() {
  key="$1"
  [ -f "$ENV_DST" ] || return 0
  sed -n "s/^${key}=//p" "$ENV_DST" | tail -n 1
}

if [ -f "$ENV_DST" ]; then
  [ "$SOURCE_SET" -eq 1 ] || SOURCE="$(get_existing STAGECORE_CAMERA_RELAY_SOURCE)"
  [ "$FLASH_SET" -eq 1 ] || FLASH_CONTROL="$(get_existing STAGECORE_CAMERA_FLASH_CONTROL)"
  [ "$LISTEN_SET" -eq 1 ] || LISTEN="$(get_existing STAGECORE_CAMERA_RELAY_LISTEN)"
fi

[ -n "$LISTEN" ] || LISTEN="0.0.0.0:9081"
[ -n "$SOURCE" ] || {
  echo "Camera source is required on first install; pass --source http://<camera>.local:81/api/v0/stream" >&2
  exit 2
}

case "$SOURCE" in
  http://*) ;;
  *) echo "Camera source must use http:// on the isolated Stage LAN" >&2; exit 2 ;;
esac
if [ -n "$FLASH_CONTROL" ]; then
  case "$FLASH_CONTROL" in
    http://*) ;;
    *) echo "Flash control must use http:// on the isolated Stage LAN" >&2; exit 2 ;;
  esac
fi
case "$SOURCE$FLASH_CONTROL$LISTEN" in
  *$'\n'*|*$'\r'*) echo "Configuration values must be single-line" >&2; exit 2 ;;
esac
case "$LISTEN" in
  *:*) ;;
  *) echo "Relay listen must be host:port, got: $LISTEN" >&2; exit 2 ;;
esac

getent group stagecore >/dev/null 2>&1 || {
  echo "StageCore service group is missing; install StageCore Hub first." >&2
  exit 1
}
id -u stagecore >/dev/null 2>&1 || {
  echo "StageCore service user is missing; install StageCore Hub first." >&2
  exit 1
}

install -d -m 0755 "$INSTALL_ROOT/bin" "$CONFIG_ROOT"
install -o root -g root -m 0755 "$BIN_SRC" "$BIN_DST"
install -o root -g root -m 0644 "$UNIT_SRC" "$UNIT_DST"

tmp="$(mktemp "$CONFIG_ROOT/.camera-relay.env.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
{
  printf 'STAGECORE_CAMERA_RELAY_SOURCE=%s\n' "$SOURCE"
  printf 'STAGECORE_CAMERA_RELAY_LISTEN=%s\n' "$LISTEN"
  printf 'STAGECORE_CAMERA_FLASH_CONTROL=%s\n' "$FLASH_CONTROL"
} >"$tmp"
chown root:stagecore "$tmp"
chmod 0640 "$tmp"
mv -f "$tmp" "$ENV_DST"
trap - EXIT

systemctl daemon-reload
systemctl enable stagecore-camera-relay.service
systemctl restart stagecore-camera-relay.service
systemctl is-active --quiet stagecore-camera-relay.service

echo "StageCore Camera Relay install: PASS"
echo "Service: stagecore-camera-relay.service (enabled + active)"
echo "Listen: $LISTEN"
echo "Health: http://127.0.0.1:9081/api/v0/health"
