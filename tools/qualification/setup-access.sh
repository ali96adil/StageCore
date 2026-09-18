#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/stagecore"
ENV_FILE="$CONFIG_DIR/qualification.env"
mkdir -p "$CONFIG_DIR"
chmod 700 "$CONFIG_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  cat >"$ENV_FILE" <<'EOF'
# Local-only StageCore physical qualification configuration.
# Never commit this file.
STAGECORE_PI_HOST=
STAGECORE_PI_USER=
STAGECORE_TABLET_DEVICE_ID=
STAGECORE_LIGHTING_NODE_ID=
EOF
  chmod 600 "$ENV_FILE"
fi

KEY="$CONFIG_DIR/qualification_ed25519"
if [[ ! -f "$KEY" ]]; then
  ssh-keygen -q -t ed25519 -N '' -f "$KEY" -C stagecore-qualification
fi
chmod 600 "$KEY"
chmod 644 "$KEY.pub"

printf 'Qualification config: %s\n' "$ENV_FILE"
printf 'SSH public key: %s.pub\n' "$KEY"
printf '\nNext: fill STAGECORE_PI_HOST/STAGECORE_PI_USER, then run tools/qualification/bootstrap-pi-access.sh once.\n'
