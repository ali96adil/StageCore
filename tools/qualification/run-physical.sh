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
Q15_DIR="$(dirname "$STATE_FILE")/q-dmx-15"
Q16_DIR="$(dirname "$STATE_FILE")/q-dmx-16"
Q19_DIR="$(dirname "$STATE_FILE")/q-dmx-19"
Q14_DIR="$(dirname "$STATE_FILE")/q-tab-14"
mkdir -p "$Q15_DIR" "$Q16_DIR" "$Q19_DIR" "$Q14_DIR"

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

milestone_evidence() {
  python3 - "$STATE_FILE" "$1" "$2" <<'PY'
import json, sys
state=json.load(open(sys.argv[1], encoding="utf-8"))
item=((state.get("gates",{}).get(sys.argv[2],{}).get("milestones",{}).get(sys.argv[3])) or {})
evidence=item.get("evidence") or []
print(evidence[0] if evidence else "")
PY
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

run_gate_probe_milestone() {
  local gate="$1" key="$2" kind="$3" device_id="$4" check="$5"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior canonical device observation"
    return
  fi
  local evidence="$RUN_DIR/evidence/$gate.$key.log"
  local args=(--input "$RUN_DIR/evidence/devices.json" --kind "$kind" --check "$check" --max-age-seconds 20)
  [[ -n "${STAGECORE_PROJECT_ID:-}" ]] && args+=(--project-id "$STAGECORE_PROJECT_ID")
  [[ -n "${STAGECORE_RUNTIME_SNAPSHOT_ID:-}" ]] && args+=(--runtime-snapshot-id "$STAGECORE_RUNTIME_SNAPSHOT_ID")
  [[ -n "$device_id" ]] && args+=(--device-id "$device_id")

  set +e
  python3 tools/qualification/assert-device-probe.py "${args[@]}" >"$evidence" 2>&1
  local rc=$?
  set -e
  case "$rc" in
    0) record_milestone "$gate" "$key" PASS "$evidence" "canonical read-only device observation/readiness passed" ;;
    3) record_milestone "$gate" "$key" BLOCKED "$evidence" "required target/baseline observation is not yet available" ;;
    *) record_milestone "$gate" "$key" FAIL "$evidence" "canonical device observation/readiness failed" ;;
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


milestone_updated_at() {
  python3 - "$STATE_FILE" "$1" "$2" <<'PY'
import json, sys
state=json.load(open(sys.argv[1], encoding="utf-8"))
item=((state.get("gates",{}).get(sys.argv[2],{}).get("milestones",{}).get(sys.argv[3])) or {})
print(item.get("updated_at",""))
PY
}

invoke_tablet_evidence() {
  local gate="$1" key="$2" mode="$3" device_id="$4" project_id="$5" extra_json="$6"
  local evidence="${7:-$RUN_DIR/evidence/$gate.$key.json}"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior Tablet evidence"
    return 0
  fi
  set +e
  python3 - "$mode" "$device_id" "$project_id" "$extra_json" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper tablet-evidence \
      >"$evidence" 2>"$evidence.stderr"
import json, sys
mode,device,project,extra=sys.argv[1:]
value={"mode":mode,"device_id":device,"project_id":project}
if extra:
    value.update(json.loads(extra))
print(json.dumps(value,separators=(",",":")))
PY
  local rc=$?
  set -e
  case "$rc" in
    0) record_milestone "$gate" "$key" PASS "$evidence" "$mode evidence validated against the live StageCore database" ;;
    3) record_milestone "$gate" "$key" BLOCKED "$evidence" "$mode evidence is not ready yet" ;;
    *) record_milestone "$gate" "$key" FAIL "$evidence" "$mode evidence validation failed" ;;
  esac
  return "$rc"
}

invoke_missing_media() {
  local device_id="$1" project_id="$2" media_number="$3"
  local gate="Q-TAB-13" key="missing_media.command"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior MEDIA_NOT_FOUND evidence"
    return 0
  fi
  local evidence="$RUN_DIR/evidence/$gate.$key.json"
  if [[ ! -f "$CREDENTIAL_FILE" ]]; then
    record_milestone "$gate" "$key" BLOCKED "$evidence" "local Operator credential not configured"
    return 3
  fi
  set +e
  emit_request "$CREDENTIAL_FILE" "$project_id" "$device_id" TABLET_PREPARE "{\"media_number\":$media_number}" | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper tablet-negative-command \
      >"$evidence" 2>"$evidence.stderr"
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    set +e
    python3 - "$evidence" <<'PY'
import json,sys
data=json.load(open(sys.argv[1],encoding="utf-8"))
result=data.get("result") or {}
error=result.get("error") or {}
assert data.get("qualification_command")=="TABLET_PREPARE"
assert data.get("status") in {"FAILED","REJECTED"}
assert error.get("error_code")=="MEDIA_NOT_FOUND"
PY
    local evidence_rc=$?
    set -e
    [[ "$evidence_rc" -eq 0 ]] || rc=1
  fi
  case "$rc" in
    0) record_milestone "$gate" "$key" PASS "$evidence" "missing media produced terminal MEDIA_NOT_FOUND evidence; Operator visibility still requires physical confirmation" ;;
    3) record_milestone "$gate" "$key" BLOCKED "$evidence" "missing-media command did not terminalize within bounded wait" ;;
    *) record_milestone "$gate" "$key" FAIL "$evidence" "missing-media qualification did not produce MEDIA_NOT_FOUND" ;;
  esac
  return "$rc"
}

invoke_tablet_scope_gate() {
  local device_id="$1" project_id="$2" snapshot_id="$3" manifest_id="$4" media_number="$5"
  local gate="Q-TAB-15" key="scope_mismatch.sequence"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior project/snapshot mismatch rejection evidence"
    return 0
  fi
  local evidence="$RUN_DIR/evidence/$gate.$key.json"
  set +e
  python3 - "$device_id" "$project_id" "$snapshot_id" "$manifest_id" "$media_number" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper envelope-gate \
      >"$evidence" 2>"$evidence.stderr"
import json,sys
print(json.dumps({
 "mode":"tablet-scope","device_id":sys.argv[1],"project_id":sys.argv[2],
 "runtime_snapshot_id":sys.argv[3],"tablet_manifest_id":sys.argv[4],
 "media_number":int(sys.argv[5]),
},separators=(",",":")))
PY
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    record_milestone "$gate" "$key" PASS "$evidence" "real Tablet rejected mismatched project and Runtime Snapshot before PREPARE execution"
    record_gate "$gate" PASS "$evidence" "project and Runtime Snapshot mismatch commands were rejected by the real Tablet scope boundary"
  elif [[ "$rc" -eq 3 ]]; then
    record_milestone "$gate" "$key" BLOCKED "$evidence" "qualification socket/Tablet scope target unavailable"
    record_gate "$gate" BLOCKED "$evidence" "scope mismatch test target unavailable"
  else
    record_milestone "$gate" "$key" FAIL "$evidence" "Tablet project/snapshot mismatch rejection failed"
    record_gate "$gate" FAIL "$evidence" "scope mismatch command was not rejected as required"
  fi
  return "$rc"
}

stage_q14_reconnect() {
  local device_id="$1" project_id="$2" snapshot_id="$3"
  local pre_key="reconnect.pre" disconnect_key="disconnect.action" reconnect_key="reconnect.action" post_key="reconnect.post"
  local pre="$Q14_DIR/reconnect.pre.json"
  if [[ "$(milestone_status Q-TAB-14 "$post_key")" == "PASS" ]]; then
    record "Q-TAB-14::$post_key" PASS "resume preserved validated reconnect/no-replay evidence"
    return 0
  fi
  if [[ "$(milestone_status Q-TAB-14 "$pre_key")" != "PASS" ]]; then
    invoke_tablet_evidence Q-TAB-14 "$pre_key" reconnect-pre "$device_id" "$project_id" \
      "$(python3 - "$snapshot_id" <<'PY'
import json,sys
print(json.dumps({"runtime_snapshot_id":sys.argv[1]}))
PY
)" "$pre" || return $?
  elif [[ ! -s "$pre" ]]; then
    prior_pre="$(milestone_evidence Q-TAB-14 "$pre_key")"
    if [[ -n "$prior_pre" && -s "$prior_pre" ]]; then
      cp "$prior_pre" "$pre"
    elif [[ "$(milestone_status Q-TAB-14 "$disconnect_key")" != "PASS" ]]; then
      extra="$(python3 - "$snapshot_id" <<'PY'
