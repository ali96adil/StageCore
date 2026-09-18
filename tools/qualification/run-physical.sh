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
  STAGECORE_QUALIFICATION_ENV       Local qualification config.
  STAGECORE_QUALIFICATION_RUN_ROOT  Evidence root (default qualification/runs).
  STAGECORE_QUALIFICATION_STATE     Durable campaign state path.
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
STATE_ROOT="${XDG_STATE_HOME:-$HOME/.local/state}/stagecore"
STATE_FILE="${STAGECORE_QUALIFICATION_STATE:-$STATE_ROOT/qualification-campaign.json}"
MANIFEST="tools/qualification/manifest.json"
CREDENTIAL_FILE="${STAGECORE_QUALIFICATION_OPERATOR_CREDENTIAL:-$HOME/.config/stagecore/qualification-operator.json}"
CURRENT_STAGECORE_SHA="$(git rev-parse HEAD 2>/dev/null || true)"

mkdir -p "$(dirname "$STATE_FILE")"
python3 tools/qualification/qualification-state.py init   --state "$STATE_FILE"   --manifest "$MANIFEST"   --stagecore-sha "$CURRENT_STAGECORE_SHA"   --tablet-build-sha "${STAGECORE_TABLET_BUILD_SHA:-}"   --tablet-apk-sha256 "${STAGECORE_TABLET_APK_SHA256:-}"   --lighting-firmware-sha "${STAGECORE_LIGHTING_FIRMWARE_SHA:-}"   --hardware-baseline-id "${STAGECORE_HARDWARE_BASELINE_ID:-}" >/dev/null

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

gate_status() {
  python3 tools/qualification/qualification-state.py get --state "$STATE_FILE" --gate "$1"
}

milestone_status() {
  python3 tools/qualification/qualification-milestone.py get --state "$STATE_FILE" --gate "$1" --key "$2"
}

record_gate() {
  local gate="$1" status="$2" evidence="$3" note="$4"
  python3 tools/qualification/qualification-state.py record     --state "$STATE_FILE" --manifest "$MANIFEST" --gate "$gate" --status "$status"     --actor runner --evidence "$evidence" --note "$note" >/dev/null
  record "$gate" "$status" "$note; evidence=$evidence"
}

record_milestone() {
  local gate="$1" key="$2" status="$3" evidence="$4" note="$5"
  python3 tools/qualification/qualification-milestone.py record     --state "$STATE_FILE" --manifest "$MANIFEST" --gate "$gate" --key "$key" --status "$status"     --actor runner --evidence "$evidence" --note "$note" >/dev/null
  record "$gate::$key" "$status" "$note; evidence=$evidence"
}

should_run_gate() {
  local gate="$1"
  if [[ "$MODE" != "resume" ]]; then return 0; fi
  local status
  status="$(gate_status "$gate")"
  [[ "$status" != "PASS" && "$status" != "N/A" ]]
}

should_run_milestone() {
  local gate="$1" key="$2"
  if [[ "$MODE" != "resume" ]]; then return 0; fi
  [[ "$(milestone_status "$gate" "$key")" != "PASS" ]]
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

run_gate_probe() {
  local gate="$1" kind="$2" device_id="$3" check="$4"
  if ! should_run_gate "$gate"; then
    record "$gate" "$(gate_status "$gate")" "resume preserved prior terminal result"
    return
  fi
  local evidence="$RUN_DIR/evidence/$gate.log"
  local args=(--input "$RUN_DIR/evidence/devices.json" --kind "$kind" --check "$check" --max-age-seconds 20)
  [[ -n "${STAGECORE_PROJECT_ID:-}" ]] && args+=(--project-id "$STAGECORE_PROJECT_ID")
  [[ -n "${STAGECORE_RUNTIME_SNAPSHOT_ID:-}" ]] && args+=(--runtime-snapshot-id "$STAGECORE_RUNTIME_SNAPSHOT_ID")
  [[ -n "$device_id" ]] && args+=(--device-id "$device_id")

  set +e
  python3 tools/qualification/assert-device-probe.py "${args[@]}" >"$evidence" 2>&1
  local rc=$?
  set -e
  case "$rc" in
    0) record_gate "$gate" PASS "$evidence" "canonical read-only device evidence passed" ;;
    3) record_gate "$gate" BLOCKED "$evidence" "required target/baseline evidence is not yet available" ;;
    *) record_gate "$gate" FAIL "$evidence" "canonical read-only device evidence failed" ;;
  esac
}

