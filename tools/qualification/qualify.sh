#!/usr/bin/env bash
# One entrypoint; keep the original bounded runner and campaign state authoritative.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
STATE="${STAGECORE_QUALIFICATION_STATE:-${XDG_STATE_HOME:-$HOME/.local/state}/stagecore/qualification-campaign.json}"
REPORT_DIR="${STAGECORE_QUALIFICATION_REPORT_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/stagecore/qualification-reports}"
usage() {
  cat <<'HELP'
Usage: tools/qualification/qualify.sh <run|retry|status|issues|manual> [manual args...]
  run     First campaign: full; existing campaign: resume without repeating PASS/N/A.
  retry   Resume FAILED/BLOCKED/PENDING work; preserve previous PASS and evidence.
  status  Rebuild percentage dashboard without touching Pi or devices.
  issues  Show locally recorded FAIL and BLOCKED entries.
  manual  Pass an explicit recorded observation to campaign.sh manual.
One-time access: setup-access.sh then bootstrap-pi-access.sh (controlled Hub restart).
Real device output actions require separate, explicit qualification config arming.
HELP
}
report() {
  if [[ ! -f "$STATE" ]]; then
    echo "No campaign state at $STATE; run qualification first." >&2
    return 4
  fi
  python3 tools/qualification/qualification-report.py \
    --manifest tools/qualification/manifest.json --state "$STATE" \
    --output-dir "$REPORT_DIR"
}
case "${1:-}" in
  run|retry)
    operation="$1"
    mode="--full"
    [[ -f "$STATE" ]] && mode="--resume"
    if [[ "$operation" == "retry" && ! -f "$STATE" ]]; then
      echo "No saved campaign to retry. Use: tools/qualification/qualify.sh run" >&2
      exit 4
    fi
    set +e
    tools/qualification/run-physical.sh "$mode" --non-interactive
    rc=$?
    set -e
    if ! report; then
      echo "Qualification report generation failed; inspect the preserved state/evidence." >&2
      [[ "$rc" -ne 0 ]] && exit "$rc"
      exit 4
    fi
    exit "$rc"
    ;;
  status)
    report
    ;;
  issues)
    report
    echo "===== FAILED ====="
    cat "$REPORT_DIR/defects.csv"
    echo "===== BLOCKED ====="
    cat "$REPORT_DIR/blocked.csv"
    ;;
  manual)
    shift
    tools/qualification/campaign.sh manual "$@"
    report
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