import json,sys
print(json.dumps({"runtime_snapshot_id":sys.argv[1]}))
PY
)"
      set +e
      python3 - reconnect-pre "$device_id" "$project_id" "$extra" <<'PY' | \
        ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper tablet-evidence \
          >"$pre" 2>"$pre.stderr"
import json,sys
mode,device,project,extra=sys.argv[1:]
value={"mode":mode,"device_id":device,"project_id":project}
value.update(json.loads(extra))
print(json.dumps(value,separators=(",",":")))
PY
      recover_rc=$?
      set -e
      if [[ "$recover_rc" -ne 0 ]]; then
        record_milestone Q-TAB-14 "$pre_key" BLOCKED "$pre" "durable reconnect baseline was lost and could not be safely recaptured"
        return 3
      fi
      record_milestone Q-TAB-14 "$pre_key" PASS "$pre" "fresh reconnect baseline recaptured before any acknowledged disconnect"
    else
      record_gate Q-TAB-14 BLOCKED "$pre" "reconnect baseline evidence is unavailable after disconnect acknowledgement; invalidate Q-TAB-14 before retrying"
      return 3
    fi
  fi
  if [[ "$(milestone_status Q-TAB-14 "$disconnect_key")" != "PASS" || "$(milestone_status Q-TAB-14 "$reconnect_key")" != "PASS" ]]; then
    record "Q-TAB-14.manual" BLOCKED "disconnect only the Tablet network, restore it, acknowledge q14 disconnect/reconnect, then resume"
    return 3
  fi
  local baseline baseline_captured_at disconnect_at reconnect_at extra
  read -r baseline baseline_captured_at < <(python3 - "$pre" <<'PY'
import json,sys
data=json.load(open(sys.argv[1],encoding="utf-8"))
print(int(data["baseline_issued_at_us"]), data["captured_at"])
PY
)
  disconnect_at="$(milestone_updated_at Q-TAB-14 "$disconnect_key")"
  reconnect_at="$(milestone_updated_at Q-TAB-14 "$reconnect_key")"
  extra="$(python3 - "$snapshot_id" "$baseline" "$baseline_captured_at" "$disconnect_at" "$reconnect_at" <<'PY'
import json,sys
print(json.dumps({
 "runtime_snapshot_id":sys.argv[1],
 "baseline_issued_at_us":int(sys.argv[2]),
 "baseline_captured_at":sys.argv[3],
 "disconnect_at":sys.argv[4],
 "reconnect_at":sys.argv[5],
},separators=(",",":")))
PY
)"
  invoke_tablet_evidence Q-TAB-14 "$post_key" reconnect-post "$device_id" "$project_id" "$extra"
}


invoke_q20_truth() {
  local device_id="$1" project_id="$2"
  local key="published_config.truth"
  local snapshot_id="${STAGECORE_RUNTIME_SNAPSHOT_ID:-}"
  local published="$RUN_DIR/evidence/Q-DMX-20.published-config.json"
  local truth="$RUN_DIR/evidence/Q-DMX-20.published_config.truth.json"

  if ! should_run_milestone Q-DMX-20 "$key"; then
    record "Q-DMX-20::$key" PASS "resume preserved prior Published Runtime Snapshot configuration truth"
    return 0
  fi
  if [[ -z "$snapshot_id" ]]; then
    record_milestone Q-DMX-20 "$key" BLOCKED "$truth" "exact STAGECORE_RUNTIME_SNAPSHOT_ID is required for authoritative lighting configuration truth"
    return 3
  fi

  set +e
  python3 - "$device_id" "$project_id" "$snapshot_id" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper published-lighting-config >"$published" 2>"$published.stderr"
import json, sys
print(json.dumps({
    "device_id": sys.argv[1],
    "project_id": sys.argv[2],
    "runtime_snapshot_id": sys.argv[3],
}, separators=(",", ":")))
PY
  local published_rc=$?
  set -e
  if [[ "$published_rc" -eq 3 ]]; then
    record_milestone Q-DMX-20 "$key" BLOCKED "$published" "exact Published Runtime Snapshot lighting configuration is unavailable"
    return 3
  fi
  if [[ "$published_rc" -ne 0 ]]; then
    record_milestone Q-DMX-20 "$key" FAIL "$published" "Published Runtime Snapshot lighting identity/configuration evidence is invalid"
    return 1
  fi

  set +e
  go run ./tools/qualification/lighting-config-truth \
    --published "$published" \
    --probe "$RUN_DIR/evidence/devices.json" \
    --state-command "$RUN_DIR/evidence/Q-DMX-20.state_read.command.json" \
    --config-command "$RUN_DIR/evidence/Q-DMX-20.config_read.command.json" \
    --device-id "$device_id" --project-id "$project_id" \
    --runtime-snapshot-id "$snapshot_id" >"$truth" 2>"$truth.stderr"
  local truth_rc=$?
  set -e
  if [[ "$truth_rc" -eq 0 ]]; then
    record_milestone Q-DMX-20 "$key" PASS "$truth" "device observation, STATE_READ and installed CONFIG_READ all match the exact Published Runtime Snapshot configuration hash"
    return 0
  fi
  record_milestone Q-DMX-20 "$key" FAIL "$truth" "lighting configuration/readiness truth does not match the exact Published Runtime Snapshot"
  return 1
}

finalize_q20() {
  local device_id="$1" project_id="$2"
  local observation state_read config_read truth truth_evidence
  observation="$(milestone_status Q-DMX-20 observation.readiness)"
  state_read="$(milestone_status Q-DMX-20 state_read.command)"
  config_read="$(milestone_status Q-DMX-20 config_read.command)"
  truth="$(milestone_status Q-DMX-20 published_config.truth)"
  truth_evidence="$(milestone_evidence Q-DMX-20 published_config.truth)"
  [[ -n "$truth_evidence" ]] || truth_evidence="$RUN_DIR/evidence/Q-DMX-20.published_config.truth.json"

  if [[ "$observation" == "FAIL" || "$state_read" == "FAIL" || "$config_read" == "FAIL" || "$truth" == "FAIL" ]]; then
    record_gate Q-DMX-20 FAIL "$truth_evidence" "lighting observation/readiness/configuration truth failed; physical lighting actions are suppressed"
    return 1
  fi
  if [[ "$observation" == "PASS" && "$state_read" == "PASS" && "$config_read" == "PASS" && "$truth" == "PASS" ]]; then
    record_gate Q-DMX-20 PASS "$truth_evidence" "lighting observation/readiness and installed configuration exactly match the pinned Published Runtime Snapshot"
    return 0
  fi
  record_gate Q-DMX-20 BLOCKED "$truth_evidence" "lighting observation/readiness/configuration truth is incomplete"
  return 3
}

