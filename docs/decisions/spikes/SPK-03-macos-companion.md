# SPK-03 — macOS Companion

**Status:** ACCEPTED for Companion Core + persistent-channel baseline  
**Scope:** macOS Companion implementation language, Hub channel, Machine Role/runtime reconciliation, duplicate protection and reconnect semantics  
**Validated by:** `prototypes/spk-03-macos-companion`

## Decision

The first StageCore macOS Companion will be implemented natively in **Swift**, with the transport/runtime logic isolated in a reusable **Swift Package** (`CompanionCore`) and the product setup/status interface provided by a **SwiftUI macOS app**.

The persistent Hub-to-Companion command/result channel uses versioned JSON messages over **WebSocket**. The production connection is authenticated and encrypted (`wss://`) according to the Security Model; the prototype uses plain loopback `ws://` only to validate transport/reconnect behavior.

Reference shape:

```text
StageCore Hub (Go)
      |
 authenticated WebSocket
      |
StageCore CompanionCore (Swift)
      |
 local capability adapters
      |
macOS apps / MIDI / OSC / automation / local files

SwiftUI app shell
      -> setup, pairing, role, health, sync, diagnostics
      -> controls/observes the same CompanionCore service
```

## Why Native Swift for the macOS Companion

The Companion is the StageCore component most tightly coupled to the local operating system. macOS integrations will eventually need native access to areas such as:

- Keychain/secure credentials;
- local application lifecycle and automation permissions;
- MIDI and other machine-local frameworks;
- filesystem/security-scoped access where applicable;
- background/login-item/service behavior;
- code signing, notarization and app distribution;
- a native SwiftUI setup/status experience.

Using Swift for this first platform avoids introducing a non-native agent plus a second native helper merely to reach macOS APIs. The Hub remains Go; the Companion contract remains language-neutral. A future Windows/Linux Companion can implement the same protocol in the technology appropriate to that platform.

## Package Boundary

The first macOS repository shape should keep these concerns separate even if they ship in one app bundle:

```text
StageCore for macOS
├── StageCoreCompanionCore     ← Swift package, no UI authority
├── macOS UI                   ← SwiftUI setup/status/client surfaces
└── local capability adapters  ← macOS-specific execution
```

`StageCoreCompanionCore` owns:

- Hub connection/reconnect state;
- Companion identity abstraction;
- role/snapshot synchronization state;
- capability declaration;
- execution request validation;
- execution result reporting;
- recent execution-ID duplicate guard;
- health/readiness observations passed to Hub/UI.

The SwiftUI shell must not own Cue logic or authoritative Project state.

## Persistent Channel Contract

The prototype validates this minimum message flow:

```text
companion.hello
 -> session.ready
 -> execution.request
 -> execution.result
```

### `companion.hello`

Carries at minimum:

- stable `companion_id`;
- Companion/agent version;
- platform;
- supported capabilities;
- last applied Runtime Snapshot identity when known.

### `session.ready`

The Hub returns the authorized Machine Role/runtime state relevant to this connection, including the expected Runtime Snapshot identity.

### `execution.request`

Carries at minimum:

- `execution_id`;
- `runtime_snapshot_id`;
- capability key;
- capability parameters.

### `execution.result`

Carries explicit status and, where applicable, acknowledgement/error fields. The Companion never turns a transport reconnect into permission to execute the previous Action again.

## Reconnect & Duplicate Rules

On connection loss:

1. Companion reconnects with the same stable identity.
2. It reports capability/applied-snapshot state again.
3. Hub reconciles before considering the role READY.
4. A request for a stale Runtime Snapshot is rejected with `SNAPSHOT_MISMATCH`.
5. A recently completed `execution_id` repeated after reconnect is rejected with `DUPLICATE_EXECUTION` rather than executed twice.
6. New valid executions can continue after reconciliation.

The production Hub remains responsible for not automatically replaying non-idempotent Commands. The Companion duplicate guard is an additional bounded protection, not a substitute for correct Hub behavior.

## Identity & Trust

The Companion has a stable identity independent of hostname/IP.

Production direction:

- asymmetric Companion identity from the Security Model;
- private material stored in macOS Keychain/appropriate protected storage;
- explicit pairing and revocation;
- trusted Hub identity/fingerprint verification;
- authenticated `wss://` channel.

The prototype `FileIdentityStore` is test scaffolding only and must not become the production secret/key store.

## Background Operation

The logical Companion service must not depend on a visible macOS window. The first implementation may package CompanionCore and the setup/status UI in one application while the product behavior is being proven.

Whether production background execution uses the main app lifecycle, a login item, or a separate helper/service is deliberately **not finalized by this spike**. That choice requires a real macOS/Xcode build and lifecycle test. Whichever packaging is chosen must preserve the same CompanionCore contracts.

## Prototype Evidence

The executable spike contains a Go Hub simulator and Swift 6 Companion package.

Repeatable validation completed:

- `swift test` passes identity persistence and execution-guard tests;
- `go test ./...` compiles the Go Hub simulator;
- Swift `URLSessionWebSocketTask` connects to the Go Hub WebSocket endpoint;
- Hub assigns `VIDEO-MAIN` + `snap-1`;
- `exec-1` executes exactly once;
- Hub intentionally closes the first connection;
- Companion reconnects with the same `companion_id`;
- replayed `exec-1` is rejected as `DUPLICATE_EXECUTION`;
- `exec-stale` is rejected as `SNAPSHOT_MISMATCH`;
- valid `exec-2` completes after reconnect;
- scenario ends with `SPK-03 PASS` / `TEST_COMPLETE`.

The Swift package was compiled/tested in the available Swift 6 environment. This environment is not macOS, therefore this spike does **not** claim validation of SwiftUI rendering, Keychain integration, ServiceManagement/background behavior, app signing or notarization yet.

## Acceptance Result

**ACCEPTED** for:

- Swift as the first macOS Companion implementation language;
- SwiftPM `CompanionCore` boundary;
- versioned JSON over persistent WebSocket for Hub/Companion runtime communication;
- stable identity abstraction;
- Machine Role + Runtime Snapshot reconciliation;
- duplicate/stale execution rejection semantics.

**Requires real macOS implementation validation later:**

- SwiftUI app bundle;
- Keychain-backed device credentials;
- background/login behavior;
- local macOS permission flows;
- signing/notarization/package download from Hub.

## Next Spike

**SPK-04 — Plugin Process / IPC** should validate how the Go Hub isolates external Plugin execution while preserving the capability contract already used by `osc.send`.
