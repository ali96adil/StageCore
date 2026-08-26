# 00 — Roles & Boundaries

## Goal

The Companion exists to execute work on a replaceable Mac/PC without moving Project authority away from the Hub.

## Hub Owns

- Project and ProjectRevision state;
- Published Runtime Snapshot;
- Machine Role definitions and assignments;
- permissions/trust decisions;
- Cue/Route state;
- required media manifest;
- execution journal and Session history;
- authoritative readiness result.

## Companion Owns Locally

- machine identity material;
- machine-local capability discovery;
- local app/device integration;
- bounded runtime cache for the assigned role;
- local media copies required by its role;
- machine health observations;
- local execution result before reporting to Hub.

## Companion Must Not

- become a second editable Project database;
- invent or mutate Published Runtime state;
- replay the previous non-idempotent Action after reconnect;
- keep executing new Hub-dependent commands after trust is revoked;
- expose secrets in normal logs;
- bypass Hub mode/permission/safety checks.

## Replaceability Rule

A Cue targets a logical Alias/Machine Role, not a specific computer hostname. Replacing the Mac should require a new pairing/role assignment and sync, not editing every Cue.