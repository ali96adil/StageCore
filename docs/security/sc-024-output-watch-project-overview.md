# SC-024 Phase 5G — Project output overview and authenticated transport activity

**2026-10-18 · Integration PR #530 (Draft) · opt-in read-only UI + Hub evidence only.**

## One-click Project overview

The Stage Devices page has a bilingual on-demand **Read project output summary / ملخص مراقبة مخارج المشروع** control. The Operator clicks once; the UI fetches existing opt-in scoped `GET .../output-watch` reports for up to 40 active V2 devices, in batches of four. It does not poll in the background, send `POST`, retry failed Cues, or actuate any output. It includes a count of devices that the limited V2 view cannot qualify; no hidden global-inventory or foreign-Project data is imported.

Each response MUST be for the exact selected Project and device, with read-only=true, automated_replay=false, physical_status=UNKNOWN, commands_enabled=false and physical_output_verified=false. Unexpected, failed, revoked or unavailable results are UNKNOWN. A switched Project/page discards asynchronous results. The preview never uses a successful network request as GO readiness.

## Transport activity is NOT a device heartbeat

The V2 DeviceChannel already stores an atomic `lastActivity` on its authenticated WebSocket connection. The new **read-only** `CurrentV2TransportActivity(deviceID)` getter provides a timestamp and the exact Hub-issued connection generation. It has no dispatch or device-state side effects.

The Output Watch reader requires:
- the same exact Device, Project, active assignment Epoch and Runtime Snapshot;
- the activity record's generation to match `CurrentV2Scope`, with activity in the past 30 seconds;
- for optional software-level evidence, a separately available current-generation report no older than five seconds, with no future clock skew.

A transport timestamp in the future yields CLOCK_UNCERTAIN. An inactive timestamp older than 30 seconds yields HEARTBEAT_STALE *as a diagnostic liveness bucket only*; that term is not proof that the device's own physical failsafe or application heartbeat was checked. An active authenticated socket is not proof that any Cue executed. `device_heartbeat_verified` remains **false** until a protocol-specific, authenticated and replay-protected heartbeat is designed and integrated. A current software-zero report never proves an actual physical zero or safe state.

This is a **limited V2 Stage Device view**. Companion/Ableton/VDMX OSC, legacy devices, and other physical outputs are not yet covered. Their exclusion is explicit; the unified overview must not represent them as safely stopped.

## Rollout / tests

- `internal/outputwatch/hub_evidence_test.go` covers foreign Project, wrong Snapshot, stale generation and report, stale transport and future clock timestamps. Recent transport remains distinct from a verified heartbeat and physical output.
- `internal/operatorweb/output_watchdog_contract_test.go` protects bilingual labels, bounded GET-only summaries, project-scoped path checks and absence of automatic polling or dispatch.
- Existing opt-in HTTP endpoint rechecks Project grants before and after reading, and remains UNWIRED in the live Hub. This phase did not add an HTTP registration, so the source-route inventory remains **264 registrations, including 17 dynamic**.
- No production Project ACL enablement, no GO/STOP change, no Pi deployment, no Stage Device firmware flash, no physical output command.

Next integration gate: source-specific signed/acknowledged heartbeat semantics and independent physical output verification, with test-based recovery from revoked runtime Session credentials and Hub restarts. A green CI run is not an attended Stage safety qualification.
