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
SOURCE_SUPERSESSION="/tmp/stagecore-qualification-supersession"
SOURCE_ENVELOPE_GATES="/tmp/stagecore-qualification-envelope-gates"
SOURCE_HUB_RESTART="/tmp/stagecore-qualification-hub-restart"
SOURCE_DMX_STABILITY="/tmp/stagecore-qualification-dmx-stability"
SOURCE_PUBLISHED_LIGHTING="/tmp/stagecore-qualification-published-lighting"
SOURCE_TABLET_EVIDENCE="/tmp/stagecore-qualification-tablet-evidence"
SOURCE_TABLET_GROUP="/tmp/stagecore-qualification-tablet-group"
SOURCE_DRAFT_EVIDENCE="/tmp/stagecore-qualification-draft-evidence"
SOURCE_DRAFT_HTTP="/tmp/stagecore-qualification-draft-http"
SOURCE_PHASE4_INVENTORY="/tmp/stagecore-qualification-phase4-inventory"
SOURCE_PHASE4_EVIDENCE="/tmp/stagecore-qualification-phase4-evidence"
SOURCE_NETWORK_FAULT="/tmp/stagecore-qualification-network-fault"
SOURCE_NETWORK_COCKPIT="/tmp/stagecore-qualification-network-cockpit"
SOURCE_CALLBOARD_RECONNECT="/tmp/stagecore-qualification-callboard-reconnect"
DEST_HELPER="/usr/local/libexec/stagecore-qualification-helper"
DEST_PROBE="/usr/local/libexec/stagecore-qualification-probe"
DEST_COMMAND="/usr/local/libexec/stagecore-qualification-command"
DEST_SUPERSESSION="/usr/local/libexec/stagecore-qualification-supersession"
DEST_ENVELOPE_GATES="/usr/local/libexec/stagecore-qualification-envelope-gates"
DEST_HUB_RESTART="/usr/local/libexec/stagecore-qualification-hub-restart"
DEST_DMX_STABILITY="/usr/local/libexec/stagecore-qualification-dmx-stability"
DEST_PUBLISHED_LIGHTING="/usr/local/libexec/stagecore-qualification-published-lighting"
DEST_TABLET_EVIDENCE="/usr/local/libexec/stagecore-qualification-tablet-evidence"
DEST_TABLET_GROUP="/usr/local/libexec/stagecore-qualification-tablet-group"
DEST_DRAFT_EVIDENCE="/usr/local/libexec/stagecore-qualification-draft-evidence"
DEST_DRAFT_HTTP="/usr/local/libexec/stagecore-qualification-draft-http"
DEST_PHASE4_INVENTORY="/usr/local/libexec/stagecore-qualification-phase4-inventory"
DEST_PHASE4_EVIDENCE="/usr/local/libexec/stagecore-qualification-phase4-evidence"
DEST_NETWORK_FAULT="/usr/local/libexec/stagecore-qualification-network-fault"
DEST_NETWORK_COCKPIT="/usr/local/libexec/stagecore-qualification-network-cockpit"
DEST_CALLBOARD_RECONNECT="/usr/local/libexec/stagecore-qualification-callboard-reconnect"
SUDOERS="/etc/sudoers.d/stagecore-qualification"
DROPIN_DIR="/etc/systemd/system/stagecore-hub.service.d"
DROPIN="$DROPIN_DIR/qualification-envelope.conf"

[[ -f "$SOURCE_HELPER" ]] || { echo "missing $SOURCE_HELPER" >&2; exit 66; }
[[ -f "$SOURCE_PROBE" ]] || { echo "missing $SOURCE_PROBE" >&2; exit 66; }
[[ -f "$SOURCE_COMMAND" ]] || { echo "missing $SOURCE_COMMAND" >&2; exit 66; }
[[ -f "$SOURCE_SUPERSESSION" ]] || { echo "missing $SOURCE_SUPERSESSION" >&2; exit 66; }
[[ -f "$SOURCE_ENVELOPE_GATES" ]] || { echo "missing $SOURCE_ENVELOPE_GATES" >&2; exit 66; }
[[ -f "$SOURCE_HUB_RESTART" ]] || { echo "missing $SOURCE_HUB_RESTART" >&2; exit 66; }
[[ -f "$SOURCE_DMX_STABILITY" ]] || { echo "missing $SOURCE_DMX_STABILITY" >&2; exit 66; }
[[ -f "$SOURCE_PUBLISHED_LIGHTING" ]] || { echo "missing $SOURCE_PUBLISHED_LIGHTING" >&2; exit 66; }
[[ -f "$SOURCE_TABLET_EVIDENCE" ]] || { echo "missing $SOURCE_TABLET_EVIDENCE" >&2; exit 66; }
[[ -f "$SOURCE_TABLET_GROUP" ]] || { echo "missing $SOURCE_TABLET_GROUP" >&2; exit 66; }
[[ -f "$SOURCE_DRAFT_EVIDENCE" ]] || { echo "missing $SOURCE_DRAFT_EVIDENCE" >&2; exit 66; }
[[ -f "$SOURCE_DRAFT_HTTP" ]] || { echo "missing $SOURCE_DRAFT_HTTP" >&2; exit 66; }
[[ -f "$SOURCE_PHASE4_INVENTORY" ]] || { echo "missing $SOURCE_PHASE4_INVENTORY" >&2; exit 66; }
[[ -f "$SOURCE_PHASE4_EVIDENCE" ]] || { echo "missing $SOURCE_PHASE4_EVIDENCE" >&2; exit 66; }
[[ -f "$SOURCE_NETWORK_FAULT" ]] || { echo "missing $SOURCE_NETWORK_FAULT" >&2; exit 66; }
[[ -f "$SOURCE_NETWORK_COCKPIT" ]] || { echo "missing $SOURCE_NETWORK_COCKPIT" >&2; exit 66; }
[[ -f "$SOURCE_CALLBOARD_RECONNECT" ]] || { echo "missing $SOURCE_CALLBOARD_RECONNECT" >&2; exit 66; }

