#!/usr/bin/env bash
# One-time reviewed Pi-native read-only campaign upgrade; never updates the Hub.
set -euo pipefail
umask 077
[[ "$(id -u)" -eq 0 ]] || { echo "Run with sudo on qualified Pi" >&2; exit 2; }
[[ "$(uname -s)" == Linux && "$(uname -m)" == aarch64 ]] || exit 2
HERE="$(cd "$(dirname "$0")" && pwd)"
DEST=/opt/stagecore-qualification-control
PRIVATE=/var/lib/stagecore-control
MANIFEST_SHA=0aaa67702940cf85f97d99a2b45df7e5330238b36643393950eb0b08a1836dad
HUB_SHA=34681e3ce7595d4caf63f1a306f496f4172f398065a2fe7885bc2bacc6ce7bfe
SERVICE=stagecore-qualification-control
PROBE=stagecore-qualification-probe
[[ "$(sha256sum /opt/stagecore/bin/stagecore-hub | awk '{print $1}')" == "$HUB_SHA" ]] || {
  echo "STOP: installed Hub digest mismatch, no update performed" >&2; exit 3;
}
[[ -f /usr/local/libexec/stagecore-qualification-probe && ! -L /usr/local/libexec/stagecore-qualification-probe ]] || {
  echo "STOP: existing canonical read-only device probe is missing" >&2; exit 3;
}
[[ -d "$PRIVATE" && ! -L "$PRIVATE" ]] || { echo "STOP: private control state missing" >&2; exit 3; }
[[ "$(stat -c %a "$PRIVATE")" == 700 ]] || { echo "STOP: private state permissions changed" >&2; exit 3; }
for file in agent.py import_campaign.py verify_campaign.py export_summary.py pi_readonly.py qualification-state.py qualification-milestone.py assert-device-probe.py qualification-report.py manifest.json collect_probe.sh "$PROBE.service" "$PROBE.timer" "$SERVICE.service" qualification-campaign.json; do
  [[ -f "$HERE/$file" && ! -L "$HERE/$file" ]] || { echo "STOP: expected input missing: $file" >&2; exit 3; }
done
[[ "$(sha256sum "$HERE/manifest.json" | awk '{print $1}')" == "$MANIFEST_SHA" ]] || {
  echo "STOP: manifest mismatch" >&2; exit 3;
}
python3 - "$HERE" <<'PY'
import json,sys
from pathlib import Path
sys.path.insert(0,sys.argv[1])
from import_campaign import verify_campaign
root=Path(sys.argv[1])
raw=(root/"manifest.json").read_bytes()
state=json.loads((root/"qualification-campaign.json").read_text(encoding="utf-8"))
manifest=json.loads(raw)
result=verify_campaign(state,manifest,raw)
print("Verified canonical pinned campaign:",result["completed"],"/",result["total"])
PY
for script in agent.py import_campaign.py verify_campaign.py export_summary.py pi_readonly.py qualification-state.py qualification-milestone.py qualification-report.py assert-device-probe.py; do
  python3 -m py_compile "$HERE/$script"
done
bash -n "$HERE/collect_probe.sh"
install -o root -g root -m 0644 "$DEST/agent.py" "$DEST/agent.py.pre-readonly"
stopped=0
rollback() {
  rc=$?
  if [[ "$stopped" == 1 && "$rc" -ne 0 ]]; then
    install -o root -g root -m 0644 "$DEST/agent.py.pre-readonly" "$DEST/agent.py" || true
    systemctl start "$SERVICE.timer" || true
    echo "STOP: previous GitHub status/report agent restored; Hub untouched" >&2
  fi
}
trap rollback EXIT
systemctl stop "$SERVICE.timer" "$SERVICE.service"
stopped=1
for file in agent.py import_campaign.py verify_campaign.py export_summary.py pi_readonly.py qualification-state.py qualification-milestone.py qualification-report.py assert-device-probe.py manifest.json collect_probe.sh; do
  mode=0644
  [[ "$file" == collect_probe.sh ]] && mode=0755
  install -o root -g root -m "$mode" "$HERE/$file" "$DEST/$file"
done
python3 "$DEST/import_campaign.py" \
  --state "$HERE/qualification-campaign.json" \
  --manifest "$DEST/manifest.json" --output "$PRIVATE/campaign.json"
chown stagecore-control:stagecore-control "$PRIVATE/campaign.json"
chmod 0600 "$PRIVATE/campaign.json"
for suffix in service timer; do
  install -o root -g root -m 0644 "$HERE/$PROBE.$suffix" \
    "/etc/systemd/system/$PROBE.$suffix"
done
install -o root -g root -m 0644 "$HERE/$SERVICE.service" \
  "/etc/systemd/system/$SERVICE.service"
systemctl daemon-reload
systemctl start "$PROBE.service"
systemctl enable --now "$PROBE.timer"
systemctl start "$SERVICE.timer"
systemctl start "$SERVICE.service"
stopped=0
trap - EXIT
echo "PI_READONLY_READY: canonical campaign imported, snapshot timer active, GitHub agent active"
echo "Only Q-TAB-04, Q-TAB-05 and Q-DMX-20 observation are in the read-only execution allowlist."
echo "No StageCore Hub restart, update, physical output or manual PASS was executed."