stage_q15_event() {
  local event="$1" key="$2" device_id="$3" project_id="$4"
  local pre_key="$key.pre" action_key="$key.action" post_key="$key.post"
  local pre="$Q15_DIR/$event.pre.json"
  local stable_post="$Q15_DIR/$event.post.json"
  local verify="$Q15_DIR/$event.verify.json"

  if [[ "$(milestone_status Q-DMX-15 "$post_key")" == "PASS" ]]; then
    record "Q-DMX-15::$post_key" PASS "resume preserved validated $event post evidence"
    return 0
  fi

  if [[ "$(milestone_status Q-DMX-15 "$pre_key")" != "PASS" ]]; then
    set +e
    python3 tools/qualification/power-event.py capture       --probe "$RUN_DIR/evidence/devices.json" --device-id "$device_id" --project-id "$project_id" --out "$pre"       >"$RUN_DIR/evidence/Q-DMX-15.$event.pre.log" 2>&1
    local capture_rc=$?
    set -e
    if [[ "$capture_rc" -eq 0 ]]; then
      record_milestone Q-DMX-15 "$pre_key" PASS "$pre" "$event baseline captured before manual hardware action"
    elif [[ "$capture_rc" -eq 3 ]]; then
      record_milestone Q-DMX-15 "$pre_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-15.$event.pre.log" "$event baseline target is not ready"
      return 3
    else
      record_milestone Q-DMX-15 "$pre_key" FAIL "$RUN_DIR/evidence/Q-DMX-15.$event.pre.log" "$event baseline capture failed"
      record_gate Q-DMX-15 FAIL "$RUN_DIR/evidence/Q-DMX-15.$event.pre.log" "Q-DMX-15 evidence capture failed; stop before unsafe hardware qualification"
      return 1
    fi
  fi

  local action_status
  action_status="$(milestone_status Q-DMX-15 "$action_key")"
  if [[ "$action_status" != "PASS" ]]; then
    if [[ "$action_status" != "BLOCKED" ]]; then
      record_milestone Q-DMX-15 "$action_key" BLOCKED "$pre" "awaiting explicit manual $event action; acknowledge only after the hardware action is complete"
    else
      record "Q-DMX-15::$action_key" BLOCKED "awaiting explicit manual $event action"
    fi
    return 3
  fi

  local post="$RUN_DIR/evidence/Q-DMX-15.$event.post.json"
  set +e
  python3 tools/qualification/power-event.py capture     --probe "$RUN_DIR/evidence/devices.json" --device-id "$device_id" --project-id "$project_id" --out "$post"     >"$RUN_DIR/evidence/Q-DMX-15.$event.post.log" 2>&1
  local post_rc=$?
  set -e
  if [[ "$post_rc" -ne 0 ]]; then
    record_milestone Q-DMX-15 "$post_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-15.$event.post.log" "$event post-reboot observation is not yet available"
    return 3
  fi

  local action_at
  action_at="$(milestone_updated_at Q-DMX-15 "$action_key")"
  set +e
  python3 tools/qualification/power-event.py verify     --event "$event" --pre "$pre" --post "$post" --action-at "$action_at" --out "$verify"     >"$RUN_DIR/evidence/Q-DMX-15.$event.verify.log" 2>&1
  local verify_rc=$?
  set -e
  if [[ "$verify_rc" -eq 0 ]]; then
    cp "$post" "$stable_post"
    record_milestone Q-DMX-15 "$post_key" PASS "$verify" "$event reboot/reset reason, fresh runtime, stable config hash, healthy DMX and logical blackout verified"
    return 0
  fi

  record_milestone Q-DMX-15 "$post_key" FAIL "$RUN_DIR/evidence/Q-DMX-15.$event.verify.log" "$event post evidence failed safe-output verification"
  record_gate Q-DMX-15 FAIL "$RUN_DIR/evidence/Q-DMX-15.$event.verify.log" "$event did not return to the required safe output state; stop and investigate before continuing"
  return 1
}

capture_live_device_probe() {
  local output="$1"
  ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper device-probe >"$output"
}

stage_q16_wifi_loss() {
  local device_id="$1" project_id="$2" channel_key="$3" test_level="$4" hold_seconds="$5"
  local precondition_key="wifi_loss.precondition_set"
  local pre_key="wifi_loss.pre"
  local disconnect_key="wifi_loss.disconnect_action"
  local reconnect_key="wifi_loss.reconnect_action"
  local post_key="wifi_loss.post"
  local stable_pre="$Q16_DIR/wifi-loss.pre.json"
  local stable_post="$Q16_DIR/wifi-loss.post.json"
  local verify="$Q16_DIR/wifi-loss.verify.json"

  if [[ "$(milestone_status Q-DMX-16 "$post_key")" == "PASS" ]]; then
    record "Q-DMX-16::$post_key" PASS "resume preserved validated Wi-Fi-loss post evidence"
    return 0
  fi

  if [[ "$(milestone_status Q-DMX-16 "$pre_key")" != "PASS" ]]; then
    if [[ "$(milestone_status Q-DMX-16 "$precondition_key")" != "PASS" ]]; then
      invoke_command physical-command Q-DMX-16 "$precondition_key" LIGHTING_CHANNELS_SET         "$device_id" "$project_id" "{\"channels\":{\"$channel_key\":$test_level}}"
    fi
    if [[ "$(milestone_status Q-DMX-16 "$precondition_key")" != "PASS" ]]; then
      return 3
    fi
    sleep "$hold_seconds"

    local probe="$RUN_DIR/evidence/Q-DMX-16.pre.probe.json"
    set +e
    capture_live_device_probe "$probe" >"$RUN_DIR/evidence/Q-DMX-16.pre.probe.log" 2>&1
    local probe_rc=$?
    set -e
    if [[ "$probe_rc" -ne 0 ]]; then
      record_milestone Q-DMX-16 "$precondition_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-16.pre.probe.log" "baseline probe unavailable; re-arm the nonzero precondition on resume"
      record_milestone Q-DMX-16 "$pre_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-16.pre.probe.log" "fresh nonzero Wi-Fi-loss baseline probe is unavailable"
      return 3
    fi

    set +e
    python3 tools/qualification/wifi-loss.py capture       --probe "$probe" --device-id "$device_id" --project-id "$project_id"       --require-nonzero-channel "$channel_key" --out "$stable_pre"       >"$RUN_DIR/evidence/Q-DMX-16.pre.log" 2>&1
    local pre_rc=$?
    set -e
    if [[ "$pre_rc" -eq 0 ]]; then
      record_milestone Q-DMX-16 "$pre_key" PASS "$stable_pre" "fresh nonzero lighting baseline captured before ESP-only Wi-Fi isolation"
    else
      record_milestone Q-DMX-16 "$precondition_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-16.pre.log" "nonzero baseline was not proven; re-arm the precondition on resume"
      record_milestone Q-DMX-16 "$pre_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-16.pre.log" "Wi-Fi-loss baseline is not a fresh nonzero lighting state"
      return 3
    fi
  fi

  local disconnect_status reconnect_status
  disconnect_status="$(milestone_status Q-DMX-16 "$disconnect_key")"
  reconnect_status="$(milestone_status Q-DMX-16 "$reconnect_key")"

  if [[ "$disconnect_status" != "PASS" ]]; then
    if [[ "$disconnect_status" != "BLOCKED" ]]; then
      record_milestone Q-DMX-16 "$disconnect_key" BLOCKED "$stable_pre" "awaiting explicit ESP-only Wi-Fi disconnect; do not power-cycle the node"
    else
      record "Q-DMX-16::$disconnect_key" BLOCKED "awaiting explicit ESP-only Wi-Fi disconnect"
    fi
    return 3
  fi

  if [[ "$reconnect_status" != "PASS" ]]; then
    if [[ "$reconnect_status" != "BLOCKED" ]]; then
      record_milestone Q-DMX-16 "$reconnect_key" BLOCKED "$stable_pre" "observe the connection-loss policy, restore Wi-Fi, then acknowledge reconnect"
    else
      record "Q-DMX-16::$reconnect_key" BLOCKED "awaiting Wi-Fi restore/reconnect acknowledgement"
    fi
    return 3
  fi

  local post_probe="$RUN_DIR/evidence/Q-DMX-16.post.probe.json"
  set +e
  capture_live_device_probe "$post_probe" >"$RUN_DIR/evidence/Q-DMX-16.post.probe.log" 2>&1
  local post_probe_rc=$?
  set -e
  if [[ "$post_probe_rc" -ne 0 ]]; then
    record_milestone Q-DMX-16 "$post_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-16.post.probe.log" "lighting node has not returned to a fresh StageCore observation yet"
    return 3
  fi

  local post="$RUN_DIR/evidence/Q-DMX-16.post.json"
  set +e
  python3 tools/qualification/wifi-loss.py capture     --probe "$post_probe" --device-id "$device_id" --project-id "$project_id" --out "$post"     >"$RUN_DIR/evidence/Q-DMX-16.post.log" 2>&1
  local post_rc=$?
  set -e
  if [[ "$post_rc" -ne 0 ]]; then
    record_milestone Q-DMX-16 "$post_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-16.post.log" "post-reconnect StageCore observation is not yet usable"
    return 3
  fi

  local disconnect_at reconnect_at
  disconnect_at="$(milestone_updated_at Q-DMX-16 "$disconnect_key")"
  reconnect_at="$(milestone_updated_at Q-DMX-16 "$reconnect_key")"
  set +e
  python3 tools/qualification/wifi-loss.py verify     --pre "$stable_pre" --post "$post" --disconnect-at "$disconnect_at" --reconnect-at "$reconnect_at" --out "$verify"     >"$RUN_DIR/evidence/Q-DMX-16.verify.log" 2>&1
  local verify_rc=$?
  set -e

  if [[ "$verify_rc" -eq 0 ]]; then
    cp "$post" "$stable_post"
    record_milestone Q-DMX-16 "$post_key" PASS "$verify" "Wi-Fi loss preserved identity/config, caused no reboot, and reconnect returned at safe blackout with healthy DMX"
    return 0
  fi

  record_milestone Q-DMX-16 "$post_key" FAIL "$RUN_DIR/evidence/Q-DMX-16.verify.log" "Wi-Fi-loss/reconnect evidence failed safe-output verification"
  record_gate Q-DMX-16 FAIL "$RUN_DIR/evidence/Q-DMX-16.verify.log" "Wi-Fi-loss failsafe/reconnect did not return to the required safe state"
  return 1
}

