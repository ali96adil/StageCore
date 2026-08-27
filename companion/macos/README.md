# StageCore macOS Companion

This directory contains the product Swift package for the first StageCore macOS Companion.

Current M4.2 boundary:

- `StageCoreCompanionCore` owns language-neutral Companion protocol models, runtime identity/state abstractions and bounded execution duplicate protection.
- The Hub remains authoritative for Project, Machine Role assignment and Runtime Snapshot truth.
- A reconnect never authorizes replay of a prior execution.
- The Companion rejects stale Runtime Snapshot, wrong Machine Role and unsupported capability requests before local execution.
- `companion_id` uses canonical UUID text to match Hub persistence.
- `stagecore-companion` is the headless SwiftPM executable bootstrap. It loads non-secret Hub URLs/display settings, obtains the Keychain device identity, completes pairing/authentication and starts the existing WebSocket agent lifecycle.

Security boundary:

- The production P-256 private device key and stable `companion_id` are created/recovered through macOS Keychain/Security APIs. The private key is non-exported by StageCore.
- Normal Companion configuration contains only non-secret Hub URLs, display name, version and config identity.
- Pairing is an explicit, expiring request approved through the Hub-local `stagecore-pairing` boundary. Reconnect uses a signed challenge and a short-lived in-memory runtime session credential.
- Product transport requires authenticated `wss://`/`https://`; insecure transport is limited to explicit loopback tests.

There is no polished SwiftUI/status window in this slice. Hub identity pinning UI, background launch packaging, real OSC/local adapters, media sync, signing and notarization remain later M4.2 work.
