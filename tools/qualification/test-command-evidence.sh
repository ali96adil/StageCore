#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/good.json" <<'EOF'
{"command_id":"cmd-1","command_type":"TABLET_PLAY","device_id":"tablet-1","qualification_command":"TABLET_PLAY","status":"COMPLETED","result":{"status":"COMPLETED","token":"[REDACTED]"}}
EOF
python3 "$ROOT/tools/qualification/validate-command-evidence.py" --input "$tmp/good.json" --command TABLET_PLAY --device-id tablet-1 >/dev/null

cat >"$tmp/bad.json" <<'EOF'
{"command_id":"cmd-1","command_type":"TABLET_PLAY","device_id":"tablet-1","qualification_command":"TABLET_PLAY","status":"COMPLETED","result":{"session_token":"leak"}}
EOF
set +e
python3 "$ROOT/tools/qualification/validate-command-evidence.py" --input "$tmp/bad.json" --command TABLET_PLAY --device-id tablet-1 >/dev/null 2>&1
rc=$?
set -e
[[ "$rc" -ne 0 ]]
echo "qualification command evidence self-test PASS"
