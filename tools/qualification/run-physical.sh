#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

MODE="full"
NON_INTERACTIVE=0
for arg in "$@"; do
  case "$arg" in
    --full) MODE="full" ;;
    --resume) MODE="resume" ;;
    --non-interactive) NON_INTERACTIVE=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: tools/qualification/run-physical.sh [--full|--resume] [--non-interactive]

Environment:
  STAGECORE_QUALIFICATION_ENV       Optional local qualification config.
                                    Default: ~/.config/stagecore/qualification.env
  STAGECORE_QUALIFICATION_RUN_ROOT  Optional evidence root.
                                    Default: qualification/runs
EOF
      exit 0
      ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

ENV_FILE="${STAGECORE_QUALIFICATION_ENV:-$HOME/.config/stagecore/qualification.env}"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi

RUN_ROOT="${STAGECORE_QUALIFICATION_RUN_ROOT:-qualification/runs}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)-$$"
RUN_DIR="$RUN_ROOT/$STAMP"
mkdir -p "$RUN_DIR/evidence"

RESULTS="$RUN_DIR/results.tsv"
REPORT="$RUN_DIR/report.md"
: >"$RESULTS"

record() {
  local id="$1" status="$2" detail="$3"
  printf '%s\t%s\t%s\n' "$id" "$status" "$detail" >>"$RESULTS"
}

check_cmd() {
  local id="$1"
  shift
  if "$@" >"$RUN_DIR/evidence/$id.log" 2>&1; then
    record "$id" PASS "see evidence/$id.log"
    return 0
  fi
  record "$id" FAIL "see evidence/$id.log"
  return 1
}

check_probe() {
  local id="$1" kind="$2" device_id="$3"
  local args=(--input "$RUN_DIR/evidence/devices.json" --kind "$kind" --max-age-seconds 20)
  if [[ -n "${STAGECORE_PROJECT_ID:-}" ]]; then
    args+=(--project-id "$STAGECORE_PROJECT_ID")
  fi
  if [[ -n "$device_id" ]]; then
    args+=(--device-id "$device_id")
  fi
  set +e
  python3 tools/qualification/assert-device-probe.py "${args[@]}" >"$RUN_DIR/evidence/$id.log" 2>&1
  local rc=$?
  set -e
  case "$rc" in
    0) record "$id" PASS "see evidence/$id.log" ;;
    3) record "$id" BLOCKED "see evidence/$id.log" ;;
    *) record "$id" FAIL "see evidence/$id.log" ;;
  esac
}

check_cmd "local.git" git rev-parse HEAD || true
check_cmd "local.go" go version || true
check_cmd "local.tests" go test ./... || true

PI_HOST="${STAGECORE_PI_HOST:-}"
PI_USER="${STAGECORE_PI_USER:-}"
SSH_KEY="${STAGECORE_QUALIFICATION_SSH_KEY:-$HOME/.config/stagecore/qualification_ed25519}"
PROBE_AVAILABLE=0

if [[ -z "$PI_HOST" || -z "$PI_USER" ]]; then
  record "pi.ssh" BLOCKED "STAGECORE_PI_HOST/STAGECORE_PI_USER not configured"
  record "pi.hub.service" BLOCKED "Pi access not configured"
  record "pi.hub.ready" BLOCKED "Pi access not configured"
elif [[ ! -f "$SSH_KEY" ]]; then
  record "pi.ssh" BLOCKED "qualification SSH key missing: $SSH_KEY"
  record "pi.hub.service" BLOCKED "qualification SSH key missing"
  record "pi.hub.ready" BLOCKED "qualification SSH key missing"
else
  SSH_TARGET="$PI_USER@$PI_HOST"
  SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=8 -o IdentitiesOnly=yes -i "$SSH_KEY")
  check_cmd "pi.ssh" ssh "${SSH_OPTS[@]}" "$SSH_TARGET" printf ready || true
  check_cmd "pi.hub.service" ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper service-status || true
  check_cmd "pi.hub.ready" ssh "${SSH_OPTS[@]}" "$SSH_TARGET" curl -fsS http://127.0.0.1:7840/health/ready || true
  if ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper device-probe >"$RUN_DIR/evidence/devices.json" 2>"$RUN_DIR/evidence/device-probe.stderr"; then
    PROBE_AVAILABLE=1
    record "devices.probe" PASS "see evidence/devices.json"
  else
    record "devices.probe" FAIL "see evidence/device-probe.stderr"
  fi
fi

if [[ "$PROBE_AVAILABLE" -eq 1 ]]; then
  check_probe "tablet.readiness" tablet "${STAGECORE_TABLET_DEVICE_ID:-}"
  check_probe "lighting.readiness" lighting "${STAGECORE_LIGHTING_NODE_ID:-}"
else
  record "tablet.readiness" BLOCKED "canonical device probe unavailable"
  record "lighting.readiness" BLOCKED "canonical device probe unavailable"
fi

{
  echo "# StageCore Physical Qualification Report"
  echo
  printf -- '- Run: `%s`\n' "$STAMP"
  printf -- '- Mode: `%s`\n' "$MODE"
  printf -- '- Non-interactive: `%s`\n' "$NON_INTERACTIVE"
  printf -- '- Repository HEAD: `%s`\n' "$(git rev-parse HEAD 2>/dev/null || echo unknown)"
  echo
  echo "| Check | Result | Evidence |"
  echo "| --- | --- | --- |"
  while IFS=$'\t' read -r id status detail; do
    printf '| %s | **%s** | %s |\n' "$id" "$status" "$detail"
  done <"$RESULTS"
} >"$REPORT"

cat "$REPORT"

if grep -q $'\tFAIL\t' "$RESULTS"; then
  exit 1
fi
if grep -q $'\tBLOCKED\t' "$RESULTS"; then
  exit 3
fi