resolve_target() {
  local kind="$1" configured_device="$2"
  local args=(--input "$RUN_DIR/evidence/devices.json" --kind "$kind")
  [[ -n "${STAGECORE_PROJECT_ID:-}" ]] && args+=(--project-id "$STAGECORE_PROJECT_ID")
  [[ -n "$configured_device" ]] && args+=(--device-id "$configured_device")
  python3 tools/qualification/resolve-device-target.py "${args[@]}"
}

validate_evidence() {
  local evidence="$1" command_type="$2" device_id="$3"
  python3 tools/qualification/validate-command-evidence.py     --input "$evidence" --command "$command_type" --device-id "$device_id"     >"$evidence.validation" 2>&1
}

emit_request() {
  local credential_file="$1" project_id="$2" device_id="$3" command_type="$4" payload_json="$5"
  python3 - "$credential_file" "$project_id" "$device_id" "$command_type" "$payload_json" <<'PY'
import json, sys
credential=json.load(open(sys.argv[1], encoding="utf-8"))
print(json.dumps({
    "username": credential.get("username", ""),
    "password": credential.get("password", ""),
    "project_id": sys.argv[2],
    "device_id": sys.argv[3],
    "command_type": sys.argv[4],
    "payload": json.loads(sys.argv[5]),
}, separators=(",", ":")))
PY
}

invoke_command() {
  local helper_op="$1" gate="$2" key="$3" command_type="$4" device_id="$5" project_id="$6" payload_json="$7"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior milestone"
    return
  fi
  local evidence="$RUN_DIR/evidence/$gate.$key.json"
  if [[ ! -f "$CREDENTIAL_FILE" ]]; then
    record_milestone "$gate" "$key" BLOCKED "$evidence" "local Operator credential not configured"
    return
  fi

  set +e
  emit_request "$CREDENTIAL_FILE" "$project_id" "$device_id" "$command_type" "$payload_json" |     ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper "$helper_op"       >"$evidence" 2>"$evidence.stderr"
  local rc=$?
  set -e

  if [[ "$rc" -eq 0 ]] && ! validate_evidence "$evidence" "$command_type" "$device_id"; then
    rc=1
  fi
  case "$rc" in
    0)
      if [[ "$helper_op" == "physical-command" ]]; then
        record_milestone "$gate" "$key" PASS "$evidence" "$command_type completed; physical observation still required"
      else
        record_milestone "$gate" "$key" PASS "$evidence" "$command_type completed with validated canonical result evidence"
      fi
      ;;
    3) record_milestone "$gate" "$key" BLOCKED "$evidence" "$command_type did not reach a terminal result within the bounded wait" ;;
    *) record_milestone "$gate" "$key" FAIL "$evidence" "$command_type command or evidence validation failed" ;;
  esac
}


