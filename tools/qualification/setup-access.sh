#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/stagecore"
ENV_FILE="$CONFIG_DIR/qualification.env"
mkdir -p "$CONFIG_DIR"
chmod 700 "$CONFIG_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  : >"$ENV_FILE"
fi
chmod 600 "$ENV_FILE"

ensure_key() {
  local key="$1" default_value="${2:-}"
  if ! grep -q "^$key=" "$ENV_FILE"; then
    printf '%s=%s\n' "$key" "$default_value" >>"$ENV_FILE"
  fi
}

if ! grep -q '^# Local-only StageCore physical qualification configuration\.$' "$ENV_FILE"; then
  tmp="$(mktemp)"
  {
    echo '# Local-only StageCore physical qualification configuration.'
    echo '# Never commit this file.'
    cat "$ENV_FILE"
  } >"$tmp"
  chmod 600 "$tmp"
  mv "$tmp" "$ENV_FILE"
fi

ensure_key STAGECORE_PI_HOST
ensure_key STAGECORE_PI_USER
ensure_key STAGECORE_PROJECT_ID
ensure_key STAGECORE_RUNTIME_SNAPSHOT_ID
ensure_key STAGECORE_TABLET_DEVICE_ID
ensure_key STAGECORE_TABLET_BUILD_SHA
ensure_key STAGECORE_TABLET_APK_SHA256
ensure_key STAGECORE_LIGHTING_NODE_ID
ensure_key STAGECORE_LIGHTING_FIRMWARE_SHA
ensure_key STAGECORE_HARDWARE_BASELINE_ID
ensure_key STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER
ensure_key STAGECORE_TABLET_QUALIFICATION_OVERLAY_MEDIA_NUMBER
ensure_key STAGECORE_TABLET_QUALIFICATION_LIVE_MEDIA_KEY
ensure_key STAGECORE_TABLET_QUALIFICATION_CUE_NAME
ensure_key STAGECORE_TABLET_QUALIFICATION_MISSING_MEDIA_NUMBER
ensure_key STAGECORE_QUALIFICATION_ENABLE_PHYSICAL_ACTIONS 0
ensure_key STAGECORE_QUALIFICATION_ENABLE_HUB_RESTART 0
ensure_key STAGECORE_QUALIFICATION_ENABLE_HUB_UNAVAILABLE 0
ensure_key STAGECORE_QUALIFICATION_PHYSICAL_HOLD_SECONDS 2
ensure_key STAGECORE_LIGHTING_QUALIFICATION_CHANNEL_KEY
ensure_key STAGECORE_LIGHTING_QUALIFICATION_SET_LEVEL
ensure_key STAGECORE_LIGHTING_QUALIFICATION_FADE_LEVEL
ensure_key STAGECORE_LIGHTING_QUALIFICATION_FADE_MS
ensure_key STAGECORE_LIGHTING_QUALIFICATION_FADE_TOLERANCE_MS
ensure_key STAGECORE_LIGHTING_QUALIFICATION_LONG_FADE_MS
ensure_key STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_FADE_MS
ensure_key STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_REPLACEMENT_LEVEL
ensure_key STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_ACTIVATION_TIMEOUT_MS 5000
ensure_key STAGECORE_LIGHTING_QUALIFICATION_HUB_RESTART_FADE_MS
ensure_key STAGECORE_LIGHTING_QUALIFICATION_HUB_RESTART_ACTIVATION_TIMEOUT_MS 15000
ensure_key STAGECORE_LIGHTING_QUALIFICATION_STABILITY_SECONDS
ensure_key STAGECORE_LIGHTING_QUALIFICATION_STABILITY_INTERVAL_MS 500
ensure_key STAGECORE_LIGHTING_QUALIFICATION_SECOND_CHANNEL_KEY
ensure_key STAGECORE_LIGHTING_QUALIFICATION_SECOND_SET_LEVEL
ensure_key STAGECORE_LIGHTING_QUALIFICATION_SECOND_FADE_LEVEL
ensure_key STAGECORE_LIGHTING_QUALIFICATION_TIMED_BLACKOUT_MS

KEY="$CONFIG_DIR/qualification_ed25519"
if [[ ! -f "$KEY" ]]; then
  ssh-keygen -q -t ed25519 -N '' -f "$KEY" -C stagecore-qualification
fi
chmod 600 "$KEY"
chmod 644 "$KEY.pub"

printf 'Qualification config: %s\n' "$ENV_FILE"
printf 'SSH public key: %s.pub\n' "$KEY"
printf '\nNext: fill Pi access and exact candidate identity pins. Device IDs may stay blank only when exactly one matching device exists. Then run tools/qualification/bootstrap-pi-access.sh once.\n'
