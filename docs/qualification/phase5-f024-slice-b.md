# Phase 5 — F-024 Slice B Qualification Gate

Candidate branch: `phase5/f024-digital-twin-state`

Slice B is software-qualified only when exact-head CI proves:

- module lock unchanged;
- unit/integration tests pass on supported Go matrix;
- race tests pass on the primary Go job;
- Linux ARM64 CGo-free product builds pass;
- configured Digital Twin faults cannot start physical output;
- canonical Session ownership prevents caller-supplied Session identity from redirecting Twin state;
- Cue and Routing share the same application Digital Twin runtime;
- no Raspberry Pi deployment is required under #148.

The slice remains part of F-024 implementation-in-progress. RANGE/CHECKPOINT semantics, persistent scenario presets, operator Simulation workspace, selected input/sensor injection, and simulation reporting remain later slices.
