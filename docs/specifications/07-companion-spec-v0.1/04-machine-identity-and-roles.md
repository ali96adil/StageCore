# 04 — Machine Identity & Roles

## Machine Identity

Each Companion presents a stable `companion_id` plus machine metadata:

- display name;
- OS/platform and architecture;
- Companion version;
- machine capabilities;
- hardware/storage summary relevant to readiness;
- last-seen timestamp;
- trust state.

The display name and hostname may change without changing identity.

## Machine Roles

Projects target logical roles such as:

- `VIDEO-MAIN`
- `VIDEO-BACKUP`
- `AUDIO`
- `PROJECTION`
- `OPERATOR-01`

Role configuration belongs to the Project/Hub. Machine configuration remains local where appropriate.

## Role lifecycle

A Machine Role has a stable identity:

- `machine_role_id` is immutable;
- `role_key` is immutable once created because published Runtime Snapshots bind that key to the role identity;
- display name, required capabilities, and the Required-for-readiness flag may be edited when the Project is not locked by an active SHOW;
- editing capability requirements does not silently reassign a Companion; Preflight must re-evaluate compatibility.

Retirement is the normal non-destructive removal path. A Role must have no active Companion assignment before it can be retired. Retired Roles remain available to historical records but cannot receive a new assignment, cannot execute through the Companion forwarder, and block/warn Preflight when referenced according to their required dependency.

Restoring a retired Role preserves the same `machine_role_id` and `role_key`; the operator must explicitly reassign a Companion and re-run Preflight.

Permanent deletion is intentionally narrow. It is allowed only after retirement and only when StageCore finds no assignment history, media requirement, execution-environment binding, live-video placement, Project target, or Runtime Snapshot reference. Otherwise deletion fails closed and the retired record remains.

## Assignment

One active Companion assignment per Machine Role in MVP.

States:

- `UNASSIGNED`
- `ASSIGNED`
- `SYNCING`
- `READY`
- `DEGRADED`
- `OFFLINE`
- `MISMATCH`
- `RELEASED`

## Replacement Flow

```text
Old VIDEO-MAIN offline/released
 -> pair new Mac
 -> assign VIDEO-MAIN
 -> sync configuration/media
 -> verify Snapshot + capabilities
 -> Preflight
 -> READY
```

Cue definitions do not change during this replacement.

## Capability Matching

Before READY, Hub checks that the assigned Companion can satisfy the role's required capabilities/plugins/local integrations. Unsupported requirements produce a clear blocker rather than best-effort silent execution.