invoke_hub_restart_gate() {
  local device_id="$1" project_id="$2" channel_key="$3" start_level="$4" target_level="$5" fade_ms="$6" activation_timeout_ms="$7"
  local gate="Q-DMX-17" key="restart.sequence"
  if ! should_run_milestone "$gate" "$key"; then
    record "$gate::$key" PASS "resume preserved prior Hub-restart no-replay sequence"
    return 0
  fi
  local evidence="$RUN_DIR/evidence/$gate.$key.json"
  if [[ ! -f "$CREDENTIAL_FILE" ]]; then
    record_milestone "$gate" "$key" BLOCKED "$evidence" "local Operator credential not configured"
    return 3
  fi

  set +e
  python3 - "$CREDENTIAL_FILE" "$device_id" "$project_id" "$channel_key" "$start_level" "$target_level" "$fade_ms" "$activation_timeout_ms" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper hub-restart-gate >"$evidence" 2>"$evidence.stderr"
import json, sys
credential=json.load(open(sys.argv[1], encoding="utf-8"))
print(json.dumps({
    "username": credential.get("username", ""),
    "password": credential.get("password", ""),
    "device_id": sys.argv[2],
    "project_id": sys.argv[3],
    "channel_key": sys.argv[4],
    "start_level": float(sys.argv[5]),
    "target_level": float(sys.argv[6]),
    "fade_ms": int(sys.argv[7]),
    "activation_timeout_ms": int(sys.argv[8]),
}, separators=(",", ":")))
PY
  local rc=$?
  set -e

  if [[ "$rc" -eq 0 ]] && ! python3 tools/qualification/validate-hub-restart-evidence.py       --input "$evidence" --device-id "$device_id" >"$evidence.validation" 2>&1; then
    rc=1
  fi

  case "$rc" in
    0)
      record_milestone "$gate" "$key" PASS "$evidence" "active real fade was interrupted by Hub restart; reconnect remained safe and the old command was not replayed"
      return 0
      ;;
    3)
      record_milestone "$gate" "$key" BLOCKED "$evidence" "Hub-restart qualification could not obtain complete reconnect evidence"
      return 3
      ;;
    *)
      record_milestone "$gate" "$key" FAIL "$evidence" "Hub-restart/no-replay qualification failed"
      record_gate "$gate" FAIL "$evidence" "Hub restart exposed ambiguous/stale replay or unsafe reconnect behavior; suppress later lighting fault gates"
      return 1
      ;;
  esac
}

stage_q18_stability() {
  local device_id="$1" project_id="$2" channel_key="$3" test_level="$4" hold_seconds="$5" duration_seconds="$6" interval_ms="$7"
  local precondition_key="stress.precondition_set"
  local pre_key="stress.pre"
  local action_key="local_web.action"
  local rearm_key="stress.rearm_set"
  local stress_key="stress.auto"

  if [[ "$(milestone_status Q-DMX-18 "$stress_key")" == "PASS" ]]; then
    record "Q-DMX-18::$stress_key" PASS "resume preserved prior DMX-stability stress evidence"
    return 0
  fi

  if [[ "$(milestone_status Q-DMX-18 "$pre_key")" != "PASS" ]]; then
    if [[ "$(milestone_status Q-DMX-18 "$precondition_key")" != "PASS" ]]; then
      invoke_command physical-command Q-DMX-18 "$precondition_key" LIGHTING_CHANNELS_SET \
        "$device_id" "$project_id" "{\"channels\":{\"$channel_key\":$test_level}}"
    fi
    if [[ "$(milestone_status Q-DMX-18 "$precondition_key")" != "PASS" ]]; then
      return 3
    fi
    sleep "$hold_seconds"

    local probe="$RUN_DIR/evidence/Q-DMX-18.pre.probe.json"
    set +e
    capture_live_device_probe "$probe" >"$RUN_DIR/evidence/Q-DMX-18.pre.probe.log" 2>&1
    local probe_rc=$?
    set -e
    if [[ "$probe_rc" -ne 0 ]]; then
      record_milestone Q-DMX-18 "$precondition_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-18.pre.probe.log" "baseline probe unavailable; re-arm the fixed output on resume"
      record_milestone Q-DMX-18 "$pre_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-18.pre.probe.log" "fresh fixed-output baseline unavailable"
      return 3
    fi

    set +e
    python3 tools/qualification/wifi-loss.py capture \
      --probe "$probe" --device-id "$device_id" --project-id "$project_id" \
      --require-nonzero-channel "$channel_key" --out "$RUN_DIR/evidence/Q-DMX-18.pre.json" \
      >"$RUN_DIR/evidence/Q-DMX-18.pre.log" 2>&1
    local pre_rc=$?
    set -e
    if [[ "$pre_rc" -eq 0 ]]; then
      record_milestone Q-DMX-18 "$pre_key" PASS "$RUN_DIR/evidence/Q-DMX-18.pre.json" "fresh nonzero fixed-output baseline prepared before local-web/network stress"
    else
      record_milestone Q-DMX-18 "$precondition_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-18.pre.log" "fixed-output baseline was not proven; re-arm on resume"
      record_milestone Q-DMX-18 "$pre_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-18.pre.log" "DMX-stability baseline is not fresh/nonzero"
      return 3
    fi
  fi

  if [[ "$(milestone_status Q-DMX-18 "$action_key")" != "PASS" ]]; then
    if [[ "$(milestone_status Q-DMX-18 "$action_key")" != "BLOCKED" ]]; then
      record_milestone Q-DMX-18 "$action_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-18.pre.json" "open the protected local diagnostics web UI and keep read-only web activity active during the next resumed stress window"
    else
      record "Q-DMX-18::$action_key" BLOCKED "awaiting protected local-web activity acknowledgement"
    fi
    return 3
  fi

  if [[ "$(milestone_status Q-DMX-18 "$rearm_key")" != "PASS" ]]; then
    invoke_command physical-command Q-DMX-18 "$rearm_key" LIGHTING_CHANNELS_SET \
      "$device_id" "$project_id" "{\"channels\":{\"$channel_key\":$test_level}}"
  fi
  if [[ "$(milestone_status Q-DMX-18 "$rearm_key")" != "PASS" ]]; then
    record_milestone Q-DMX-18 "$action_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-18.pre.json" "re-arm failed; restart local-web activity before retry"
    return 3
  fi
  sleep "$hold_seconds"

  local evidence="$RUN_DIR/evidence/Q-DMX-18.stress.json"
  set +e
  python3 - "$device_id" "$project_id" "$channel_key" "$test_level" "$duration_seconds" "$interval_ms" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper dmx-stability-gate >"$evidence" 2>"$evidence.stderr"
import json, sys
print(json.dumps({
    "device_id": sys.argv[1],
    "project_id": sys.argv[2],
    "channel_key": sys.argv[3],
    "expected_level": float(sys.argv[4]),
    "duration_seconds": int(sys.argv[5]),
    "interval_ms": int(sys.argv[6]),
}, separators=(",", ":")))
PY
  local rc=$?
  set -e

  if [[ "$rc" -eq 0 ]] && ! python3 tools/qualification/validate-dmx-stability-evidence.py \
      --input "$evidence" --device-id "$device_id" --requested-level "$test_level" \
      --min-duration-seconds "$duration_seconds" >"$evidence.validation" 2>&1; then
    rc=1
  fi

  case "$rc" in
    0)
      record_milestone Q-DMX-18 "$stress_key" PASS "$evidence" "bounded Stage Device state/config traffic completed while fixed DMX state and health remained stable"
      return 0
      ;;
    3)
      record_milestone Q-DMX-18 "$stress_key" BLOCKED "$evidence" "network stress evidence path unavailable"
      record_milestone Q-DMX-18 "$action_key" BLOCKED "$evidence" "repeat protected local-web activity when retrying the stress window"
      return 3
      ;;
    *)
      record_milestone Q-DMX-18 "$stress_key" FAIL "$evidence" "DMX/network stability stress failed"
      record_gate Q-DMX-18 FAIL "$evidence" "DMX health/output/authority was not stable under bounded network/local-web stress"
      return 1
      ;;
  esac
}

