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
  STAGECORE_QUALIFICATION_STATE     Optional durable campaign state path.
                                    Default: ~/.local/state/stagecore/qualification-campaign.json
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

record_milestone() {
  local gate="$1" key="$2" status="$3" evidence="$4" note="$5"
  python3 tools/qualification/qualification-milestone.py record \
    --state "$STATE_FILE" --manifest "$MANIFEST" --gate "$gate" --key "$key" --status "$status" \
    --actor runner --evidence "$evidence" --note "$note" >/dev/null
  record "$gate::$key" "$status" "$note; evidence=$evidence"
}

should_run_milestone() {
  local gate="$1" key="$2"
  if [[ "$MODE" != "resume" ]]; then
    return 0
  fi
  [[ "$(milestone_status "$gate" "$key")" != "PASS" ]]
}

record_gate() {
  local gate="$1" status="$2" evidence="$3" note="$4"
  python3 tools/qualification/qualification-state.py record     --state "$STATE_FILE" --manifest "$MANIFEST" --gate "$gate" --status "$status"     --actor runner --evidence "$evidence" --note "$note" >/dev/null
  record "$gate" "$status" "$note; evidence=$evidence"
}

should_run_gate() {
  local gate="$1"
  if [[ "$MODE" != "resume" ]]; then
    return 0
  fi
  local status
  status="$(gate_status "$gate")"
  [[ "$status" != "PASS" && "$status" != "N/A" ]]
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
  if [[ -n "${STAGECORE_PROJECT_ID:-}" ]]; then
    args+=(--project-id "$STAGECORE_PROJECT_ID")
  fi
  if [[ -n "${STAGECORE_RUNTIME_SNAPSHOT_ID:-}" ]]; then
    args+=(--runtime-snapshot-id "$STAGECORE_RUNTIME_SNAPSHOT_ID")
  fi
  if [[ -n "$device_id" ]]; then
    args+=(--device-id "$device_id")
  fi

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

check_cmd "local.git" git rev-parse HEAD || true
check_cmd "local.go" go version || true
check_cmd "local.tests" go test ./... || true
check_cmd "manifest.validation" python3 tools/qualification/validate-manifest.py --summary || true
cp "$MANIFEST" "$RUN_DIR/evidence/qualification-manifest.json"

CREDENTIAL_FILE="${STAGECORE_QUALIFICATION_OPERATOR_CREDENTIAL:-$HOME/.config/stagecore/qualification-operator.json}"

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
  if ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper device-probe     >"$RUN_DIR/evidence/devices.json" 2>"$RUN_DIR/evidence/device-probe.stderr"; then
    PROBE_AVAILABLE=1
    record "devices.probe" PASS "see evidence/devices.json"
  else
    record "devices.probe" FAIL "see evidence/device-probe.stderr"
  fi
fi

if [[ "$PROBE_AVAILABLE" -eq 1 ]]; then
  run_gate_probe "Q-TAB-04" tablet "${STAGECORE_TABLET_DEVICE_ID:-}" readiness
  run_gate_probe "Q-TAB-05" tablet "${STAGECORE_TABLET_DEVICE_ID:-}" scope
  run_gate_probe "Q-DMX-20" lighting "${STAGECORE_LIGHTING_NODE_ID:-}" observation
else
  for gate in Q-TAB-04 Q-TAB-05 Q-DMX-20; do
    if should_run_gate "$gate"; then
      record_gate "$gate" BLOCKED "$RUN_DIR/evidence/device-probe.stderr" "canonical device probe unavailable"
    else
      record "$gate" "$(gate_status "$gate")" "resume preserved prior terminal result"
    fi
  done
fi


resolve_target() {
  local kind="$1" configured_device="$2"
  local args=(--input "$RUN_DIR/evidence/devices.json" --kind "$kind")
  if [[ -n "${STAGECORE_PROJECT_ID:-}" ]]; then
    args+=(--project-id "$STAGECORE_PROJECT_ID")
  fi
  if [[ -n "$configured_device" ]]; then
    args+=(--device-id "$configured_device")
  fi
  python3 tools/qualification/resolve-device-target.py "${args[@]}"
}

invoke_safe_command() {
  local gate="$1" key="$2" command_type="$3" device_id="$4" project_id="$5" media_number="$6"
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
  python3 - "$CREDENTIAL_FILE" "$project_id" "$device_id" "$command_type" "$media_number" <<'PY' | \
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" sudo -n /usr/local/libexec/stagecore-qualification-helper safe-command >"$evidence" 2>"$evidence.stderr"
import json, sys
credential=json.load(open(sys.argv[1], encoding="utf-8"))
project_id, device_id, command_type, media_number = sys.argv[2:6]
payload={}
if command_type == "TABLET_PREPARE":
    payload={"media_number": int(media_number)}
print(json.dumps({
    "username": credential.get("username", ""),
    "password": credential.get("password", ""),
    "project_id": project_id,
    "device_id": device_id,
    "command_type": command_type,
    "payload": payload,
}, separators=(",", ":")))
PY
  local rc=$?
  set -e
  case "$rc" in
    0) record_milestone "$gate" "$key" PASS "$evidence" "$command_type completed through canonical Stage Device result path" ;;
    3) record_milestone "$gate" "$key" BLOCKED "$evidence" "$command_type did not reach a terminal result within the bounded wait" ;;
    *) record_milestone "$gate" "$key" FAIL "$evidence" "$command_type qualification command failed" ;;
  esac
}