install -d -o root -g root -m 0755 /usr/local/libexec
install -o root -g root -m 0755 "$SOURCE_HELPER" "$DEST_HELPER"
install -o root -g root -m 0755 "$SOURCE_PROBE" "$DEST_PROBE"
install -o root -g root -m 0755 "$SOURCE_COMMAND" "$DEST_COMMAND"
install -o root -g root -m 0755 "$SOURCE_SUPERSESSION" "$DEST_SUPERSESSION"
install -o root -g root -m 0755 "$SOURCE_ENVELOPE_GATES" "$DEST_ENVELOPE_GATES"
install -o root -g root -m 0755 "$SOURCE_HUB_RESTART" "$DEST_HUB_RESTART"
install -o root -g root -m 0755 "$SOURCE_DMX_STABILITY" "$DEST_DMX_STABILITY"
install -o root -g root -m 0755 "$SOURCE_PUBLISHED_LIGHTING" "$DEST_PUBLISHED_LIGHTING"
install -o root -g root -m 0755 "$SOURCE_TABLET_EVIDENCE" "$DEST_TABLET_EVIDENCE"
install -o root -g root -m 0755 "$SOURCE_TABLET_GROUP" "$DEST_TABLET_GROUP"
install -o root -g root -m 0755 "$SOURCE_DRAFT_EVIDENCE" "$DEST_DRAFT_EVIDENCE"
install -o root -g root -m 0755 "$SOURCE_DRAFT_HTTP" "$DEST_DRAFT_HTTP"
install -o root -g root -m 0755 "$SOURCE_PHASE4_INVENTORY" "$DEST_PHASE4_INVENTORY"
install -o root -g root -m 0755 "$SOURCE_PHASE4_EVIDENCE" "$DEST_PHASE4_EVIDENCE"
install -o root -g root -m 0755 "$SOURCE_NETWORK_FAULT" "$DEST_NETWORK_FAULT"
install -o root -g root -m 0755 "$SOURCE_NETWORK_COCKPIT" "$DEST_NETWORK_COCKPIT"
install -o root -g root -m 0755 "$SOURCE_CALLBOARD_RECONNECT" "$DEST_CALLBOARD_RECONNECT"

install -d -o root -g root -m 0755 "$DROPIN_DIR"
dropin_tmp="$(mktemp)"
printf '%s\n' \
  '[Service]' \
  'Environment=STAGECORE_QUALIFICATION_SOCKET=/run/stagecore-qualification/qualification-envelope.sock' \
  'RuntimeDirectory=stagecore-qualification' \
  'RuntimeDirectoryMode=0700' >"$dropin_tmp"
install -o root -g root -m 0644 "$dropin_tmp" "$DROPIN"
rm -f "$dropin_tmp"
systemctl daemon-reload

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
printf '%s ALL=(root) NOPASSWD: %s service-status, %s service-restart, %s service-stop, %s service-journal, %s device-probe, %s safe-command, %s physical-command, %s tablet-negative-command, %s tablet-evidence, %s tablet-group-play, %s draft-evidence, %s draft-owner-only, %s draft-show-lock, %s phase4-inventory, %s phase4-evidence, %s network-cockpit, %s network-fault, %s callboard-reconnect, %s supersession-command, %s envelope-gate, %s hub-restart-gate, %s dmx-stability-gate, %s published-lighting-config, %s qualification-socket-status\n' \
  "$TARGET_USER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" "$DEST_HELPER" >"$tmp"

command -v visudo >/dev/null 2>&1 || { echo "visudo is required to validate the bounded sudo policy" >&2; exit 69; }
visudo -cf "$tmp" >/dev/null
install -o root -g root -m 0440 "$tmp" "$SUDOERS"
visudo -cf "$SUDOERS" >/dev/null

echo "installed bounded StageCore qualification helper for $TARGET_USER"