stage_q19_emergency_blackout() {
  local device_id="$1" project_id="$2" channel_key="$3" test_level="$4" hold_seconds="$5"
  local precondition_key="emergency.precondition_set"
  local pre_key="emergency.pre"
  local unavailable_key="hub_unavailable.action"
  local blackout_key="local_blackout.action"
  local recovery_key="hub_recovery.action"
  local post_key="emergency.post"
  local stable_pre="$Q19_DIR/emergency.pre.json"
  local verify="$Q19_DIR/emergency.verify.json"

  if [[ "$(milestone_status Q-DMX-19 "$post_key")" == "PASS" ]]; then
    record "Q-DMX-19::$post_key" PASS "resume preserved validated local-emergency post evidence"
    return 0
  fi

  if [[ "$(milestone_status Q-DMX-19 "$pre_key")" != "PASS" ]]; then
    if [[ "$(milestone_status Q-DMX-19 "$precondition_key")" != "PASS" ]]; then
      invoke_command physical-command Q-DMX-19 "$precondition_key" LIGHTING_CHANNELS_SET \
        "$device_id" "$project_id" "{\"channels\":{\"$channel_key\":$test_level}}"
    fi
    if [[ "$(milestone_status Q-DMX-19 "$precondition_key")" != "PASS" ]]; then
      return 3
    fi
    sleep "$hold_seconds"

    local probe="$RUN_DIR/evidence/Q-DMX-19.pre.probe.json"
    set +e
    capture_live_device_probe "$probe" >"$RUN_DIR/evidence/Q-DMX-19.pre.probe.log" 2>&1
    local probe_rc=$?
    set -e
    if [[ "$probe_rc" -ne 0 ]]; then
      record_milestone Q-DMX-19 "$precondition_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.pre.probe.log" "baseline probe unavailable; re-arm the nonzero output on resume"
      record_milestone Q-DMX-19 "$pre_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.pre.probe.log" "fresh local-emergency baseline unavailable"
      return 3
    fi

    set +e
    python3 tools/qualification/emergency-blackout.py capture \
      --probe "$probe" --device-id "$device_id" --project-id "$project_id" \
      --require-nonzero-channel "$channel_key" --out "$stable_pre" \
      >"$RUN_DIR/evidence/Q-DMX-19.pre.log" 2>&1
    local pre_rc=$?
    set -e
    if [[ "$pre_rc" -eq 0 ]]; then
      record_milestone Q-DMX-19 "$pre_key" PASS "$stable_pre" "fresh nonzero baseline captured before taking only the Hub unavailable"
    else
      record_milestone Q-DMX-19 "$precondition_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.pre.log" "nonzero baseline was not proven; re-arm on resume"
      record_milestone Q-DMX-19 "$pre_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.pre.log" "local-emergency baseline is not fresh/nonzero"
      return 3
    fi
  fi

  if [[ "$(milestone_status Q-DMX-19 "$unavailable_key")" != "PASS" ]]; then
    local outage_evidence="$RUN_DIR/evidence/Q-DMX-19.hub-unavailable.log"
    set +e
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper service-stop \
      >"$outage_evidence" 2>&1
    local stop_rc=$?
    set -e
    if [[ "$stop_rc" -eq 0 ]]; then
      record_milestone Q-DMX-19 "$unavailable_key" PASS "$outage_evidence" "stagecore-hub.service stopped and verified inactive; ESP32 remains powered for local emergency blackout"
    else
      record_milestone Q-DMX-19 "$unavailable_key" FAIL "$outage_evidence" "could not establish bounded Hub-unavailable state"
      record_gate Q-DMX-19 FAIL "$outage_evidence" "Hub-unavailable precondition could not be established safely"
      return 1
    fi
  fi

  if [[ "$(milestone_status Q-DMX-19 "$blackout_key")" != "PASS" ]]; then
    if [[ "$(milestone_status Q-DMX-19 "$blackout_key")" != "BLOCKED" ]]; then
      record_milestone Q-DMX-19 "$blackout_key" BLOCKED "$stable_pre" "Hub is unavailable; use the protected local emergency-blackout control, confirm real blackout, then run q19-ack before resuming"
    else
      record "Q-DMX-19::$blackout_key" BLOCKED "Hub remains intentionally unavailable; awaiting protected local emergency-blackout acknowledgement"
    fi
    return 3
  fi

  if [[ "$(milestone_status Q-DMX-19 "$recovery_key")" != "PASS" ]]; then
    record "Q-DMX-19::$recovery_key" BLOCKED "resume runner so the bounded recovery hook can restart Hub before post verification"
    return 3
  fi

  local post_probe="$RUN_DIR/evidence/Q-DMX-19.post.probe.json"
  local stable_probe="$RUN_DIR/evidence/Q-DMX-19.stable.probe.json"
  local post="$RUN_DIR/evidence/Q-DMX-19.post.json"
  local stable="$RUN_DIR/evidence/Q-DMX-19.stable.json"

  set +e
  capture_live_device_probe "$post_probe" >"$RUN_DIR/evidence/Q-DMX-19.post.probe.log" 2>&1
  local post_probe_rc=$?
  set -e
  if [[ "$post_probe_rc" -ne 0 ]]; then
    record_milestone Q-DMX-19 "$post_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.post.probe.log" "lighting node has not returned to a fresh StageCore observation after Hub recovery"
    return 3
  fi
  python3 tools/qualification/emergency-blackout.py capture \
    --probe "$post_probe" --device-id "$device_id" --project-id "$project_id" --out "$post" \
    >"$RUN_DIR/evidence/Q-DMX-19.post.log" 2>&1 || {
      record_milestone Q-DMX-19 "$post_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.post.log" "post-recovery observation is not usable"
      return 3
    }

  sleep 2
  set +e
  capture_live_device_probe "$stable_probe" >"$RUN_DIR/evidence/Q-DMX-19.stable.probe.log" 2>&1
  local stable_probe_rc=$?
  set -e
  if [[ "$stable_probe_rc" -ne 0 ]]; then
    record_milestone Q-DMX-19 "$post_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.stable.probe.log" "stable post-recovery observation unavailable"
    return 3
  fi
  python3 tools/qualification/emergency-blackout.py capture \
    --probe "$stable_probe" --device-id "$device_id" --project-id "$project_id" --out "$stable" \
    >"$RUN_DIR/evidence/Q-DMX-19.stable.log" 2>&1 || {
      record_milestone Q-DMX-19 "$post_key" BLOCKED "$RUN_DIR/evidence/Q-DMX-19.stable.log" "stable post-recovery observation is not usable"
      return 3
    }

  local unavailable_at blackout_at recovery_at
  unavailable_at="$(milestone_updated_at Q-DMX-19 "$unavailable_key")"
  blackout_at="$(milestone_updated_at Q-DMX-19 "$blackout_key")"
  recovery_at="$(milestone_updated_at Q-DMX-19 "$recovery_key")"

  set +e
  python3 tools/qualification/emergency-blackout.py verify \
    --pre "$stable_pre" --post "$post" --stable "$stable" \
    --hub-unavailable-at "$unavailable_at" --blackout-at "$blackout_at" --recovery-at "$recovery_at" \
    --out "$verify" >"$RUN_DIR/evidence/Q-DMX-19.verify.log" 2>&1
  local verify_rc=$?
  set -e
  if [[ "$verify_rc" -eq 0 ]]; then
    record_milestone Q-DMX-19 "$post_key" PASS "$verify" "local emergency blackout remained safe through Hub recovery with no ESP reboot or stale replay"
    return 0
  fi

  record_milestone Q-DMX-19 "$post_key" FAIL "$RUN_DIR/evidence/Q-DMX-19.verify.log" "local emergency-blackout recovery evidence failed"
  record_gate Q-DMX-19 FAIL "$RUN_DIR/evidence/Q-DMX-19.verify.log" "local emergency blackout or recovery violated safe-state/no-replay requirements"
  return 1
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

recover_q19_if_needed() {
  if [[ "$(milestone_status Q-DMX-19 hub_unavailable.action)" != "PASS" ]]; then
    return 0
  fi
  if [[ "$(milestone_status Q-DMX-19 hub_recovery.action)" == "PASS" ]]; then
    return 0
  fi
  if [[ "$(milestone_status Q-DMX-19 local_blackout.action)" != "PASS" ]]; then
    return 0
  fi

  local evidence="$RUN_DIR/evidence/Q-DMX-19.hub-recovery.log"
  set +e
  ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper service-restart \
    >"$evidence" 2>&1
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    record_milestone Q-DMX-19 hub_recovery.action PASS "$evidence" "bounded recovery restarted stagecore-hub.service after local emergency blackout acknowledgement"
  else
    record_milestone Q-DMX-19 hub_recovery.action BLOCKED "$evidence" "Hub recovery failed; keep the local output at emergency blackout and retry recovery"
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
  recover_q19_if_needed
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
  run_gate_probe_milestone Q-DMX-20 observation.readiness lighting "${STAGECORE_LIGHTING_NODE_ID:-}" observation
else
  for gate in Q-TAB-04 Q-TAB-05; do
    if should_run_gate "$gate"; then
      record_gate "$gate" BLOCKED "$RUN_DIR/evidence/device-probe.stderr" "canonical device probe unavailable"
    else
      record "$gate" "$(gate_status "$gate")" "resume preserved prior terminal result"
    fi
  done
  if should_run_milestone Q-DMX-20 observation.readiness; then
    record_milestone Q-DMX-20 observation.readiness BLOCKED "$RUN_DIR/evidence/device-probe.stderr" "canonical device probe unavailable"
  else
    record "Q-DMX-20::observation.readiness" PASS "resume preserved prior canonical lighting observation"
  fi
  if should_run_gate Q-DMX-20; then
    record_gate Q-DMX-20 BLOCKED "$RUN_DIR/evidence/device-probe.stderr" "canonical device probe unavailable"
  else
    record "Q-DMX-20" "$(gate_status Q-DMX-20)" "resume preserved prior authoritative lighting configuration truth"
  fi
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


if [[ "$tablet_target_rc" -eq 0 && "$(gate_status Q-TAB-04)" == "PASS" && "$(gate_status Q-TAB-05)" == "PASS" ]]; then
  IFS="$(printf '\t')" read -r tablet_device tablet_project <<<"$tablet_target"
  media="${STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER:-}"
  cue_name="${STAGECORE_TABLET_QUALIFICATION_CUE_NAME:-}"
  missing_media="${STAGECORE_TABLET_QUALIFICATION_MISSING_MEDIA_NUMBER:-}"
  snapshot_id="${STAGECORE_RUNTIME_SNAPSHOT_ID:-}"
  manifest_id="$(python3 - "$RUN_DIR/evidence/devices.json" "$tablet_device" <<'PY'
import json,sys
data=json.load(open(sys.argv[1],encoding="utf-8"))
for d in data.get("devices",[]):
    if d.get("device_id")==sys.argv[2]:
        print(((d.get("runtime") or {}).get("observed") or {}).get("tablet_manifest_id",""))
        break
PY
)"

  if [[ -n "$cue_name" ]]; then
    cue_extra="$(python3 - "$cue_name" <<'PY'
import json,sys
print(json.dumps({"cue_name":sys.argv[1]},separators=(",",":")))
PY
)"
    invoke_tablet_evidence Q-TAB-11 cue_builder.canonical cue-canonical "$tablet_device" "$tablet_project" "$cue_extra" || true

    if [[ "$(gate_status Q-TAB-11)" == "PASS" && -n "$snapshot_id" ]]; then
      execution_extra="$(python3 - "$cue_name" "$snapshot_id" <<'PY'
