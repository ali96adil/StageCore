#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SETUP="$SCRIPT_DIR/stagecore-setup"

if [ ! -x "$SETUP" ]; then
  echo "StageCore installer error: $SETUP is missing or not executable." >&2
  echo "Run this script from an unpacked StageCore release bundle." >&2
  exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "StageCore installation requires root privileges and sudo is unavailable." >&2
    echo "Run this installer as root or install sudo using your operating system's supported administration path." >&2
    exit 1
  fi
  exec sudo "$SETUP" install --bundle "$SCRIPT_DIR" "$@"
fi

exec "$SETUP" install --bundle "$SCRIPT_DIR" "$@"
