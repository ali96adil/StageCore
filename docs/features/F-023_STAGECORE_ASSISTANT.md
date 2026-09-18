# F-023 — StageCore Assistant / Natural-Language Show Builder

## Status

Phase 7 software implementation through Slice F2 is merged on `main`. Slices A–E provide the bounded Assistant contract, evidence-grounded diagnostics, editable Draft proposals, bilingual/RTL Operator workspace, and rehearsal/simulation assistance with explicit no-live-output authority. Slice F2 adds deterministic provider-absent fail-closed regression coverage. Software freeze closeout is pending only the tracker-required exact-main CI PASS evidence; GitHub connector visibility has not exposed a run/status for the current candidate SHA. Physical/product qualification remains deferred under Issue #148.

## Product goal

StageCore Assistant is an optional advisory, drafting, rehearsal-analysis, and diagnostic layer. It helps an operator understand canonical StageCore evidence and prepare editable configuration proposals without becoming show-control authority.

Representative questions and tasks include:

- explain why a cue or action failed;
- summarize readiness or Preflight problems;
- analyze rehearsal or timing evidence;
- draft cue groups, routing/device mappings, checklists, notes, or templates;
- prepare bounded configuration proposals for operator review.

The normal manual/offline StageCore path remains fully functional when no AI/model provider is configured or reachable.

## Non-negotiable authority boundary

The Assistant never owns or proxies live execution authority.

The only Assistant authority classes are:

- `READ_ONLY` — explanation and diagnosis from supplied canonical evidence;
- `DRAFT_PROPOSAL` — generation of a structured proposal that still requires the normal StageCore Draft/validation/apply/publish flow.

There is intentionally no `LIVE_EXECUTION`, `GO`, emergency, panic, blackout, runtime-command, or equivalent authority class in the Assistant contract.

The Assistant must never:

- press or proxy `GO`;
- issue emergency, blackout, panic, or other safety-critical runtime commands;
- bypass RBAC or F-012 SHOW structural-mutation locks;
- bypass pairing/trust or Command Envelope authority;
- mutate immutable Runtime Snapshots;
- change a published revision directly;
- silently apply an irreversible configuration change;
- fabricate capability, health, readiness, latency, timing, or execution evidence;
- create a parallel project/history/analytics truth source.

## Versioned contract

Slice A defines `assistant` contract version `1`.

Request kinds:

- `EXPLAIN`
- `DIAGNOSE`
- `DRAFT`

`EXPLAIN` and `DIAGNOSE` require `READ_ONLY` authority. `DRAFT` requires `DRAFT_PROPOSAL` authority.

Draft proposals are deliberately constrained to non-executing structural proposal kinds:

- `CUE_DRAFT`
- `ROUTING_DRAFT`
- `DEVICE_MAPPING_DRAFT`
- `CHECKLIST_DRAFT`
- `NOTE_DRAFT`
- `TEMPLATE_DRAFT`

A proposal records a `base_revision_id`. Later slices must reject stale proposals when the canonical Draft/revision baseline has changed.

## Canonical context boundary

The Assistant does not receive an arbitrary database dump or unrestricted application state. Context is assembled from an allowlist of summarized canonical evidence classes:

- Project and Revision summaries;
- Cue and Routing summaries;
- Stage Device and Device Profile summaries;
- Machine Role and LiveSource summaries;
- Runtime Snapshot summaries;
- Preflight findings;
- Doctor findings;
- Flight Recorder evidence;
- F-028 timing evidence;
- F-024 simulation evidence.

Raw Secret Store records are not an allowed context class. Credentials, tokens, passwords, private keys, and other secret values are not Assistant context.

Before a context bundle is handed to a provider, the assembler uses a narrow `Redactor` interface compatible with the existing `secretstore.Service.RedactString` behavior. This preserves the existing StageCore secret authority rather than introducing a second secret store or redaction database.

Authorization must be applied before context assembly. A provider receives only evidence the authenticated StageCore caller is allowed to inspect.

## Provider boundary

Model/provider access is replaceable behind the narrow Go `Provider` interface:

```go
type Provider interface {
    Complete(context.Context, Request) (Response, error)
}
```