import json,sys
print(json.dumps({"cue_name":sys.argv[1],"runtime_snapshot_id":sys.argv[2]},separators=(",",":")))
PY
)"
      invoke_tablet_evidence Q-TAB-12 published.execution cue-execution "$tablet_device" "$tablet_project" "$execution_extra" || true
    elif should_run_milestone Q-TAB-12 published.execution; then
      record_milestone Q-TAB-12 published.execution BLOCKED "$RUN_DIR/evidence/Q-TAB-12.published.execution.json" "first confirm Q-TAB-11 graphical builder, then Publish and execute the qualification Tablet Scene in the pinned Runtime Snapshot"
    fi
  else
    if should_run_milestone Q-TAB-11 cue_builder.canonical; then
      record_milestone Q-TAB-11 cue_builder.canonical BLOCKED "$RUN_DIR/evidence/Q-TAB-11.cue_builder.canonical.json" "STAGECORE_TABLET_QUALIFICATION_CUE_NAME is not configured"
    fi
    if should_run_milestone Q-TAB-12 published.execution; then
      record_milestone Q-TAB-12 published.execution BLOCKED "$RUN_DIR/evidence/Q-TAB-12.published.execution.json" "qualification Tablet Scene name is not configured"
    fi
  fi

  if [[ "$missing_media" =~ ^[1-9][0-9]*$ && "$missing_media" -le 9999 ]]; then
    invoke_missing_media "$tablet_device" "$tablet_project" "$missing_media" || true
  elif should_run_milestone Q-TAB-13 missing_media.command; then
    record_milestone Q-TAB-13 missing_media.command BLOCKED "$RUN_DIR/evidence/Q-TAB-13.missing_media.command.json" "configure an intentionally absent media number 1..9999"
  fi

  if [[ -n "$snapshot_id" && "$media" =~ ^[1-9][0-9]*$ ]]; then
    invoke_tablet_scope_gate "$tablet_device" "$tablet_project" "$snapshot_id" "$manifest_id" "$media" || true
  elif should_run_milestone Q-TAB-15 scope_mismatch.sequence; then
    record_milestone Q-TAB-15 scope_mismatch.sequence BLOCKED "$RUN_DIR/evidence/Q-TAB-15.scope_mismatch.sequence.json" "pinned Runtime Snapshot and qualification media are required"
    record_gate Q-TAB-15 BLOCKED "$RUN_DIR/evidence/Q-TAB-15.scope_mismatch.sequence.json" "scope mismatch test prerequisites unavailable"
  fi
