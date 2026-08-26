# StageCore macOS Companion

This directory contains the product Swift package for the first StageCore macOS Companion.

Current M4.2 boundary:

- `StageCoreCompanionCore` owns language-neutral Companion protocol models, runtime identity/state abstractions and bounded execution duplicate protection.
- The Hub remains authoritative for Project, Machine Role assignment and Runtime Snapshot truth.
- A reconnect never authorizes replay of a prior execution.
- The Companion rejects stale Runtime Snapshot, wrong Machine Role and unsupported capability requests before local execution.
- `companion_id` uses canonical UUID text to match Hub persistence.

Security boundary:

- This package does **not** persist private credentials in a normal file.
- Production identity keys/credentials must use macOS Keychain or another Security-Model-approved protected store.
- Unauthenticated Stage LAN WebSocket exposure is not enabled by this package. Product transport must use authenticated `wss://`; loopback `ws://` remains test-only until the security pairing baseline is implemented.

The SwiftUI app shell, Keychain implementation, real WebSocket agent transport and macOS-local capability adapters are subsequent M4.2 slices.
