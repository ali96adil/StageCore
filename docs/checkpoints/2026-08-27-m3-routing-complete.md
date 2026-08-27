# StageCore — M3 Routing Completion Checkpoint

**Date:** 2026-08-27  
**Implementation slice:** M3 — Routing  
**Status:** COMPLETE  
**Merged product commit:** `7f573c32d8e8e8bbad151105025900869cf8eee5`  
**Pull request:** #16  
**Tracking issue:** #15 — closed as completed

## 1. Completion Decision

M3 is accepted as complete.

StageCore now has one deterministic Routing runtime shared by Test and supported OSC inputs. Routes evaluate only definitions captured by the active immutable Runtime Snapshot, apply bounded conditions/transforms/debounce, persist Route Trace evidence, and dispatch either Cue commands or generic capability Actions without bypassing the existing runtime/safety boundary.

**Decision:** `M3 COMPLETE — M4 COMPANION + MACHINE ROLE IS NEXT`.

## 2. Delivered Runtime Path

```text
Test / OSC input
→ typed input authority
→ Runtime Snapshot Route lookup
→ condition / transform / debounce
→ route trace
→ Cue dispatch OR Output capability dispatch
→ existing capability registry
→ explicit result + history
```

Delivered evidence includes:

- deterministic equals/not-equals/numeric/boolean condition handling;
- disabled/non-matching Route no-dispatch behavior;
- deterministic debounce;
- Route -> Cue through the normal validated `cue.go` path;
- Route -> Output through the generic capability registry;
- real Route -> `osc.send` UDP path with truthful `TRANSPORT_ONLY` acknowledgement;
- persistent Route Trace and explicit failure history;
- route-atomic manual Test safety preflight for CRITICAL/SAFETY_CRITICAL targets;
- typed input authority so free-form issuer text cannot spoof OSC origin;
- external OSC receive process remains outside Hub Core;
- malformed OSC isolation and bounded process lifecycle;
- duplicate/restart/no-replay preservation;
- implementation-level pre-adapter routing p95 evidence against the <=20 ms target.

## 3. Verification Evidence

Final reviewed head:

`378018f53cdab9f82761544b6b73ef692ba2a2cf`

Final reviewed tree:

`513e96c5a4520680b81b07d7c9645dda1eac2794`

Pre-merge Core CI:

- run `32989581177` / #41 — PASS;
- Go 1.26/1.27 module lock, tests and vet — PASS;
- Go 1.26 race and Linux ARM64 CGo-free product builds — PASS.

PR #16 was squash-merged to `main` as:

`7f573c32d8e8bbad151105025900869cf8eee5`

The squash commit tree is byte-identical to the final reviewed/tested tree. GitHub did not create a new post-merge push check-suite for that exact squash event; this Actions delivery artifact is recorded in Issue #15 and was not hidden by inventing a code change.

## 4. Boundary Preserved

M3 did not silently pull in Companion trust/runtime, Vault/media workflows, non-loopback Stage LAN control, Operator UI, Hardware Nodes, full DMX/lighting, AI/Vision or HA/cloud work.

## 5. Transition

M3 handed the stable logical-target and runtime-dispatch boundary to M4. A Machine Role replacement therefore can remain independent of Cue/Route definitions instead of introducing a second routing model.