fi

if [[ "$lighting_target_rc" -eq 0 && "$(milestone_status Q-DMX-20 observation.readiness)" == "PASS" ]]; then
  IFS="$(printf '\t')" read -r lighting_device lighting_project <<<"$lighting_target"
  invoke_command safe-command Q-DMX-20 state_read.command LIGHTING_STATE_READ "$lighting_device" "$lighting_project" '{}'
  invoke_command safe-command Q-DMX-20 config_read.command LIGHTING_CONFIG_READ "$lighting_device" "$lighting_project" '{}'
  if [[ "$(milestone_status Q-DMX-20 state_read.command)" == "PASS" && "$(milestone_status Q-DMX-20 config_read.command)" == "PASS" ]]; then
    invoke_q20_truth "$lighting_device" "$lighting_project" || true
  elif should_run_milestone Q-DMX-20 published_config.truth; then
    record_milestone Q-DMX-20 published_config.truth BLOCKED "$RUN_DIR/evidence/Q-DMX-20.published_config.truth.json" "bounded state/config reads must PASS before authoritative Published Runtime Snapshot comparison"
  fi
  finalize_q20 "$lighting_device" "$lighting_project" || true
else
  for key in state_read.command config_read.command published_config.truth; do
    if should_run_milestone Q-DMX-20 "$key"; then
      record_milestone Q-DMX-20 "$key" BLOCKED "$RUN_DIR/evidence/Q-DMX-20.$key.json" "Lighting observation/readiness target is not ready for authoritative configuration truth"
    else
      record "Q-DMX-20::$key" PASS "resume preserved prior milestone"
    fi
  done
  if [[ "$(milestone_status Q-DMX-20 observation.readiness)" == "FAIL" ]]; then
    record_gate Q-DMX-20 FAIL "$RUN_DIR/evidence/Q-DMX-20.observation.readiness.log" "lighting observation/readiness failed; authoritative configuration truth cannot be trusted"
  else
    record_gate Q-DMX-20 BLOCKED "$RUN_DIR/evidence/Q-DMX-20.observation.readiness.log" "lighting observation/readiness target is not ready for authoritative configuration truth"
  fi
fi

q21_status="$(gate_status Q-DMX-21)"
case "$q21_status" in
  PASS) record "Q-DMX-21" PASS "manual MAX485/DMX pre-energization electrical path is fully documented and physically verified" ;;
  FAIL) record "Q-DMX-21" FAIL "manual MAX485/DMX pre-energization electrical verification contains a failed check; do not energize" ;;
  *) record "Q-DMX-21" BLOCKED "manual MAX485/DMX pre-energization verification is incomplete; use campaign.sh q21-status/q21-ack before physical lighting actions" ;;
esac

if [[ "${STAGECORE_QUALIFICATION_ENABLE_PHYSICAL_ACTIONS:-0}" == "1" ]]; then
  hold="${STAGECORE_QUALIFICATION_PHYSICAL_HOLD_SECONDS:-2}"
  [[ "$hold" =~ ^[0-9]+([.][0-9]+)?$ ]] || { echo "invalid STAGECORE_QUALIFICATION_PHYSICAL_HOLD_SECONDS" >&2; exit 2; }

  if [[ "$tablet_target_rc" -eq 0 && "$(milestone_status Q-TAB-06 prepare.command)" == "PASS" ]]; then
    IFS="$(printf '\t')" read -r tablet_device tablet_project <<<"$tablet_target"
    media="${STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER:-}"
    overlay_media="${STAGECORE_TABLET_QUALIFICATION_OVERLAY_MEDIA_NUMBER:-}"
    live_media_key="${STAGECORE_TABLET_QUALIFICATION_LIVE_MEDIA_KEY:-}"
    if [[ "$media" =~ ^[1-9][0-9]*$ ]]; then
      invoke_command physical-command Q-TAB-06 play.command TABLET_PLAY "$tablet_device" "$tablet_project" "{\"media_number\":$media}"
      if [[ "$(milestone_status Q-TAB-06 play.command)" == "PASS" ]]; then
        sleep "$hold"

        if [[ "$overlay_media" =~ ^[1-9][0-9]*$ ]]; then
          invoke_command physical-command Q-TAB-08 overlay_play.command TABLET_OVERLAY_PLAY "$tablet_device" "$tablet_project" "{\"media_number\":$overlay_media}"
          sleep "$hold"
          invoke_command physical-command Q-TAB-08 overlay_clear.command TABLET_OVERLAY_CLEAR "$tablet_device" "$tablet_project" '{}'
        else
          for key in overlay_play.command overlay_clear.command; do
            if should_run_milestone Q-TAB-08 "$key"; then
              record_milestone Q-TAB-08 "$key" BLOCKED "$RUN_DIR/evidence/Q-TAB-08.$key.json" "qualification overlay media number is not configured"
            fi
          done
        fi

        if [[ -n "$live_media_key" && "${#live_media_key}" -le 256 ]]; then
          live_payload="$(python3 - "$live_media_key" <<'PY'
