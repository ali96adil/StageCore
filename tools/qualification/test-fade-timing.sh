#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/good.json" <<'EOF'
{"status":"COMPLETED","command_id":"cmd-good","lifecycle_ms":2050}
EOF
python3 "$ROOT/tools/qualification/validate-fade-timing.py"   --input "$tmp/good.json" --expected-ms 2000 --tolerance-ms 100 >"$tmp/good-result.json"

cat >"$tmp/bad.json" <<'EOF'
{"status":"COMPLETED","command_id":"cmd-bad","lifecycle_ms":2400}
EOF
set +e
python3 "$ROOT/tools/qualification/validate-fade-timing.py"   --input "$tmp/bad.json" --expected-ms 2000 --tolerance-ms 100 >/dev/null 2>&1
rc=$?
set -e
[[ "$rc" -ne 0 ]]

echo "qualification fade timing self-test PASS"