invoke_envelope_gate() {
  local gate="$1" mode="$2" device_id="$3" project_id="$4" channel="$5" start_level="$6" target_level="$7" fade_ms="$8"
  if ! should_run_gate "$gate"; then
    record "$gate" "$(gate_status "$gate")" "resume preserved prior terminal result"
    return
  fi
  local evidence="$RUN_DIR/evidence/$gate.envelope.json"
  set +e
  python3 - "$mode" "$device_id" "$project_id" "$channel" "$start_level" "$target_level" "$fade_ms" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper envelope-gate >"$evidence" 2>"$evidence.stderr"
import json, sys
print(json.dumps({
    "mode": sys.argv[1],
    "device_id": sys.argv[2],
    "project_id": sys.argv[3],
    "channel_key": sys.argv[4],
    "start_level": float(sys.argv[5]),
    "target_level": float(sys.argv[6]),
    "fade_ms": int(sys.argv[7]),
}, separators=(",", ":")))
PY
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]] && ! python3 tools/qualification/validate-envelope-gate-evidence.py       --input "$evidence" --mode "$mode" >"$evidence.validation" 2>&1; then
    rc=1
  fi
  case "$rc" in
    0) record_gate "$gate" PASS "$evidence" "real-node $mode envelope gate passed through the root-only qualification socket" ;;
    3) record_gate "$gate" BLOCKED "$evidence" "qualification envelope path is not ready" ;;
    *) record_gate "$gate" FAIL "$evidence" "real-node $mode envelope gate failed" ;;
  esac
}

invoke_envelope_milestone() {
  local gate="$1" key="$2" mode="$3" device_id="$4" project_id="$5" channel="$6" start_level="$7" target_level="$8" fade_ms="$9"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior envelope milestone"
    return
  fi
  local evidence="$RUN_DIR/evidence/$gate.$key.json"
  set +e
  python3 - "$mode" "$device_id" "$project_id" "$channel" "$start_level" "$target_level" "$fade_ms" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper envelope-gate >"$evidence" 2>"$evidence.stderr"
import json, sys
print(json.dumps({
    "mode": sys.argv[1],
    "device_id": sys.argv[2],
    "project_id": sys.argv[3],
    "channel_key": sys.argv[4],
    "start_level": float(sys.argv[5]),
    "target_level": float(sys.argv[6]),
    "fade_ms": int(sys.argv[7]),
}, separators=(",", ":")))
PY
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]] && ! python3 tools/qualification/validate-envelope-gate-evidence.py \
      --input "$evidence" --mode "$mode" >"$evidence.validation" 2>&1; then
    rc=1
  fi
  case "$rc" in
    0) record_milestone "$gate" "$key" PASS "$evidence" "real-node $mode envelope milestone passed through the root-only qualification socket" ;;
    3) record_milestone "$gate" "$key" BLOCKED "$evidence" "qualification envelope path is not ready" ;;
    *) record_milestone "$gate" "$key" FAIL "$evidence" "real-node $mode envelope milestone failed" ;;
  esac
}

invoke_supersession() {
  local gate="$1" key="$2" device_id="$3" channel_key="$4" start_level="$5" target_level="$6" replacement_level="$7" fade_ms="$8" activation_timeout_ms="$9"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior supersession sequence"
    return
  fi
  local evidence="$RUN_DIR/evidence/$gate.$key.json"
  if [[ ! -f "$CREDENTIAL_FILE" ]]; then
    record_milestone "$gate" "$key" BLOCKED "$evidence" "local Operator credential not configured"
    return
  fi

  set +e
  python3 - "$CREDENTIAL_FILE" "$device_id" "$channel_key" "$start_level" "$target_level" "$replacement_level" "$fade_ms" "$activation_timeout_ms" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper supersession-command >"$evidence" 2>"$evidence.stderr"
import json, sys
credential=json.load(open(sys.argv[1], encoding="utf-8"))
print(json.dumps({
    "username": credential.get("username", ""),
    "password": credential.get("password", ""),
    "device_id": sys.argv[2],
    "channel_key": sys.argv[3],
    "start_level": float(sys.argv[4]),
    "target_level": float(sys.argv[5]),
    "replacement_level": float(sys.argv[6]),
    "fade_ms": int(sys.argv[7]),
    "activation_timeout_ms": int(sys.argv[8]),
}, separators=(",", ":")))
PY
  local rc=$?
  set -e

  if [[ "$rc" -eq 0 ]] && ! python3 tools/qualification/validate-supersession-evidence.py \
      --input "$evidence" --device-id "$device_id" >"$evidence.validation" 2>&1; then
    rc=1
  fi
  case "$rc" in
    0) record_milestone "$gate" "$key" PASS "$evidence" "active fade was observed, terminalized CANCELLED, and replacement SET became authoritative" ;;
    *) record_milestone "$gate" "$key" FAIL "$evidence" "fade supersession sequence or evidence validation failed" ;;
  esac
}

