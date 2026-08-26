# 06 — Plugin/Add-on Security & Permissions

## Install Does Not Equal Trust Everything

A Plugin manifest declares permissions before activation. StageCore shows and records the grant decision. Installing/storing a package does not automatically give it network, filesystem, local-control or secret access.

Reference permission families include:

- `network.udp.send`
- `network.tcp.connect`
- `filesystem.project.read`
- `filesystem.plugin-data.write`
- `local.midi.send`
- `local.application.control`
- scoped `secret.use:<secret-ref-or-class>` where required.

## Least Privilege

Plugins receive only permissions needed by declared capabilities. A Plugin cannot create new privileged extension points at runtime or bypass Hub authorization/safety gates.

## Process Isolation

I/O/vendor/parser Plugins run outside the critical Core process by default. The Core uses versioned bounded IPC, timeouts and health supervision.

A Plugin crash or malicious request must not grant Project authority or crash the Cue Engine directly.

## Package Integrity

Before activation StageCore verifies package bytes against repository checksum/manifest identity.

Production first-party packages should support signature verification against an approved StageCore publisher key. Explicit development mode may allow unsigned local builds, but the UI/status must label them as development/unverified rather than production-trusted.

Third-party signing/marketplace PKI is deferred.

## Secret Access

Plugins cannot enumerate/export all Vault secrets. When a capability needs a credential, the Project/plugin configuration references a specific secret and Core checks both Plugin permission and user/project authorization before supplying it.

Returned errors/logs are passed through redaction rules.

## Network Access

A generic network permission does not grant Hub administrative API authority. Plugin-to-Core communication uses its dedicated authenticated/isolated host channel.

## Disable / Revoke

Removing a Plugin permission or disabling the Plugin prevents new executions requiring it. Existing Project configuration remains preserved as unavailable and Preflight reports required capability blockers.

## SHOW Mode

Plugin install, uninstall, permission expansion, incompatible upgrade and configuration migration are blocked during SHOW in v0.1.