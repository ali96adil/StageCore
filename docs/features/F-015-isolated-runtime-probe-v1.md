# F-015 — Isolated Runtime Probe v1

**Status:** implementation checkpoint  
**Scope:** first bounded execution of installable Plugin bytes inside the F-015 Linux isolation boundary.

## Purpose

This slice advances F-015 from a non-executing isolation plan to a real, short-lived `plugin.ready` probe. It does not create a persistent enabled/running lifecycle.

An installable Plugin may be probed only when the existing extension readiness checks and Runtime Isolation Contract v1 both authorize it.

## Transient executable rule

The immutable installed `payload.pkg` remains mode `0440` and is never executed directly.

For each probe StageCore:

1. re-verifies the installed payload and runtime artifact;
2. copies the exact bytes into `<extensions-root>/runtime/probe/probe-*.bin`;
3. verifies exact size and SHA-256 while copying;
4. fsyncs the transient file;
5. changes only the transient copy to mode `0500`;
6. re-verifies the immutable installed payload before execution;
7. launches only the transient copy through the approved Bubblewrap plan;
8. stops the process immediately after the bounded handshake;
9. verifies the transient copy and immutable source again;
10. removes the transient executable and syncs the managed directory.

Startup cleanup removes only regular managed `probe-*.bin` files. Any unexpected entry fails closed.

## Isolation boundary

The probe uses the Runtime Isolation Contract v1:

- `--unshare-all`;
- no host network sharing;
- cleared environment;
- only the transient executable bound read-only at `/stagecore/plugin`;
- private `/proc`, `/dev`, and `/tmp`;
- no StageCore data, Vault, config, installed-extension tree, or home-directory mounts.

If Bubblewrap is unavailable, the probe does not fall back to direct execution.

Approved `network.*` permissions remain blocked with `RUNTIME_NETWORK_BROKER_REQUIRED`; this slice does not weaken that boundary to make a network Plugin run.

## Live `plugin.ready` contract

StageCore now starts the isolated process and requires a bounded `plugin.ready` handshake through the existing Plugin Protocol v1 parser.

The probe additionally requires:

- exact `plugin_id` match with the installed extension ID;
- exact `plugin_version` match with the installed extension version;
- exact advertised capability set match with the registered manifest.

A process that changes identity, version, or capabilities at runtime fails the probe and is terminated.

## Operator API

Plugin-management authority may request:

`POST /api/v1/extensions/installations/{installation_id}/runtime-probe`

Successful response status is:

`PROBE_VERIFIED`

The response truthfully reports that the bounded process was started and stopped, while persistent execution remains unauthorized:

- `probe_execution_authorized: true`;
- `process_started: true`;
- `process_stopped: true`;
- `persistent_execution_authorized: false`;
- `persistent_execution_blocker: RUNTIME_LIFECYCLE_REQUIRED`.

Attempts are security-audited as:

`extension.runtime.probe`

Active SHOW remains a blocker before staging, immediately before execution, and after the process stops.

## Explicit non-goals

This checkpoint does not yet:

- persist enabled/disabled state;
- leave an extension process running after the probe;
- grant durable runtime permissions;
- expose direct host networking;
- implement the StageCore network broker;
- update, repair, remove, or roll back an installed extension;
- expose the final bilingual Extension Manager UI.

## Verification

Coverage proves:

- the existing Plugin Host can start and return `plugin.ready` without sending an execution request;
- a ready no-network installation is launched only through the isolation plan;
- the process is closed immediately after the handshake;
- the transient `0500` executable is removed;
- the immutable installed payload remains non-executable `0440`;
- identity/version mismatch fails closed and still cleans up;
- approved network permissions block before process creation;
- startup cleanup removes only known managed probe files and fails closed on unexpected entries.

## Next dependency-first slice

The next F-015 slice should introduce a durable Enable/Disable lifecycle that performs a fresh hash-bound isolated probe before enabling, keeps the process under managed supervision, and stops it deterministically on disable, Hub shutdown, crash, or SHOW policy transition. Persistent state must distinguish configured `ENABLED` intent from observed `RUNNING/READY/FAILED` runtime truth.