measure_fade_timing() {
  local gate="$1" key="$2" evidence="$3" expected_ms="$4" tolerance_ms="$5"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior timing measurement"
    return
  fi
  local measurement="$RUN_DIR/evidence/$gate.$key.json"
  set +e
  python3 tools/qualification/validate-fade-timing.py     --input "$evidence" --expected-ms "$expected_ms" --tolerance-ms "$tolerance_ms"     >"$measurement" 2>"$measurement.stderr"
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    record_milestone "$gate" "$key" PASS "$measurement" "real-device fade lifecycle timing is within configured tolerance"
  else
    record_milestone "$gate" "$key" FAIL "$measurement" "real-device fade lifecycle timing is outside configured tolerance or unavailable"
  fi
}

check_cmd "local.git" git rev-parse HEAD || true
check_cmd "local.go" go version || true
check_cmd "local.tests" go test ./... || true
check_cmd "manifest.validation" python3 tools/qualification/validate-manifest.py --summary || true
cp "$MANIFEST" "$RUN_DIR/evidence/qualification-manifest.json"

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
  if ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper device-probe       >"$RUN_DIR/evidence/devices.json" 2>"$RUN_DIR/evidence/device-probe.stderr"; then
    PROBE_AVAILABLE=1
    record "devices.probe" PASS "see evidence/devices.json"
  else
    record "devices.probe" FAIL "see evidence/device-probe.stderr"
  fi
fi

if [[ "$PROBE_AVAILABLE" -eq 1 ]]; then
  run_gate_probe Q-TAB-04 tablet "${STAGECORE_TABLET_DEVICE_ID:-}" readiness
  run_gate_probe Q-TAB-05 tablet "${STAGECORE_TABLET_DEVICE_ID:-}" scope
  run_gate_probe Q-DMX-20 lighting "${STAGECORE_LIGHTING_NODE_ID:-}" observation
else
  for gate in Q-TAB-04 Q-TAB-05 Q-DMX-20; do
    if should_run_gate "$gate"; then
      record_gate "$gate" BLOCKED "$RUN_DIR/evidence/device-probe.stderr" "canonical device probe unavailable"
    else
      record "$gate" "$(gate_status "$gate")" "resume preserved prior terminal result"
    fi
  done
fi

tablet_target=""
lighting_target=""
tablet_target_rc=3
lighting_target_rc=3
if [[ "$PROBE_AVAILABLE" -eq 1 ]]; then
  set +e
  tablet_target="$(resolve_target tablet "${STAGECORE_TABLET_DEVICE_ID:-}")"
  tablet_target_rc=$?
  lighting_target="$(resolve_target lighting "${STAGECORE_LIGHTING_NODE_ID:-}")"
  lighting_target_rc=$?
  set -e
fi

if [[ "$tablet_target_rc" -eq 0 && "$(gate_status Q-TAB-04)" == "PASS" && "$(gate_status Q-TAB-05)" == "PASS" ]]; then
  IFS="$(printf '\t')" read -r tablet_device tablet_project <<<"$tablet_target"
  media="${STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER:-}"
  if [[ "$media" =~ ^[1-9][0-9]*$ ]]; then
    invoke_command safe-command Q-TAB-06 prepare.command TABLET_PREPARE "$tablet_device" "$tablet_project" "{\"media_number\":$media}"
  else
    if should_run_milestone Q-TAB-06 prepare.command; then
      record_milestone Q-TAB-06 prepare.command BLOCKED "$RUN_DIR/evidence/Q-TAB-06.prepare.command.json" "qualification media number is not configured"
    else
      record "Q-TAB-06::prepare.command" PASS "resume preserved prior milestone"
    fi
  fi
