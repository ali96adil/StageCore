# F-025 — VDMX Workstation Inspection Provider

**Status:** Phase 3 implementation slice
**Feature ID:** F-025
**Adapter key:** `stagecore.adapter.vdmx`
**Builds on:** authenticated workstation inspection transport, Execution Environment Manifest v1, F-025 readiness evaluation

## Purpose

This slice adds the first production workstation inspection provider to the macOS Companion. It translates bounded VDMX6 / VDMX6 Plus workstation facts into the existing engine-neutral `executionenv.Observation` model.

The provider is inspection-only. It does not launch VDMX, automate its interface, send OSC/MIDI, modify projects, or gain execution authority.

## Application detection

The provider checks only these known application bundle locations, in deterministic order:

1. `/Applications/VDMX6 Plus.app`
2. `/Applications/VDMX6.app`
3. `~/Applications/VDMX6 Plus.app`
4. `~/Applications/VDMX6.app`

No recursive application scan, Spotlight query, shell command, or arbitrary filesystem search is performed.

When a bundle is present, the provider reads its local `Contents/Info.plist` and reports `CFBundleShortVersionString`, falling back to `CFBundleVersion`. If version metadata cannot be read, application presence remains true but version compatibility remains unknown rather than being fabricated.

VDMX6 is currently distributed for Intel and ARM64 Macs, and VIDVOX documents macOS 12 as the minimum platform for VDMX6. StageCore itself continues to use the stricter Companion platform requirement defined by its Swift package.

## Version-constraint grammar

Version grammar is adapter-owned, as required by the F-025 contract.

This provider supports:

- `6.x-tested` — verifies the detected application bundle is a VDMX6 generation bundle;
- exact dotted numeric versions such as `1.3.4`;
- minimum dotted numeric versions such as `>=1.3.4`.

Unknown version-constraint strings produce `version_constraint_satisfied = null`. The Core readiness evaluator therefore cannot turn an unverified version into a false PASS.

## Declared asset inspection

The provider inspects only locators explicitly present in the supplied Execution Environment Manifest.

For `CONTENT_BOUND` assets:

- the locator must be an absolute filesystem path or `file://` URL;
- the declared path must exist and must not resolve through a symbolic link;
- directories are not content-hashed;
- regular files are read in 1 MiB chunks;
- SHA-256 is calculated incrementally with CryptoKit;
- observed byte size and SHA-256 are returned to the existing readiness evaluator.

Large media files are therefore never loaded fully into memory by inspection.

For `REFERENCE_ONLY` assets:

- only presence of the exact declared locator is reported;
- StageCore does not claim checksum integrity or backup coverage;
- the existing readiness evaluator preserves the reference-only portability warning.

## Unsupported requirement scope

This first VDMX provider does not yet claim truthful probes for VDMX extensions or external bindings.

If a VDMX manifest contains `external_extensions` or `bindings`, inspection returns:

`FAILED / VDMX_INSPECTION_SCOPE_UNSUPPORTED`

It does not guess that the requirement is present or absent. Future slices may add narrow VDMX-specific probes for these requirement types after their stable discovery contracts are defined.

## Security and privacy boundary

The provider grants no authority for:

- process or shell execution;
- launching, quitting, restarting, or controlling VDMX;
- AppleScript, Accessibility, or UI automation;
- broad `/Applications`, home-directory, or Application Support scanning;
- following symlinked declared assets to another location;
- copying, replacing, deleting, restoring, or rewriting files;
- installing VDMX, plugins, ISF packages, Vuo, TouchDesigner components, or licenses;
- transmitting workstation file contents to the Hub.

Only observation facts and hashes needed by F-025 readiness leave the provider.

## Companion integration

On macOS, `CompanionBootstrap` registers exactly one production provider for this slice:

`VDMXInspectionProvider` → `stagecore.adapter.vdmx`

The provider uses the authenticated `inspection.request` / `inspection.result` transport introduced by the preceding F-025 slice. It remains separate from `CompanionCapabilityExecutor`, Cue execution, OSC, and MIDI authority.

## Verification

Acceptance requires macOS tests proving:

- VDMX6 Plus application detection and version extraction;
- VDMX6 generation, exact-version, and minimum-version compatibility;
- unknown version grammar stays unverified;
- missing VDMX is reported as absent;
- content-bound files return streaming SHA-256 and observed size;
- reference-only locators report presence without content claims;
- symlinked declared assets are not followed;
- extension/binding requirements fail explicitly as unsupported;
- the real macOS Companion executable builds with the provider registered;
- Companion Core CI and real macOS replacement/media acceptance pass on the exact PR head.

## Deferred

- VDMX extension / ISF / Vuo / TouchDesigner component inspection;
- display, MIDI, audio, and network binding probes;
- VDMX process-running state;
- project parsing or VDMX internal workspace introspection;
- launch/open/reconnect controls;
- automatic SHOW Preflight wiring and Operator UI.
