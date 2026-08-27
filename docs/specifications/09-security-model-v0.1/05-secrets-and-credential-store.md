# 05 — Secrets & Credential Store

## Separation from Project Data

Passwords, API tokens, private keys and similar credentials are stored in a dedicated StageCore Secret Store, not directly inside Cue/Route/Plugin configuration and not in the normal Vault object tree.

Project configuration references a secret by stable `secret_id`/logical reference.

Example:

```text
projector-control
 -> credential_ref: secret:pjlink-main-password
```

## Storage at Rest

Secrets are encrypted at rest using a random Hub master encryption key. The master key is protected separately from the encrypted secret records using the strongest practical platform mechanism.

Reference deployment:

- macOS development Hub: Keychain where available;
- Linux/appliance Hub: protected local key material with restrictive filesystem permissions; TPM/secure hardware integration can be added later.

The exact crypto library/format must use maintained standard primitives; StageCore must not invent custom encryption algorithms.

## Access Rules

- only authorized Core services/users can create/update/delete secrets;
- Plugins request declared/scoped access rather than receiving the entire secret store;
- a Plugin that needs a credential receives only the specific permitted secret for the operation/configuration;
- secret values are redacted in UI after entry unless explicit reveal permission/action exists;
- secret access metadata may be audited, but the value is never written to audit logs.

## Logging & Errors

Secrets must be redacted from:

- normal logs;
- Event/Command diagnostic payloads;
- stack traces shown to users;
- Project exports;
- Git repository/config examples;
- Plugin health/status output.

## Backup

Ordinary Project backup/export does not include decryptable global secrets by default.

A Full/System Recovery backup may contain a separately encrypted secret bundle with explicit operator intent and recovery protection. Restore must not silently replace active security material.

## Rotation

Updating a secret creates a new current value without requiring Cue definitions to change because Cues reference the logical secret/target configuration. Rotation outside SHOW is preferred; changes affecting active runtime are validated before they become Show Ready.