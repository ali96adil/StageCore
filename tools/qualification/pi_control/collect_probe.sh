#!/usr/bin/env bash
# Root-owned and timer-driven; the unprivileged GitHub agent cannot invoke sudo.
set -euo pipefail
umask 077
PATH=/usr/sbin:/usr/bin:/sbin:/bin
OUT=/run/stagecore-qualification-probe
PROBE=/usr/local/libexec/stagecore-qualification-probe
[[ "$(id -u)" == 0 && -f "$PROBE" && ! -L "$PROBE" ]] || exit 3
# A read-only timer must never activate an intentionally stopped production Hub.
systemctl is-active --quiet stagecore-hub.service || exit 3
install -d -o root -g stagecore-control -m 0750 "$OUT"
TEMP="$(mktemp "$OUT/.probe.XXXXXXXX")"
trap 'rm -f "$TEMP"' EXIT
timeout 15 "$PROBE" >"$TEMP"
python3 - "$TEMP" <<'PY'
import json,sys
from pathlib import Path
path=Path(sys.argv[1])
if path.stat().st_size > 1048576:
    raise SystemExit("probe exceeded allowed size")
data=json.loads(path.read_text(encoding="utf-8"))
if data.get("schema_version") != 1 or not isinstance(data.get("devices"),list):
    raise SystemExit("invalid canonical probe result")
PY
chown root:stagecore-control "$TEMP"
chmod 0640 "$TEMP"
mv -f "$TEMP" "$OUT/current.json"
trap - EXIT
