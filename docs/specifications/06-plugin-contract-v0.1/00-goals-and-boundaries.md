# 00 — Goals & Boundaries

## Goals

The Plugin Contract must make extensions feel native to StageCore while keeping the Core stable.

A plugin may contribute one or more of the following:

- capabilities/actions;
- routing inputs or outputs;
- device/endpoint types;
- configuration schemas;
- validation and Preflight checks;
- status/health information;
- log/trace enrichments;
- native UI contributions at approved extension points;
- project/runtime requirements.

## Non-Goals for v0.1

- arbitrary replacement of the StageCore UI shell;
- unrestricted code injection into the critical Core process;
- plugin marketplace, billing or remote catalog;
- arbitrary cross-plugin shared state;
- plugin-defined authentication system;
- plugin ability to bypass Core permissions, runtime snapshot or safety gates.

## Separation of Responsibility

**Core owns:** Project state, Runtime Snapshot, Cue/Route semantics, permissions, priority, safety gates, plugin registry, UI composition, execution IDs, result tracking and audit.

**Plugin owns:** protocol/device-specific validation, adapter logic, encoding/decoding, bounded I/O, plugin-local health and capability-specific configuration.

A plugin must not become a second source of truth for the Project.