elif should_run_milestone Q-TAB-06 prepare.command; then
  record_milestone Q-TAB-06 prepare.command BLOCKED "$RUN_DIR/evidence/Q-TAB-06.prepare.command.json" "Tablet readiness/scope target is not ready for PREPARE"
else
  record "Q-TAB-06::prepare.command" PASS "resume preserved prior milestone"
fi

if [[ "$lighting_target_rc" -eq 0 && "$(gate_status Q-DMX-20)" == "PASS" ]]; then
  IFS="$(printf '\t')" read -r lighting_device lighting_project <<<"$lighting_target"
  invoke_command safe-command Q-DMX-20 state_read.command LIGHTING_STATE_READ "$lighting_device" "$lighting_project" '{}'
  invoke_command safe-command Q-DMX-20 config_read.command LIGHTING_CONFIG_READ "$lighting_device" "$lighting_project" '{}'
else
  for key in state_read.command config_read.command; do
    if should_run_milestone Q-DMX-20 "$key"; then
      record_milestone Q-DMX-20 "$key" BLOCKED "$RUN_DIR/evidence/Q-DMX-20.$key.json" "Lighting target is not ready for bounded read command"
    else
      record "Q-DMX-20::$key" PASS "resume preserved prior milestone"
    fi
  done
fi

