# F-015 — Supervised Runtime Lifecycle v1

## Scope

This slice turns a verified isolated runtime probe into a durable, supervised Plugin lifecycle without widening the F-015 trust boundary.

## State model

Operator intent and observed process state are deliberately separate:

- desired state: `DISABLED` or `ENABLED`;
- observed state: `STOPPED`, `STARTING`, `READY`, or `FAILED`;
- each explicit enable/disable transition increments a persisted generation;
- observed-state writes are compare-and-set against that generation so a stale process watcher cannot overwrite a newer operator decision.

A Hub shutdown stops child processes but does not rewrite `ENABLED` intent. On the next Hub start, enabled intent is reconciled through the same isolation and handshake gates.

## Enable path

Before persisting `ENABLED`, StageCore requires the existing bounded, hash-bound isolated runtime probe to pass. This means a known blocker such as `RUNTIME_NETWORK_BROKER_REQUIRED` cannot leave misleading enabled intent behind.

After intent is persisted:

1. the immutable installed payload is re-verified;
2. a fresh transient executable copy is created under the managed runtime root;
3. Bubblewrap isolation is planned again from current readiness and permission state;
4. the transient executable is launched only through that isolation plan;
5. `plugin.ready` must exactly match extension ID, version, protocol schema, and declared capabilities;
6. the transient file hash is re-verified;
7. observed state becomes `READY` for the same generation;
8. a supervisor waits for process exit.

Unexpected exit changes only the matching generation to `FAILED` with `RUNTIME_PROCESS_EXITED` and removes the transient executable.

## Disable path

Disable is an explicit configuration transition and remains blocked during active `SHOW`, consistent with other F-015 mutation gates.

A successful disable:

- persists `DISABLED` with a new generation;
- removes the supervised process from the active generation;
- stops the child process;
- removes its transient executable;
- reports observed `STOPPED`.

Any delayed watcher from the previous generation is unable to overwrite the new state.

## Restart reconciliation

At Hub startup, explicitly enabled installations are retried. Each retry still requires:

- activation readiness;
- immutable payload integrity;
- static Linux ELF compatibility;
- Bubblewrap availability;
- no unsupported direct `network.*` permission;
- a fresh bounded isolated probe;
- a fresh persistent `plugin.ready` handshake.

A restore failure is persisted as `FAILED` but does not prevent the Hub itself from starting.

## API

- `GET /api/v1/extensions/installations/{installation_id}/runtime`
- `POST /api/v1/extensions/installations/{installation_id}/enable`
- `POST /api/v1/extensions/installations/{installation_id}/disable`

Read access uses project-read permission. Enable/disable require plugin-manage permission and emit security-audit events.

## Security boundary retained

This slice does **not** add host networking to installable extensions. Approved `network.*` permissions still fail closed at `RUNTIME_NETWORK_BROKER_REQUIRED`. The next dependency-first slice is a StageCore-owned broker for explicitly scoped network operations.
