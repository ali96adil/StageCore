#!/usr/bin/env bash
set -euo pipefail

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "run this installer through sudo" >&2
  exit 77
fi

TARGET_USER="${1:-}"
if [[ ! "$TARGET_USER" =~ ^[a-z_][a-z0-9_-]*[$]?$ ]]; then
  echo "invalid qualification user" >&2
  exit 64
fi

SOURCE_HELPER="/tmp/stagecore-qualification-helper"
SOURCE_PROBE="/tmp/stagecore-qualification-probe"
SOURCE_COMMAND="/tmp/stagecore-qualification-command"
DEST_HELPER="/usr/local/libexec/stagecore-qualification-helper"
DEST_PROBE="/usr/local/libexec/stagecore-qualification-probe"
DEST_COMMAND="/usr/local/libexec/stagecore-qualification-command"
SUDOERS="/etc/sudoers.d/stagecore-qualification"

[[ -f "$SOURCE_HELPER" ]] || { echo "missing $SOURCE_HELPER" >&2; exit 66; }
[[ -f "$SOURCE_PROBE" ]] || { echo "missing $SOURCE_PROBE" >&2; exit 66; }
[[ -f "$SOURCE_COMMAND" ]] || { echo "missing $SOURCE_COMMAND" >&2; exit 66; }

install -d -o root -g root -m 0755 /usr/local/libexec
install -o root -g root -m 0755 "$SOURCE_HELPER" "$DEST_HELPER"
install -o root -g root -m 0755 "$SOURCE_PROBE" "$DEST_PROBE"
install -o root -g root -m 0755 "$SOURCE_COMMAND" "$DEST_COMMAND"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
printf '%s ALL=(root) NOPASSWD: %s service-status, %s service-restart, %s service-journal, %s device-probe, %s safe-command\n'   "$TARGET_USER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" >"$tmp"

if ! command -v visudo >/dev/null 2>&1; then
  echo "visudo is required to validate the bounded sudo policy" >&2
  exit 69
fi
visudo -cf "$tmp" >/dev/null
install -o root -g root -m 0440 "$tmp" "$SUDOERS"
visudo -cf "$SUDOERS" >/dev/null

echo "installed bounded StageCore qualification helper for $TARGET_USER"
