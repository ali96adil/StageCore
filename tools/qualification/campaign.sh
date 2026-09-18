#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST="$ROOT/tools/qualification/manifest.json"
STATE_ROOT="${XDG_STATE_HOME:-$HOME/.local/state}/stagecore"
STATE="${STAGECORE_QUALIFICATION_STATE:-$STATE_ROOT/qualification-campaign.json}"

usage() {
  cat <<'EOF'
Usage:
  tools/qualification/campaign.sh status [--all]
  tools/qualification/campaign.sh manual <GATE_ID> <PASS|FAIL|BLOCKED|N/A> <evidence-or-note>
  tools/qualification/campaign.sh repin <pin> <new-value> <reason> <GATE_ID> [GATE_ID...]
  tools/qualification/campaign.sh pending-physical
  tools/qualification/campaign.sh confirm-pending <PASS|FAIL> <note>
  tools/qualification/campaign.sh confirm-one <GATE_ID> <PASS|FAIL> <note>
  tools/qualification/campaign.sh q15-status
  tools/qualification/campaign.sh q15-ack <power-cycle|brownout> <note>

Examples:
  tools/qualification/campaign.sh status
  tools/qualification/campaign.sh manual Q-TAB-06 PASS "operator observed expected video on tablet"
  tools/qualification/campaign.sh repin lighting_firmware_sha abc123 "firmware fix" Q-DMX-08 Q-DMX-09 Q-DMX-22
EOF
}

cmd="${1:-}"
case "$cmd" in
  status)
    shift
    exec python3 "$ROOT/tools/qualification/qualification-state.py" status --state "$STATE" "$@"
    ;;
  manual)
    [[ "$#" -ge 4 ]] || { usage >&2; exit 64; }
    gate="$2"; result="$3"; note="$4"
    exec python3 "$ROOT/tools/qualification/qualification-state.py" record \
      --state "$STATE" --manifest "$MANIFEST" --gate "$gate" --status "$result" \
      --actor manual --evidence "manual-observation" --note "$note"
    ;;
  pending-physical)
    exec python3 "$ROOT/tools/qualification/physical-confirmations.py" pending --state "$STATE"
    ;;
  confirm-pending)
    [[ "$#" -ge 3 ]] || { usage >&2; exit 64; }
    exec python3 "$ROOT/tools/qualification/physical-confirmations.py" confirm-all --state "$STATE" --status "$2" --note "$3"
    ;;
  confirm-one)
    [[ "$#" -ge 4 ]] || { usage >&2; exit 64; }
    exec python3 "$ROOT/tools/qualification/physical-confirmations.py" confirm-one --state "$STATE" --gate "$2" --status "$3" --note "$4"
    ;;
  q15-status)
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    for event in power_cycle brownout; do
      printf '%s\tpre=%s\taction=%s\tpost=%s\n'         "$event"         "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-15 --key "$event.pre")"         "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-15 --key "$event.action")"         "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-15 --key "$event.post")"
    done
    ;;
  q15-ack)
    [[ "$#" -ge 4 ]] || { usage >&2; exit 64; }
    event="$2"; note="$4"
    case "$event" in
      power-cycle) key="power_cycle" ;;
      brownout) key="brownout" ;;
      *) echo "event must be power-cycle or brownout" >&2; exit 64 ;;
    esac
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    pre="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-15 --key "$key.pre")"
    action="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-15 --key "$key.action")"
    post="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-15 --key "$key.post")"
    [[ "$pre" == "PASS" ]] || { echo "$event baseline is not prepared; run qualification first" >&2; exit 3; }
    [[ "$action" != "PASS" ]] || { echo "$event hardware action is already acknowledged" >&2; exit 3; }
    [[ "$post" != "PASS" ]] || { echo "$event post evidence is already PASS" >&2; exit 3; }
    if [[ "$event" == "brownout" ]]; then
      power_post="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-15 --key power_cycle.post)"
      [[ "$power_post" == "PASS" ]] || { echo "power-cycle post evidence must PASS before brownout" >&2; exit 3; }
    fi
    [[ -n "$note" ]] || { echo "hardware action note is required" >&2; exit 64; }
    exec python3 "$ROOT/tools/qualification/qualification-milestone.py" record       --state "$STATE" --manifest "$MANIFEST" --gate Q-DMX-15 --key "$key.action"       --status PASS --actor manual-hardware-action --evidence manual-hardware-action --note "$note"
    ;;
  repin)
    [[ "$#" -ge 5 ]] || { usage >&2; exit 64; }
    pin="$2"; value="$3"; reason="$4"; shift 4
    args=()
    for gate in "$@"; do args+=(--invalidate "$gate"); done
    exec python3 "$ROOT/tools/qualification/qualification-state.py" repin \
      --state "$STATE" --manifest "$MANIFEST" --pin "$pin" --value "$value" \
      --reason "$reason" "${args[@]}"
    ;;
  *)
    usage >&2
    exit 64
    ;;
esac