StageCore project semantics, RBAC, revision authority, SHOW locks, validation, audit, and apply/publish behavior stay outside provider implementations.

CI and deterministic correctness tests use `FakeProvider`, which performs no network access and returns only explicitly registered responses. Correctness must never depend on an Internet model service.

A future real provider adapter may be local or remote. Adding a provider must not expand the Assistant authority contract.

## Evidence grounding

Assistant responses can reference canonical evidence IDs. Later diagnostic slices must expose the evidence used for explanations and distinguish:

- observed evidence;
- assumptions;
- missing context;
- stale or unavailable evidence.

When evidence is absent or insufficient, the correct behavior is to say that evidence is missing rather than invent a cause.

## Draft lifecycle

Slice A only defines proposal structure. Future mutation integration must follow this sequence:

1. collect authorization-scoped canonical context;
2. request a `DRAFT_PROPOSAL`;
3. show the exact proposed changes to the operator;
4. confirm the proposal still targets the current Draft/revision baseline;
5. require an authorized explicit Apply action;
6. translate the accepted proposal into ordinary StageCore Draft mutations;
7. reuse existing validation, SHOW locks, audit/security events, and publish flow.

The provider itself never receives a direct mutation or runtime execution handle.

## Critical-path isolation

Assistant/model work is noncritical background/operator work. It must not sit on GO/P0/P1 execution paths or add provider latency to live command dispatch.

## Slice A acceptance

Slice A is accepted when:

- versioned request/response/proposal contracts exist;
- only `READ_ONLY` and `DRAFT_PROPOSAL` authority classes exist;
- request validation prevents authority escalation;
- draft operation kinds cannot encode GO/blackout/runtime-command authority;
- context uses an explicit canonical allowlist;
- context construction supports the existing Secret Store redaction boundary;
- a replaceable Provider interface exists;
- a deterministic no-network fake provider exists;
- tests prove read-only responses cannot smuggle mutation proposals and unsupported live authority fails closed.

Slice A does not add HTTP endpoints, Operator UI, persistence schema, provider credentials, real model network access, or any runtime command path.

## Implemented slices

- Slice A — versioned Assistant contract, safety boundary, canonical context/redaction, and deterministic provider seam.
- Slice B — evidence-grounded read-only diagnostics and explanations from canonical StageCore evidence.
- Slice C — editable Draft proposal preview/apply flow with explicit operator authority, stale-baseline protection, SHOW locks, RBAC, and audit reuse.
- Slice D — authenticated bilingual/RTL Operator Assistant workspace with explicit Apply/Discard and provider-offline independence.
- Slice E — F-024/rehearsal assistance grounded in SIMULATION, Flight Recorder, and advisory F-028 evidence; deterministic acceptance tests prove troubleshooting remains read-only, preparation remains Draft-only, and SIMULATION context cannot acquire real render/device/live execution authority.

## Slice F — software qualification / freeze

Slice F is closeout work, not a new Assistant execution feature. The freeze may be declared only when:

- promoted-slice deterministic Core CI evidence is PASS;
- Companion/Android CI is required only if their code or contracts changed;
- deterministic safety tests retain the no-GO/emergency/safety-critical authority boundary;
- normal offline/manual StageCore operation remains independent of Assistant/provider availability;
- required exact-main CI is verified PASS on the final freeze SHA;
- this document and the Phase 7 tracker identify the exact frozen software SHA truthfully.

Current software candidate: `98592c3be71f586346951c3be933676c2f2e37c6`. Its merged F2 change has exact-head Core CI #1015 PASS, and the provider-absent regression proves Assistant diagnostics fail closed before evidence/runtime collection. The required post-merge exact-main CI result for this candidate is not currently visible through the connected GitHub evidence surface, so that gate remains explicitly pending rather than inferred.

Until the exact-main CI gate is evidenced PASS, Phase 7 is not described as software-frozen. Issue #148 remains the separate cumulative physical/product qualification gate.

## Qualification boundary

Software CI may qualify Phase 7 as software-ready while Issue #148 remains active. Full product qualification must later verify Assistant safety, authority, offline/manual fallback, and operator behavior on the final cumulative deployed candidate.
