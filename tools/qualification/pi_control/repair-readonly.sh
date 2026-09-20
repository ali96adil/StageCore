#!/usr/bin/env bash
# Repair ONLY GitHub-control read-only report dependency and probe access.
# Never overwrites canonical campaign, raw evidence, Hub or PAT.
set -euo pipefail
umask 077
[[ "$(id -u)" -eq 0 && "$(uname -s)" == Linux && "$(uname -m)" == aarch64 ]] || exit 2
HERE="$(cd "$(dirname "$0")" && pwd)"
DEST=/opt/stagecore-qualification-control
PRIVATE=/var/lib/stagecore-control
HUB_SHA=34681e3ce7595d4caf63f1a306f496f4172f398065a2fe7885bc2bacc6ce7bfe
MANIFEST_SHA=0aaa67702940cf85f97d99a2b45df7e5330238b36643393950eb0b08a1836dad
CONTROL=stagecore-qualification-control
PROBE=stagecore-qualification-probe
[[ "$(sha256sum /opt/stagecore/bin/stagecore-hub | awk '{print $1}')" == "$HUB_SHA" ]] || { echo "STOP: deployed Hub digest differs" >&2; exit 3; }
[[ "$(sha256sum "$DEST/manifest.json" | awk '{print $1}')" == "$MANIFEST_SHA" ]] || { echo "STOP: installed manifest differs" >&2; exit 3; }
[[ -f "$PRIVATE/campaign.json" && ! -L "$PRIVATE/campaign.json" ]] || { echo "STOP: canonical Pi campaign unavailable" >&2; exit 3; }
for file in export_summary.py pi_readonly.py qualification-report.py collect_probe.sh "$PROBE.service"; do
  [[ -f "$HERE/$file" && ! -L "$HERE/$file" ]] || { echo "STOP: missing reviewed repair input $file" >&2; exit 3; }
done
python3 -m py_compile "$HERE/export_summary.py" "$HERE/pi_readonly.py" "$HERE/qualification-report.py"
bash -n "$HERE/collect_probe.sh"
python3 - "$HERE" "$PRIVATE/campaign.json" "$DEST/manifest.json" <<'PY'
import importlib.util,sys
from pathlib import Path
folder=Path(sys.argv[1])
spec=importlib.util.spec_from_file_location("repair_candidate_export",folder/"export_summary.py")
module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
snapshot=module.export_summary(sys.argv[2],sys.argv[3])
if snapshot["candidate_sha"] != "809d1f4ce7a5824c27ecf82d78cd779cd28f6e6a" or snapshot["total"] != 79:
    raise SystemExit("STOP: campaign baseline differs")
print("VERIFIED_CAMPAIGN_COUNTS:",
      "PASS="+str(snapshot["counts"]["PASS"]),
      "FAIL="+str(snapshot["counts"]["FAIL"]),
      "BLOCKED="+str(snapshot["counts"]["BLOCKED"]),
      "PENDING="+str(snapshot["counts"]["PENDING"]))
PY

# Keep the GitHub agent halted until verified Pi output has been inspected.
systemctl stop "$CONTROL.timer" "$CONTROL.service"
systemctl stop "$PROBE.timer" "$PROBE.service"
install -o root -g root -m 0644 "$HERE/export_summary.py" "$DEST/export_summary.py"
install -o root -g root -m 0644 "$HERE/pi_readonly.py" "$DEST/pi_readonly.py"
install -o root -g root -m 0644 "$HERE/qualification-report.py" "$DEST/qualification-report.py"
install -o root -g root -m 0755 "$HERE/collect_probe.sh" "$DEST/collect_probe.sh"
install -o root -g root -m 0644 "$HERE/$PROBE.service" "/etc/systemd/system/$PROBE.service"
systemctl daemon-reload
systemctl start "$PROBE.service"
systemctl start "$PROBE.timer"
runuser -u stagecore-control -- test -r /run/stagecore-qualification-probe/current.json || {
  echo "STOP: snapshot remains unreadable; GitHub agent timer remains stopped" >&2
  exit 4
}
python3 - "$PRIVATE/campaign.json" <<'PY'
import json,sys
state=json.load(open(sys.argv[1],encoding="utf-8"))
for gate_id in ("Q-TAB-04","Q-TAB-05","Q-DMX-20"):
    gate=state["gates"][gate_id]
    done=gate.get("actor")=="pi-readonly-probe"
    print("GATE_STATUS",gate_id,gate["status"],"pi_readonly_recorded="+str(done),
          "history_entries="+str(len(gate.get("history",[]))))
print("CAMPAIGN_PRESERVED: no reset or repin")
PY
echo "READONLY_REPAIR_READY: collector snapshot readable; control timer intentionally stopped."
echo "After reviewing this output, resume the SAME open GitHub request #235. Do not create a new issue."
