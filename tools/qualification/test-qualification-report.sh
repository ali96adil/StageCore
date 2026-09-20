#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
state="$tmp/state.json"
manifest="$ROOT/tools/qualification/manifest.json"
tool="$ROOT/tools/qualification/qualification-state.py"
report="$ROOT/tools/qualification/qualification-report.py"
python3 "$tool" init --state "$state" --manifest "$manifest" --stagecore-sha test-head >/dev/null
python3 "$report" --state "$state" --manifest "$manifest" --output-dir "$tmp/report" >"$tmp/first"
python3 "$tool" record --state "$state" --manifest "$manifest" --gate Q-TAB-04 --status PASS --evidence probe.json >/dev/null
python3 "$tool" record --state "$state" --manifest "$manifest" --gate Q-DMX-08 --status FAIL --note "fade timeout" --evidence fade.json >/dev/null
python3 "$tool" record --state "$state" --manifest "$manifest" --gate Q-LIVE-04 --status N/A --note "not applicable on test fixture" >/dev/null
python3 "$ROOT/tools/qualification/qualification-milestone.py" record --state "$state" --manifest "$manifest" --gate Q-TAB-06 --key play.command --status BLOCKED --note "no device" >/dev/null
python3 "$report" --state "$state" --manifest "$manifest" --output-dir "$tmp/report" >"$tmp/second"
python3 - "$tmp/report" "$manifest" <<'PY'
import csv, json, pathlib, sys
out, manifest = pathlib.Path(sys.argv[1]), json.load(open(sys.argv[2]))
total = sum(len(g["gates"]) for g in manifest["groups"])
s = json.loads((out / "summary.json").read_text())
assert s["total"] == total
assert s["counts"] == {"PENDING": total - 3, "PASS": 1, "FAIL": 1, "BLOCKED": 0, "N/A": 1}
assert s["completed"] == 2 and s["remaining"] == total - 2
assert s["completion_percent"] == round(200 / total, 1)
assert sum(g["total"] for g in s["groups"]) == total
def read(name):
    with (out / name).open() as f:
        return list(csv.DictReader(f))
assert {r["gate_id"] for r in read("defects.csv")} == {"Q-DMX-08"}
assert {r["gate_id"] for r in read("blocked.csv")} == {"Q-TAB-06"}
assert any(r["gate_id"] == "Q-DMX-08" for r in read("manual.csv"))
assert "Q-DMX-08" in (out / "summary.md").read_text()
assert "password=" not in (out / "summary.md").read_text().lower()
PY
python3 "$tool" record --state "$state" --manifest "$manifest" --gate Q-DMX-08 --status PASS --evidence fixed.json >/dev/null
python3 "$report" --state "$state" --manifest "$manifest" --output-dir "$tmp/report" >"$tmp/third"
python3 - "$tmp/report" <<'PY'
import csv,json,pathlib,sys
out=pathlib.Path(sys.argv[1])
s=json.loads((out/"summary.json").read_text())
assert s["counts"]["FAIL"] == 0 and s["counts"]["PASS"] == 2
assert list(csv.DictReader((out/"defects.csv").open())) == []
PY
STAGECORE_QUALIFICATION_STATE="$state" STAGECORE_QUALIFICATION_REPORT_DIR="$tmp/wrapper-report" \
  "$ROOT/tools/qualification/qualify.sh" status >"$tmp/wrapper-status"
grep -q "^QUALIFICATION " "$tmp/wrapper-status"
bash -n "$ROOT/tools/qualification/qualify.sh"
echo "qualification progress and retry reporting self-test PASS"
