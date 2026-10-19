# SC-024 Phase 5H — Companion Machine Role watch (Ableton/VDMX)

**2026-10-19 · Draft integration PR #530 · opt-in, GET-only, uninstalled.**

The Stage Devices on-demand Output Watch summary now includes **only the
Companion Machine Roles belonging to the selected Project**, with simple
AUDIO/VIDEO categorization for role keys beginning with AUDIO- or VIDEO-.
For example, AUDIO-ABLETON and VIDEO-VDMX are labels, **not** proof that
Ableton is playing or that VDMX received an OSC packet.

## Trusted evidence hierarchy

The read model `outputwatch.CompanionRoleEvidence` uses the Hub-owned
`ListMachineRoles(ctx, projectID)`, followed by **only** the current
assignment and named Companion for each role. It never calls the global
`ListCompanions` endpoint or returns unrelated hosts, MIDI destination
names, IP addresses, credentials, or machine paths.

The read qualifies these independent observations:
- trusted identity and current authenticated Companion WebSocket from
  `companionchannel.RuntimeChannel.IsConnected(companionID)`;
- Companion last-seen report is neither future-dated nor older than a
  configurable positive heartbeat deadline;
- required published Runtime Snapshot, role configuration and required
  capabilities match the current Companion-reported configuration;
- active assignment is `READY`, and role+Companion evaluation is READY.

Outcomes include UNASSIGNED, OFFLINE, UNTRUSTED, CLOCK_UNCERTAIN, STALE,
MISMATCH, DEGRADED, and COMPANION_READY. **COMPANION_READY proves only
reported transport and role readiness.** It does NOT assert MIDI
command execution, Ableton scene playback, VDMX OSC delivery, output
voltage, video/audio content, or physical safety. The response sets
`midi_execution_confirmed:false`, `osc_delivery_confirmed:false`,
`physical_output_verified:false` unconditionally.

The report fails closed if any returned role belongs to a different
Project, if role/assignment identities mismatch, or if the unpaginated
inventory exceeds 64 items. These are diagnostics, not command authority.

## Browser/API

`WithScopedCompanionRoleWatch` is an **opt-in, unwired** Handler Option:

`GET /api/v1/projects/{project_id}/companion-output-watch`

The authenticated browser must have a live `project.view` grant, checked
before and after the read. The endpoint sends `Cache-Control:no-store`,
requires secure origin, denies foreign/missing Project access consistently,
and cannot trigger Companion operation. It is included in the negative
production assembler guard.

The bilingual Stage Devices on-demand summary fetches this endpoint for
the **exact selected Project** and rejects any response that suggests
successful OSC/MIDI execution or physical output verification. If the
opt-in API is missing, the UI explicitly says Companion role observation
is unavailable and OSC/MIDI are UNKNOWN (never zero roles or READY).
No background polling, auto-retry, command replay, silent GO or STOP is
introduced. The pre-existing device V2 summary remains available.

## Verification and gates

New tests cover current transport + fresh report, unplugged Companion,
stale and future timestamps, mismatched required Snapshot, untrusted
identity, degraded readiness, foreign Project assignment/role and role
inventory ceiling. API tests cover anonymous, insecure-origin,
unauthorized Project, revocation, wrong-project reader result and
unconditional false command confirmation. UI contract tests require
both languages and reject command/polling code.

**Next unresolved safety boundary:** actual OSC/MIDI delivery
acknowledgment from the *receiving application*, not merely Companion's
send completion. That requires Ableton/Max for Live or VDMX-side state
feedback designed, authenticated, correlated to a Cue and tested after
network/Hub restarts. Without this, show the output state as UNKNOWN.

No `main` merge, Pi/Companion update, device firmware changes, GO/STOP
modification, physical output or all-route ACL cutover.