if [[ "$PROBE_AVAILABLE" -eq 1 && -n "${SSH_TARGET:-}" ]]; then
  set +e
  tablet_target="$(resolve_target tablet "${STAGECORE_TABLET_DEVICE_ID:-}")"
  tablet_target_rc=$?
  lighting_target="$(resolve_target lighting "${STAGECORE_LIGHTING_NODE_ID:-}")"
  lighting_target_rc=$?
  set -e

  if [[ "$tablet_target_rc" -eq 0 && "$(gate_status Q-TAB-04)" == "PASS" && "$(gate_status Q-TAB-05)" == "PASS" ]]; then
    IFS=$'\t' read -r tablet_device tablet_project <<<"$tablet_target"
    if [[ "${STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER:-}" =~ ^[1-9][0-9]*$ ]]; then
      invoke_safe_command Q-TAB-06 prepare.command TABLET_PREPARE "$tablet_device" "$tablet_project" "$STAGECORE_TABLET_QUALIFICATION_MEDIA_NUMBER"
    else
      if should_run_milestone Q-TAB-06 prepare.command; then
        record_milestone Q-TAB-06 prepare.command BLOCKED "$RUN_DIR/evidence/Q-TAB-06.prepare.command.json" "qualification media number is not configured"
      else
        record "Q-TAB-06::prepare.command" PASS "resume preserved prior milestone"
      fi
    fi
  else
    if should_run_milestone Q-TAB-06 prepare.command; then
      record_milestone Q-TAB-06 prepare.command BLOCKED "$RUN_DIR/evidence/Q-TAB-06.prepare.command.json" "Tablet readiness/scope target is not ready for PREPARE"
    else
      record "Q-TAB-06::prepare.command" PASS "resume preserved prior milestone"
    fi
  fi

  if [[ "$lighting_target_rc" -eq 0 && "$(gate_status Q-DMX-20)" == "PASS" ]]; then
    IFS=$'\t' read -r lighting_device lighting_project <<<"$lighting_target"
    invoke_safe_command Q-DMX-20 state_read.command LIGHTING_STATE_READ "$lighting_device" "$lighting_project" ""
    invoke_safe_command Q-DMX-20 config_read.command LIGHTING_CONFIG_READ "$lighting_device" "$lighting_project" ""
  else
    for key in state_read.command config_read.command; do
      if should_run_milestone Q-DMX-20 "$key"; then
        record_milestone Q-DMX-20 "$key" BLOCKED "$RUN_DIR/evidence/Q-DMX-20.$key.json" "Lighting target is not ready for bounded read command"
      else
        record "Q-DMX-20::$key" PASS "resume preserved prior milestone"
      fi
    done
  fi
fi

cp "$STATE_FILE" "$RUN_DIR/evidence/campaign-state.json"

{
  echo "# StageCore Physical Qualification Report"
  echo
  printf -- '- Run: `%s`\n' "$STAMP"
  printf -- '- Mode: `%s`\n' "$MODE"
  printf -- '- Non-interactive: `%s`\n' "$NON_INTERACTIVE"
  printf -- '- Repository HEAD: `%s`\n' "$(git rev-parse HEAD 2>/dev/null || echo unknown)"
  printf -- '- Campaign state: `%s`\n' "$STATE_FILE"
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
