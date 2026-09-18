#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

summary="$(python3 tools/qualification/validate-manifest.py --summary)"
python3 - "$summary" <<'PY'
import json, sys
s=json.loads(sys.argv[1])
assert s["schema_version"] == 1
assert s["groups"] >= 7
assert s["gates"] >= 70
for issue in ("138","148","195","221"):
    assert s["source_issues"].get(issue, 0) > 0
assert s["methods"]["AUTO"] > 0
assert s["methods"]["AUTO_PHYSICAL"] > 0
assert s["methods"]["MANUAL"] > 0
assert s["methods"]["EVIDENCE"] > 0
PY

echo "qualification manifest self-test PASS"
