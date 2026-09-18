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
  tools/qualification/campaign.sh q16-status
  tools/qualification/campaign.sh q16-ack <disconnect|reconnect> <note>
  tools/qualification/campaign.sh q18-status
  tools/qualification/campaign.sh q18-ack <note>
  tools/qualification/campaign.sh q19-status
  tools/qualification/campaign.sh q19-ack <note>
  tools/qualification/campaign.sh q21-status
  tools/qualification/campaign.sh q21-ack <documented|logic-voltage|de-re|polarity|common|termination> <PASS|FAIL> <note>

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
    [[ "$#" -ge 3 ]] || { usage >&2; exit 64; }
    event="$2"; note="$3"
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
  q16-status)
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    printf 'wifi_loss\tprecondition=%s\tpre=%s\tdisconnect=%s\treconnect=%s\tpost=%s\n'       "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.precondition_set)"       "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.pre)"       "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.disconnect_action)"       "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.reconnect_action)"       "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.post)"
    ;;
  q16-ack)
    [[ "$#" -ge 3 ]] || { usage >&2; exit 64; }
    phase="$2"; note="$3"
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    pre="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.pre)"
    disconnect="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.disconnect_action)"
    reconnect="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.reconnect_action)"
    post="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-16 --key wifi_loss.post)"
    [[ "$pre" == "PASS" ]] || { echo "Wi-Fi-loss baseline is not prepared; run qualification first" >&2; exit 3; }
    [[ "$post" != "PASS" ]] || { echo "Wi-Fi-loss post evidence is already PASS" >&2; exit 3; }
    [[ -n "$note" ]] || { echo "network action note is required" >&2; exit 64; }
    case "$phase" in
      disconnect)
        [[ "$disconnect" != "PASS" ]] || { echo "Wi-Fi disconnect is already acknowledged" >&2; exit 3; }
        [[ "$reconnect" != "PASS" ]] || { echo "Wi-Fi reconnect is already acknowledged" >&2; exit 3; }
        key="wifi_loss.disconnect_action"
        ;;
      reconnect)
        [[ "$disconnect" == "PASS" ]] || { echo "acknowledge Wi-Fi disconnect before reconnect" >&2; exit 3; }
        [[ "$reconnect" != "PASS" ]] || { echo "Wi-Fi reconnect is already acknowledged" >&2; exit 3; }
        key="wifi_loss.reconnect_action"
        ;;
      *)
        echo "phase must be disconnect or reconnect" >&2
        exit 64
        ;;
    esac
    exec python3 "$ROOT/tools/qualification/qualification-milestone.py" record       --state "$STATE" --manifest "$MANIFEST" --gate Q-DMX-16 --key "$key"       --status PASS --actor manual-network-action --evidence manual-network-action --note "$note"
    ;;
  q18-status)
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    printf 'dmx_stability\tprecondition=%s\tpre=%s\tlocal_web=%s\trearm=%s\tstress=%s\n' \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key stress.precondition_set)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key stress.pre)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key local_web.action)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key stress.rearm_set)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key stress.auto)"
    ;;
  q18-ack)
    [[ "$#" -ge 2 ]] || { usage >&2; exit 64; }
    note="$2"
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    pre="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key stress.pre)"
    action="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key local_web.action)"
    stress="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-18 --key stress.auto)"
    [[ "$pre" == "PASS" ]] || { echo "DMX-stability baseline is not prepared; run qualification first" >&2; exit 3; }
    [[ "$action" != "PASS" ]] || { echo "local-web activity is already acknowledged" >&2; exit 3; }
    [[ "$stress" != "PASS" ]] || { echo "DMX-stability stress evidence is already PASS" >&2; exit 3; }
    [[ -n "$note" ]] || { echo "local-web activity note is required" >&2; exit 64; }
    exec python3 "$ROOT/tools/qualification/qualification-milestone.py" record \
      --state "$STATE" --manifest "$MANIFEST" --gate Q-DMX-18 --key local_web.action \
      --status PASS --actor manual-local-web-activity --evidence manual-local-web-activity --note "$note"
    ;;
  q19-status)
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    printf 'local_emergency_blackout\tprecondition=%s\tpre=%s\thub_unavailable=%s\tlocal_blackout=%s\thub_recovery=%s\tpost=%s\n' \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key emergency.precondition_set)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key emergency.pre)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key hub_unavailable.action)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key local_blackout.action)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key hub_recovery.action)" \
      "$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key emergency.post)"
    ;;
  q19-ack)
    [[ "$#" -ge 2 ]] || { usage >&2; exit 64; }
    note="$2"
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    pre="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key emergency.pre)"
    unavailable="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key hub_unavailable.action)"
    blackout="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key local_blackout.action)"
    recovery="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key hub_recovery.action)"
    post="$(python3 "$ROOT/tools/qualification/qualification-milestone.py" get --state "$STATE" --gate Q-DMX-19 --key emergency.post)"
    [[ "$pre" == "PASS" ]] || { echo "local-emergency baseline is not prepared; run qualification first" >&2; exit 3; }
    [[ "$unavailable" == "PASS" ]] || { echo "Hub-unavailable evidence is not PASS" >&2; exit 3; }
    [[ "$blackout" != "PASS" ]] || { echo "local emergency blackout is already acknowledged" >&2; exit 3; }
    [[ "$recovery" != "PASS" ]] || { echo "Hub was already recovered before local blackout acknowledgement" >&2; exit 3; }
    [[ "$post" != "PASS" ]] || { echo "local-emergency post evidence is already PASS" >&2; exit 3; }
    [[ -n "$note" ]] || { echo "local emergency blackout note is required" >&2; exit 64; }
    exec python3 "$ROOT/tools/qualification/qualification-milestone.py" record \
      --state "$STATE" --manifest "$MANIFEST" --gate Q-DMX-19 --key local_blackout.action \
      --status PASS --actor manual-local-emergency --evidence manual-local-emergency --note "$note"
    ;;
  q21-status)
    [[ -f "$STATE" ]] || { echo "qualification campaign state does not exist: $STATE" >&2; exit 66; }
    exec python3 "$ROOT/tools/qualification/electrical-path.py" status --state "$STATE"
    ;;
  q21-ack)
    [[ "$#" -ge 4 ]] || { usage >&2; exit 64; }
    exec python3 "$ROOT/tools/qualification/electrical-path.py" ack \
      --state "$STATE" --manifest "$MANIFEST" --check "$2" --status "$3" --note "$4"
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