import json, sys
print(json.dumps({"media_key": sys.argv[1]}, separators=(",", ":")))
PY
)"
          invoke_command physical-command Q-TAB-09 live_show.command TABLET_LIVE_SHOW "$tablet_device" "$tablet_project" "$live_payload"
          sleep "$hold"
          invoke_command physical-command Q-TAB-09 live_hide.command TABLET_LIVE_HIDE "$tablet_device" "$tablet_project" '{}'
        else
          for key in live_show.command live_hide.command; do
            if should_run_milestone Q-TAB-09 "$key"; then
              record_milestone Q-TAB-09 "$key" BLOCKED "$RUN_DIR/evidence/Q-TAB-09.$key.json" "qualification live media key is not configured"
            fi
          done
        fi

        invoke_command physical-command Q-TAB-10 blackout.command TABLET_BLACKOUT "$tablet_device" "$tablet_project" '{}'
        sleep "$hold"
        invoke_command physical-command Q-TAB-10 blackout_clear.command TABLET_BLACKOUT_CLEAR "$tablet_device" "$tablet_project" '{}'
        sleep "$hold"

        invoke_command physical-command Q-TAB-07 pause.command TABLET_PAUSE "$tablet_device" "$tablet_project" '{}'
        sleep "$hold"
        invoke_command physical-command Q-TAB-07 stop.command TABLET_STOP "$tablet_device" "$tablet_project" '{}'
      fi
    fi
  fi

  if [[ "$tablet_target_rc" -eq 0 && "$(milestone_status Q-TAB-12 published.execution)" == "PASS" && "$(milestone_status Q-TAB-13 missing_media.command)" == "PASS" && "$(gate_status Q-TAB-15)" == "PASS" && -n "${STAGECORE_RUNTIME_SNAPSHOT_ID:-}" ]]; then
    IFS="$(printf '\t')" read -r tablet_device tablet_project <<<"$tablet_target"
    stage_q14_reconnect "$tablet_device" "$tablet_project" "$STAGECORE_RUNTIME_SNAPSHOT_ID" || true
  elif should_run_milestone Q-TAB-14 reconnect.pre; then
    record_milestone Q-TAB-14 reconnect.pre BLOCKED "$RUN_DIR/evidence/Q-TAB-14.reconnect.pre.json" "finish Published Cue, missing-media and scope-rejection evidence before preparing the no-replay reconnect window"
  fi

  if [[ "$(gate_status Q-DMX-21)" != "PASS" ]]; then
    if [[ "$(gate_status Q-DMX-21)" == "FAIL" ]]; then
      record "Q-DMX-21.safety-stop" FAIL "electrical path verification failed; all lighting physical actions suppressed"
    else
      record "Q-DMX-21.safety-stop" BLOCKED "complete Q-DMX-21 manual pre-energization electrical verification before any lighting physical action"
    fi
  elif [[ "$lighting_target_rc" -eq 0 && "$(gate_status Q-DMX-20)" == "PASS" ]]; then
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
    hub_restart_fade_ms="${STAGECORE_LIGHTING_QUALIFICATION_HUB_RESTART_FADE_MS:-}"
    hub_restart_activation_timeout_ms="${STAGECORE_LIGHTING_QUALIFICATION_HUB_RESTART_ACTIVATION_TIMEOUT_MS:-15000}"
    stability_seconds="${STAGECORE_LIGHTING_QUALIFICATION_STABILITY_SECONDS:-}"
    stability_interval_ms="${STAGECORE_LIGHTING_QUALIFICATION_STABILITY_INTERVAL_MS:-500}"

    single_ok=0
    multi_ok=0
    timed_ok=0
    timing_ok=0
    long_ok=0
    supersession_ok=0
    hub_restart_ok=0
    stability_ok=0
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
    if [[ "$single_ok" -eq 1 && "$set_level" != "$fade_level" && "$hub_restart_fade_ms" =~ ^[0-9]+$ && "$hub_restart_fade_ms" -ge 20000 && "$hub_restart_fade_ms" -le 120000 && "$hub_restart_activation_timeout_ms" =~ ^[0-9]+$ && "$hub_restart_activation_timeout_ms" -ge 1000 && "$hub_restart_activation_timeout_ms" -le 20000 ]]; then
      hub_restart_ok=1
    fi
    if [[ "$single_ok" -eq 1 && "$set_level" != "0" && "$set_level" != "0.0" && "$stability_seconds" =~ ^[0-9]+$ && "$stability_seconds" -ge 10 && "$stability_seconds" -le 300 && "$stability_interval_ms" =~ ^[0-9]+$ && "$stability_interval_ms" -ge 100 && "$stability_interval_ms" -le 5000 ]]; then
      stability_ok=1
    fi

    if [[ "$(gate_status Q-DMX-15)" != "FAIL" && "$(gate_status Q-DMX-15)" != "PASS" ]]; then
      stage_q15_event power-cycle power_cycle "$lighting_device" "$lighting_project" || true
      if [[ "$(milestone_status Q-DMX-15 power_cycle.post)" == "PASS" ]]; then
        stage_q15_event brownout brownout "$lighting_device" "$lighting_project" || true
      fi
    fi

    if [[ "$(gate_status Q-DMX-15)" == "FAIL" ]]; then
      record "Q-DMX-15.safety-stop" FAIL "power-event safe-output verification failed; remaining lighting physical actions suppressed"
    else
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

    if [[ "$(milestone_status Q-DMX-15 power_cycle.post)" == "PASS" && "$(milestone_status Q-DMX-15 brownout.post)" == "PASS" ]]; then
      if [[ "${STAGECORE_QUALIFICATION_ENABLE_HUB_RESTART:-0}" == "1" && "$hub_restart_ok" -eq 1 ]]; then
        invoke_hub_restart_gate "$lighting_device" "$lighting_project" "$channel" "$set_level" "$fade_level" "$hub_restart_fade_ms" "$hub_restart_activation_timeout_ms" || true
      elif should_run_milestone Q-DMX-17 restart.sequence; then
        record_milestone Q-DMX-17 restart.sequence BLOCKED "$RUN_DIR/evidence/Q-DMX-17.restart.sequence.json" "Hub-restart gate requires explicit restart arm plus distinct levels and fade_ms 20000..120000"
      fi
    elif should_run_milestone Q-DMX-17 restart.sequence; then
      record_milestone Q-DMX-17 restart.sequence BLOCKED "$RUN_DIR/evidence/Q-DMX-17.restart.sequence.json" "complete Q-DMX-15 automated power-event evidence before Hub-restart qualification"
    fi

    if [[ "$(gate_status Q-DMX-17)" == "FAIL" ]]; then
      record "Q-DMX-17.safety-stop" FAIL "Hub restart exposed replay/authority failure; Wi-Fi-loss fault gate suppressed"
    elif [[ "$(milestone_status Q-DMX-15 power_cycle.post)" == "PASS" && "$(milestone_status Q-DMX-15 brownout.post)" == "PASS" ]]; then
      if [[ "$single_ok" -eq 1 && "$set_level" != "0" && "$set_level" != "0.0" ]]; then
        stage_q16_wifi_loss "$lighting_device" "$lighting_project" "$channel" "$set_level" "$hold" || true
      elif should_run_milestone Q-DMX-16 wifi_loss.pre; then
        record_milestone Q-DMX-16 wifi_loss.pre BLOCKED "$RUN_DIR/evidence/Q-DMX-16.pre.log" "Wi-Fi-loss qualification requires a configured nonzero single-channel test level"
      fi
    elif should_run_milestone Q-DMX-16 wifi_loss.pre; then
      record_milestone Q-DMX-16 wifi_loss.pre BLOCKED "$RUN_DIR/evidence/Q-DMX-16.pre.log" "complete Q-DMX-15 automated power-event evidence before preparing Wi-Fi-loss qualification"
    fi

    if [[ "$stability_ok" -eq 1 ]]; then
      stage_q18_stability "$lighting_device" "$lighting_project" "$channel" "$set_level" "$hold" "$stability_seconds" "$stability_interval_ms" || true
    elif should_run_milestone Q-DMX-18 stress.pre; then
      record_milestone Q-DMX-18 stress.pre BLOCKED "$RUN_DIR/evidence/Q-DMX-18.pre.log" "configure a nonzero test level and explicit stability duration 10..300 seconds"
    fi

    if [[ "${STAGECORE_QUALIFICATION_ENABLE_HUB_UNAVAILABLE:-0}" == "1" && "$single_ok" -eq 1 && "$set_level" != "0" && "$set_level" != "0.0" ]]; then
      stage_q19_emergency_blackout "$lighting_device" "$lighting_project" "$channel" "$set_level" "$hold" || true
    elif should_run_milestone Q-DMX-19 emergency.pre; then
      record_milestone Q-DMX-19 emergency.pre BLOCKED "$RUN_DIR/evidence/Q-DMX-19.pre.log" "local emergency gate requires explicit Hub-unavailable arm and a configured nonzero single-channel test level"
    fi
    fi
    fi
  fi

evaluate_qdmx22() {
  local key="regression.prereqs"
  local evidence="$RUN_DIR/evidence/Q-DMX-22.regression.prereqs.json"

  if ! should_run_milestone Q-DMX-22 "$key"; then
    record "Q-DMX-22::$key" PASS "resume preserved prior full-chain prerequisite aggregate"
    return 0
  fi

  set +e
  python3 tools/qualification/dmx-full-chain.py --state "$STATE_FILE" --out "$evidence" \
    >"$evidence.log" 2>&1
  local rc=$?
  set -e
  case "$rc" in
    0)
      record_milestone Q-DMX-22 "$key" PASS "$evidence" "all Q-DMX-01..21 prerequisites are PASS/N/A on the pinned StageCore/firmware/hardware baseline"
      return 0
      ;;
    3)
      record_milestone Q-DMX-22 "$key" BLOCKED "$evidence" "full-chain regression prerequisites are not complete"
      record_gate Q-DMX-22 BLOCKED "$evidence" "full Raspberry Pi + Stage LAN + ESP32 + DMX + 24 V regression awaits all prerequisite gates"
      return 3
      ;;
    *)
      record_milestone Q-DMX-22 "$key" FAIL "$evidence" "one or more full-chain prerequisite gates failed"
      record_gate Q-DMX-22 FAIL "$evidence" "full-chain regression cannot pass while a prerequisite Q-DMX gate is failed"
      return 1
      ;;
  esac
}

evaluate_qdmx22 || true

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
