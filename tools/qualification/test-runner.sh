#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNNER="$REPO_ROOT/tools/qualification/run-physical.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/bin"
cat >"$tmp/bin/go" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  version) echo "go version go-test qualification-self-test" ;;
  test) exit 0 ;;
  *) exit 0 ;;
esac
EOF
chmod +x "$tmp/bin/go"

cat >"$tmp/qualification.env" <<'EOF'
STAGECORE_PI_HOST=
STAGECORE_PI_USER=
STAGECORE_PROJECT_ID=
STAGECORE_TABLET_DEVICE_ID=
STAGECORE_LIGHTING_NODE_ID=
QUALIFICATION_SENTINEL_SECRET=do-not-print-this-value
EOF
chmod 600 "$tmp/qualification.env"

set +e
PATH="$tmp/bin:$PATH" \
STAGECORE_QUALIFICATION_ENV="$tmp/qualification.env" \
STAGECORE_QUALIFICATION_RUN_ROOT="$tmp/runs" \
"$RUNNER" --full --non-interactive >"$tmp/stdout" 2>"$tmp/stderr"
rc=$?
set -e

[[ "$rc" -eq 3 ]] || { echo "expected BLOCKED exit 3, got $rc" >&2; exit 1; }

report="$(find "$tmp/runs" -name report.md -type f -print -quit)"
results="$(find "$tmp/runs" -name results.tsv -type f -print -quit)"
[[ -n "$report" && -n "$results" ]]

grep -F '| local.git | **PASS** |' "$report" >/dev/null
grep -F '| local.go | **PASS** |' "$report" >/dev/null
grep -F '| local.tests | **PASS** |' "$report" >/dev/null
grep -F '| pi.ssh | **BLOCKED** |' "$report" >/dev/null
grep -F '| tablet.readiness | **BLOCKED** |' "$report" >/dev/null
grep -F '| lighting.readiness | **BLOCKED** |' "$report" >/dev/null

if grep -R -F 'do-not-print-this-value' "$tmp/runs" "$tmp/stdout" "$tmp/stderr" >/dev/null 2>&1; then
  echo "qualification secret leaked into evidence/report output" >&2
  exit 1
fi

"$RUNNER" --help >/dev/null
echo "qualification runner self-test PASS"
