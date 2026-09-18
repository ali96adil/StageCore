#!/usr/bin/env bash
set -euo pipefail

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
  STAGECORE_QUALIFICATION_ENV  Optional path to local qualification secrets/config.
                              Default: ~/.config/stagecore/qualification.env
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
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="$RUN_ROOT/$STAMP"
mkdir -p "$RUN_DIR/evidence"

RESULTS="$RUN_DIR/results.tsv"
REPORT="$RUN_DIR/report.md"
: >"$RESULTS"

record() {
  local id="$1" status="$2" detail="$3"
  printf '%s\t%s\t%s\n' "$id" "$status" "$detail" >>"$RESULTS"
}

check_local() {
  local id="$1" cmd="$2"
  if bash -lc "$cmd" >"$RUN_DIR/evidence/$id.log" 2>&1; then
    record "$id" PASS "see evidence/$id.log"
  else
    record "$id" FAIL "see evidence/$id.log"
  fi
}

check_local "local.git" "git rev-parse HEAD && git status --short"
check_local "local.go" "go version"
check_local "local.tests" "go test ./..."

if [[ -n "${STAGECORE_PI_HOST:-}" ]]; then
  SSH_TARGET="${STAGECORE_PI_USER:-stagecore}@${STAGECORE_PI_HOST}"
  SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=8)
  check_local "pi.ssh" "ssh ${SSH_OPTS[*]} '$SSH_TARGET' 'printf ready'"
  check_local "pi.hub.service" "ssh ${SSH_OPTS[*]} '$SSH_TARGET' 'sudo -n systemctl is-active stagecore-hub.service'"
  check_local "pi.hub.ready" "ssh ${SSH_OPTS[*]} '$SSH_TARGET' 'curl -fsS http://127.0.0.1:7840/health/ready'"
else
  record "pi.ssh" BLOCKED "STAGECORE_PI_HOST not configured"
  record "pi.hub.service" BLOCKED "STAGECORE_PI_HOST not configured"
  record "pi.hub.ready" BLOCKED "STAGECORE_PI_HOST not configured"
fi

# Device-specific probes intentionally enter only after trusted endpoints are configured.
# They will be expanded in later slices against the canonical Stage Device contracts.
[[ -n "${STAGECORE_TABLET_DEVICE_ID:-}" ]] && record "tablet.target" PASS "configured" || record "tablet.target" BLOCKED "STAGECORE_TABLET_DEVICE_ID not configured"
[[ -n "${STAGECORE_LIGHTING_NODE_ID:-}" ]] && record "lighting.target" PASS "configured" || record "lighting.target" BLOCKED "STAGECORE_LIGHTING_NODE_ID not configured"

{
  echo "# StageCore Physical Qualification Report"
  echo
  echo "- Run: `$STAMP`"
  echo "- Mode: `$MODE`"
  echo "- Repository HEAD: `$(git rev-parse HEAD 2>/dev/null || echo unknown)`"
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
