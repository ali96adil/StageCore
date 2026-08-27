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