if [[ "${STAGECORE_QUALIFICATION_ENABLE_PHYSICAL_ACTIONS:-0}" == "1" ]]; then
  hold="${STAGECORE_QUALIFICATION_PHYSICAL_HOLD_SECONDS:-2}"
  [[ "$hold" =~ ^[0-9]+([.][0-9]+)?$ ]] || { echo "invalid STAGECORE_QUALIFICATION_PHYSICAL_HOLD_SECONDS" >&2; exit 2; }

  if [[ "$tablet_target_rc" -eq 0 && "$(milestone_status Q-TAB-06 prepare.command)" == "PASS" ]]; then
    IFS="$(printf '\t')" read -r tablet_device tablet_project <<<"$tablet_target"
    media="${STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER:-}"
    if [[ "$media" =~ ^[1-9][0-9]*$ ]]; then
      invoke_command physical-command Q-TAB-06 play.command TABLET_PLAY "$tablet_device" "$tablet_project" "{\"media_number\":$media}"
      if [[ "$(milestone_status Q-TAB-06 play.command)" == "PASS" ]]; then
        sleep "$hold"
        invoke_command physical-command Q-TAB-07 pause.command TABLET_PAUSE "$tablet_device" "$tablet_project" '{}'
        sleep "$hold"
        invoke_command physical-command Q-TAB-07 stop.command TABLET_STOP "$tablet_device" "$tablet_project" '{}'
      fi
    fi
  fi

  if [[ "$lighting_target_rc" -eq 0 && "$(gate_status Q-DMX-20)" == "PASS" ]]; then
    IFS="$(printf '\t')" read -r lighting_device lighting_project <<<"$lighting_target"
    channel="${STAGECORE_LIGHTING_QUALIFICATION_CHANNEL_KEY:-}"
    set_level="${STAGECORE_LIGHTING_QUALIFICATION_SET_LEVEL:-}"
    fade_level="${STAGECORE_LIGHTING_QUALIFICATION_FADE_LEVEL:-}"
    fade_ms="${STAGECORE_LIGHTING_QUALIFICATION_FADE_MS:-}"
    second_channel="${STAGECORE_LIGHTING_QUALIFICATION_SECOND_CHANNEL_KEY:-}"
    second_set_level="${STAGECORE_LIGHTING_QUALIFICATION_SECOND_SET_LEVEL:-}"
    second_fade_level="${STAGECORE_LIGHTING_QUALIFICATION_SECOND_FADE_LEVEL:-}"
    timed_blackout_ms="${STAGECORE_LIGHTING_QUALIFICATION_TIMED_BLACKOUT_MS:-}"
    fade_tolerance_ms="${STAGECORE_LIGHTING_QUALIFICATION_FADE_TOLERANCE_MS:-}"
    long_fade_ms="${STAGECORE_LIGHTING_QUALIFICATION_LONG_FADE_MS:-}"
    supersession_fade_ms="${STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_FADE_MS:-}"
    supersession_replacement_level="${STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_REPLACEMENT_LEVEL:-}"
    supersession_activation_timeout_ms="${STAGECORE_LIGHTING_QUALIFICATION_SUPERSESSION_ACTIVATION_TIMEOUT_MS:-5000}"

    single_ok=0
    multi_ok=0
    timed_ok=0
    timing_ok=0
    long_ok=0
    supersession_ok=0
    if [[ -n "$channel" && "$set_level" =~ ^([0-9]|[1-9][0-9]|100)([.][0-9]+)?$ && "$fade_level" =~ ^([0-9]|[1-9][0-9]|100)([.][0-9]+)?$ && "$fade_ms" =~ ^[0-9]+$ ]]; then
      single_ok=1
    fi
    if [[ "$single_ok" -eq 1 && -n "$second_channel" && "$second_channel" != "$channel" && "$second_set_level" =~ ^([0-9]|[1-9][0-9]|100)([.][0-9]+)?$ && "$second_fade_level" =~ ^([0-9]|[1-9][0-9]|100)([.][0-9]+)?$ ]]; then
      multi_ok=1
    fi
    if [[ "$timed_blackout_ms" =~ ^[0-9]+$ && "$timed_blackout_ms" -ge 100 && "$timed_blackout_ms" -le 120000 ]]; then
      timed_ok=1
    fi
    if [[ "$fade_tolerance_ms" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
      timing_ok=1
    fi
    if [[ "$single_ok" -eq 1 && "$long_fade_ms" =~ ^[0-9]+$ && "$long_fade_ms" -gt "$fade_ms" && "$long_fade_ms" -le 120000 ]]; then
      long_ok=1
    fi
    if [[ "$single_ok" -eq 1 && "$supersession_fade_ms" =~ ^[0-9]+$ && "$supersession_fade_ms" -ge 1000 && "$supersession_fade_ms" -le 120000 && "$supersession_replacement_level" =~ ^([0-9]|[1-9][0-9]|100)([.][0-9]+)?$ && "$supersession_activation_timeout_ms" =~ ^[0-9]+$ && "$supersession_activation_timeout_ms" -ge 250 && "$supersession_activation_timeout_ms" -le 10000 ]]; then
      supersession_ok=1
    fi

    if [[ "$single_ok" -eq 1 ]]; then
      invoke_command physical-command Q-DMX-03 set.command LIGHTING_CHANNELS_SET "$lighting_device" "$lighting_project" "{\"channels\":{\"$channel\":$set_level}}"
      sleep "$hold"
      invoke_command physical-command Q-DMX-05 fade.command LIGHTING_CHANNELS_FADE "$lighting_device" "$lighting_project" "{\"channels\":{\"$channel\":$fade_level},\"fade_ms\":$fade_ms}"
      if [[ "$(milestone_status Q-DMX-05 fade.command)" == "PASS" ]]; then
        if [[ "$timing_ok" -eq 1 ]]; then
          measure_fade_timing Q-DMX-07 timing.measurement "$RUN_DIR/evidence/Q-DMX-05.fade.command.json" "$fade_ms" "$fade_tolerance_ms"
        elif should_run_milestone Q-DMX-07 timing.measurement; then
          record_milestone Q-DMX-07 timing.measurement BLOCKED "$RUN_DIR/evidence/Q-DMX-07.timing.measurement.json" "fade timing tolerance is not configured"
        fi
      fi
      python3 - "$fade_ms" "$hold" <<'PY'
import sys, time
time.sleep((int(sys.argv[1]) / 1000.0) + float(sys.argv[2]))
PY
    else
      for spec in "Q-DMX-03 set.command" "Q-DMX-05 fade.command"; do
        set -- $spec
        if should_run_milestone "$1" "$2"; then
          record_milestone "$1" "$2" BLOCKED "$RUN_DIR/evidence/$1.$2.json" "single-channel physical-action values are not configured"
        fi
      done
    fi

    if [[ "$long_ok" -eq 1 ]]; then
      invoke_command physical-command Q-DMX-08 precondition_set.command LIGHTING_CHANNELS_SET "$lighting_device" "$lighting_project" "{\"channels\":{\"$channel\":$set_level}}"
      sleep "$hold"
      if [[ "$(milestone_status Q-DMX-08 precondition_set.command)" == "PASS" ]]; then
        invoke_command physical-command Q-DMX-08 long_fade.command LIGHTING_CHANNELS_FADE "$lighting_device" "$lighting_project" "{\"channels\":{\"$channel\":$fade_level},\"fade_ms\":$long_fade_ms}"
      fi
      if [[ "$(milestone_status Q-DMX-08 long_fade.command)" == "PASS" ]]; then
        python3 - "$long_fade_ms" "$hold" <<'PY'
import sys, time
time.sleep((int(sys.argv[1]) / 1000.0) + float(sys.argv[2]))
PY
      fi
    else
      for key in precondition_set.command long_fade.command; do
        if should_run_milestone Q-DMX-08 "$key"; then
          record_milestone Q-DMX-08 "$key" BLOCKED "$RUN_DIR/evidence/Q-DMX-08.$key.json" "long-fade duration must be configured greater than the normal fade and <=120000 ms"
        fi
      done
    fi

    if [[ "$single_ok" -eq 1 && "$supersession_fade_ms" =~ ^[0-9]+$ && "$supersession_fade_ms" -ge 1000 ]]; then
      invoke_envelope_gate Q-DMX-12 duplicate "$lighting_device" "$lighting_project" "$channel" "$set_level" "$fade_level" "$supersession_fade_ms"
      sleep "$hold"
      invoke_envelope_gate Q-DMX-13 expired "$lighting_device" "$lighting_project" "$channel" "$set_level" "$fade_level" "$supersession_fade_ms"
      sleep "$hold"
    else
      for gate in Q-DMX-12 Q-DMX-13; do
        if should_run_gate "$gate"; then
          record_gate "$gate" BLOCKED "$RUN_DIR/evidence/$gate.envelope.json" "duplicate/expiry test values are not configured"
        fi
      done
    fi

    if [[ "$single_ok" -eq 1 ]]; then
      invoke_envelope_milestone Q-DMX-14 invalid_value.sequence invalid-value "$lighting_device" "$lighting_project" "$channel" "$set_level" "$fade_level" "$fade_ms"
      sleep "$hold"
    elif should_run_milestone Q-DMX-14 invalid_value.sequence; then
      record_milestone Q-DMX-14 invalid_value.sequence BLOCKED "$RUN_DIR/evidence/Q-DMX-14.invalid_value.sequence.json" "invalid-value qualification channel/levels are not configured"
    fi

    if [[ "$supersession_ok" -eq 1 ]]; then
      invoke_supersession Q-DMX-09 supersession.sequence "$lighting_device" "$channel" "$set_level" "$fade_level" "$supersession_replacement_level" "$supersession_fade_ms" "$supersession_activation_timeout_ms"
      sleep "$hold"
    elif should_run_milestone Q-DMX-09 supersession.sequence; then
      record_milestone Q-DMX-09 supersession.sequence BLOCKED "$RUN_DIR/evidence/Q-DMX-09.supersession.sequence.json" "supersession fade/replacement/activation values are not configured"
    fi

    if [[ "$multi_ok" -eq 1 ]]; then
      invoke_command physical-command Q-DMX-04 multi_set.command LIGHTING_CHANNELS_SET "$lighting_device" "$lighting_project" "{\"channels\":{\"$channel\":$set_level,\"$second_channel\":$second_set_level}}"
      sleep "$hold"
      invoke_command physical-command Q-DMX-06 multi_fade.command LIGHTING_CHANNELS_FADE "$lighting_device" "$lighting_project" "{\"channels\":{\"$channel\":$fade_level,\"$second_channel\":$second_fade_level},\"fade_ms\":$fade_ms}"
      python3 - "$fade_ms" "$hold" <<'PY'
import sys, time
time.sleep((int(sys.argv[1]) / 1000.0) + float(sys.argv[2]))
PY
    else
      for spec in "Q-DMX-04 multi_set.command" "Q-DMX-06 multi_fade.command"; do
        set -- $spec
        if should_run_milestone "$1" "$2"; then
          record_milestone "$1" "$2" BLOCKED "$RUN_DIR/evidence/$1.$2.json" "second-channel physical-action values are not configured"
        fi
      done
    fi

    invoke_command physical-command Q-DMX-10 blackout.command LIGHTING_BLACKOUT "$lighting_device" "$lighting_project" '{}'
    sleep "$hold"

    if [[ "$multi_ok" -eq 1 && "$timed_ok" -eq 1 ]]; then
      invoke_command physical-command Q-DMX-11 precondition_set.command LIGHTING_CHANNELS_SET "$lighting_device" "$lighting_project" "{\"channels\":{\"$channel\":$set_level,\"$second_channel\":$second_set_level}}"
      sleep "$hold"
      invoke_command physical-command Q-DMX-11 timed_blackout.command LIGHTING_BLACKOUT "$lighting_device" "$lighting_project" "{\"fade_ms\":$timed_blackout_ms}"
      python3 - "$timed_blackout_ms" "$hold" <<'PY'
import sys, time
time.sleep((int(sys.argv[1]) / 1000.0) + float(sys.argv[2]))
PY
    else
      for key in precondition_set.command timed_blackout.command; do
        if should_run_milestone Q-DMX-11 "$key"; then
          record_milestone Q-DMX-11 "$key" BLOCKED "$RUN_DIR/evidence/Q-DMX-11.$key.json" "timed-blackout precondition or duration is not configured"
        fi
      done
    fi
  fi
fi

python3 tools/qualification/physical-confirmations.py pending --state "$STATE_FILE" >"$RUN_DIR/evidence/pending-physical.txt"
cp "$STATE_FILE" "$RUN_DIR/evidence/campaign-state.json"

{
  echo "# StageCore Physical Qualification Report"
  echo
  printf -- '- Run: `%s`\n' "$STAMP"
  printf -- '- Mode: `%s`\n' "$MODE"
  printf -- '- Non-interactive: `%s`\n' "$NON_INTERACTIVE"
  printf -- '- Repository HEAD: `%s`\n' "$CURRENT_STAGECORE_SHA"
  printf -- '- Campaign state: `%s`\n' "$STATE_FILE"
  echo
  echo "| Check | Result | Evidence |"
  echo "| --- | --- | --- |"
  while IFS="$(printf '\t')" read -r id status detail; do
    printf '| %s | **%s** | %s |\n' "$id" "$status" "$detail"
  done <"$RESULTS"
  echo
  cat "$RUN_DIR/evidence/pending-physical.txt"
} >"$REPORT"

cat "$REPORT"

if grep -q "$(printf '\tFAIL\t')" "$RESULTS"; then exit 1; fi
if grep -q "$(printf '\tBLOCKED\t')" "$RESULTS"; then exit 3; fi
