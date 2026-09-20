#!/usr/bin/env bash
# One-time, bounded install for the read-only private GitHub status agent.
set -euo pipefail
umask 077
[[ "$(id -u)" -eq 0 ]] || { echo "Run with sudo on the Pi" >&2; exit 2; }
[[ "$(uname -s)" == Linux && "$(uname -m)" == aarch64 ]] || { echo "Requires the qualified Linux ARM64 Pi" >&2; exit 2; }
HERE="$(cd "$(dirname "$0")" && pwd)"
EXPECTED_HUB_SHA="34681e3ce7595d4caf63f1a306f496f4172f398065a2fe7885bc2bacc6ce7bfe"
PINNED_SHA="809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a"
REPO="ali96adil/StageCore"
ACCOUNT="stagecore-control"
CONFIG_DIR="/etc/stagecore-qualification-control"
STATE_DIR="/var/lib/stagecore-control"
INSTALL_DIR="/opt/stagecore-qualification-control"
SERVICE="stagecore-qualification-control"
TOKEN_PATH="$CONFIG_DIR/token"

[[ -f /opt/stagecore/bin/stagecore-hub && ! -L /opt/stagecore/bin/stagecore-hub ]] || {
  echo "STOP: installed Hub is missing or symlinked" >&2; exit 3;
}
actual="$(sha256sum /opt/stagecore/bin/stagecore-hub | awk '{print $1}')"
[[ "$actual" == "$EXPECTED_HUB_SHA" ]] || {
  echo "STOP: Hub binary differs from pinned candidate $PINNED_SHA" >&2; exit 3;
}
for file in agent.py "$SERVICE.service" "$SERVICE.timer"; do
  [[ -f "$HERE/$file" && ! -L "$HERE/$file" ]] || { echo "STOP: missing input $file" >&2; exit 3; }
done
python3 -m py_compile "$HERE/agent.py"
echo "Verified Hub binary and agent files."

[[ -r /dev/tty ]] || { echo "Use an interactive SSH -tt session for the token prompt" >&2; exit 2; }
printf 'Paste a fine-grained GitHub PAT for %s ONLY (Issues Read/write): ' "$REPO" >/dev/tty
IFS= read -r -s token </dev/tty
printf '\n' >/dev/tty
[[ "$token" =~ ^(github_pat_|ghp_)[A-Za-z0-9_]+$ ]] || {
  echo "STOP: missing or invalid GitHub token format" >&2; exit 2;
}
if ! id -u "$ACCOUNT" >/dev/null 2>&1; then
  useradd --system --home-dir "$STATE_DIR" --shell /usr/sbin/nologin --no-create-home "$ACCOUNT"
fi
install -d -o "$ACCOUNT" -g "$ACCOUNT" -m 0700 "$STATE_DIR"
install -d -o root -g root -m 0755 "$INSTALL_DIR"
install -d -o root -g "$ACCOUNT" -m 0750 "$CONFIG_DIR"
install -o root -g root -m 0644 "$HERE/agent.py" "$INSTALL_DIR/agent.py"

temp="$(mktemp "$CONFIG_DIR/.token.XXXXXXXX")"
trap 'rm -f "$temp"' EXIT
printf '%s\n' "$token" >"$temp"
unset token
chown "$ACCOUNT:$ACCOUNT" "$temp"
chmod 0600 "$temp"
mv -f "$temp" "$TOKEN_PATH"
trap - EXIT

config_temp="$(mktemp "$CONFIG_DIR/.config.XXXXXXXX")"
trap 'rm -f "$config_temp"' EXIT
python3 - "$config_temp" "$REPO" "$PINNED_SHA" "$EXPECTED_HUB_SHA" "$TOKEN_PATH" "$STATE_DIR" <<'PY'
import json,sys
path,repo,sha,digest,token,state=sys.argv[1:]
value=dict(repo=repo,owner_login="ali96adil",pinned_sha=sha,
           expected_hub_sha256=digest,token_file=token,state_dir=state)
with open(path,"w",encoding="utf-8") as file:
    json.dump(value,file,sort_keys=True,indent=2)
    file.write("\n")
PY
chown root:"$ACCOUNT" "$config_temp"
chmod 0640 "$config_temp"
mv -f "$config_temp" "$CONFIG_DIR/config.json"
trap - EXIT

install -o root -g root -m 0644 "$HERE/$SERVICE.service" "/etc/systemd/system/$SERVICE.service"
install -o root -g root -m 0644 "$HERE/$SERVICE.timer" "/etc/systemd/system/$SERVICE.timer"
systemctl daemon-reload
# If the PAT/repo connection fails, do not enable unattended polling.
if ! systemctl start "$SERVICE.service"; then
  echo "STOP: agent connectivity failed; see sudo journalctl -u $SERVICE.service -n 30" >&2
  exit 4
fi
systemctl enable --now "$SERVICE.timer"
echo "STATUS_AGENT_READY: $SERVICE.timer (outbound status only)"
echo "No Hub update, restart, physical output, or public listener was performed."
