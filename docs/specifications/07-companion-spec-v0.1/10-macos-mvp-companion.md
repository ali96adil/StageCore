# 10 — macOS MVP Companion

## First Platform

macOS is the first practical Companion target. The contract remains platform-neutral so Windows/Linux agents can be added later.

## MVP Product Shape

A practical first build may be a single macOS app containing the Companion agent and a small setup/status UI. The logical Companion service boundary must remain clear even if packaged together.

Minimum UI/status:

- Hub connection/discovery;
- Pairing request/code status;
- Companion identity/name/version;
- assigned Machine Role;
- READY/DEGRADED/OFFLINE state;
- current Snapshot/config sync status;
- required media status;
- diagnostic connection log without secrets.

## Background Operation

Production direction: the Companion agent can continue running without an open operator window, using normal macOS background/service mechanisms appropriate to the final implementation. The MVP prototype may start simpler, but Cue semantics must not depend on a visible window.

## Local Permissions

When a capability needs macOS permissions (for example local automation, MIDI, file access), the Companion exposes a clear readiness check and setup guidance. It must not report READY when a required permission is missing.

## Distribution

The Hub Web UI can host/download the compatible macOS package from its Software Repository. Release builds should be code-signed and notarized before normal field use.

## Update Rule

- show installed vs Hub-compatible version;
- allow explicit download/install outside SHOW;
- never force/update/restart the Companion during SHOW;
- incompatible version becomes WARN/BLOCK according to API compatibility.

## MVP Definition

The macOS build is successful when one clean Mac can install, pair, receive `VIDEO-MAIN`, execute a local/OSC capability, disconnect/reconnect safely and be replaced by another paired Mac without Cue edits.