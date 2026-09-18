#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/stagecore"
ENV_FILE="${STAGECORE_QUALIFICATION_ENV:-$CONFIG_DIR/qualification.env}"
KEY="${STAGECORE_QUALIFICATION_SSH_KEY:-$CONFIG_DIR/qualification_ed25519}"

"$SCRIPT_DIR/setup-access.sh"

# shellcheck disable=SC1090
source "$ENV_FILE"

: "${STAGECORE_PI_HOST:?set STAGECORE_PI_HOST in $ENV_FILE}"
: "${STAGECORE_PI_USER:?set STAGECORE_PI_USER in $ENV_FILE}"

TARGET="$STAGECORE_PI_USER@$STAGECORE_PI_HOST"
PUBKEY="$(cat "$KEY.pub")"

echo "Step 1/3: authorize the dedicated qualification SSH key (one SSH password prompt may occur)."
printf '%s\n' "$PUBKEY" | ssh "$TARGET" '
  set -eu
  umask 077
  mkdir -p "$HOME/.ssh"
  touch "$HOME/.ssh/authorized_keys"
  IFS= read -r key
  grep -qxF "$key" "$HOME/.ssh/authorized_keys" || printf "%s\n" "$key" >>"$HOME/.ssh/authorized_keys"
'

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=8 -o IdentitiesOnly=yes -i "$KEY")

echo "Step 2/3: install the root-owned bounded helper (one sudo password prompt may occur)."
scp "${SSH_OPTS[@]}" "$SCRIPT_DIR/pi/stagecore-qualification-helper" "$TARGET:/tmp/stagecore-qualification-helper"
scp "${SSH_OPTS[@]}" "$SCRIPT_DIR/pi/install-stagecore-qualification-helper.sh" "$TARGET:/tmp/install-stagecore-qualification-helper.sh"
ssh -t -i "$KEY" -o IdentitiesOnly=yes "$TARGET" \
  "chmod 700 /tmp/install-stagecore-qualification-helper.sh && sudo /tmp/install-stagecore-qualification-helper.sh '$STAGECORE_PI_USER'"

echo "Step 3/3: verify future qualification access is non-interactive."
ssh "${SSH_OPTS[@]}" "$TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper service-status

echo "qualification access ready"
