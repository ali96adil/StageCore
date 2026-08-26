# Documentation Source Policy

The active StageCore engineering source of truth is the Markdown documentation in:

- `docs/baseline` — product vision and long-lived scope boundaries;
- `docs/adr` — approved architectural decisions and invariants;
- `docs/architecture` — system architecture and responsibility boundaries;
- `docs/specifications` — executable product, data, contract, security, storage and reliability requirements;
- `docs/decisions` — accepted implementation technology decisions validated by spikes;
- `docs/checkpoints` — dated consistency reviews that record the resolved interpretation and implementation-entry state without silently rewriting older documents.

## Conflict Resolution

Use the most specific applicable document, but preserve these rules:

1. A newer explicit ADR, superseding specification, accepted decision spike or dated checkpoint resolution may clarify an older document.
2. Decision spikes select implementation technology; they do not silently weaken product, safety, security or reliability requirements.
3. Older architecture/specification text may continue to list a technology as open after a later accepted spike resolves it. The later accepted decision is authoritative for that technology choice.
4. If two active documents conflict on behavior or authority and no explicit resolution exists, treat it as a documentation defect and resolve it before implementing the affected slice.
5. Prototype code under `prototypes/` is evidence for a decision, not production StageCore source.

Original Word documents are retained outside the active engineering flow and can be archived separately when needed. Markdown remains the working source for routine engineering, reviews and Codex/AI reads because it is small, diffable and version-controlled.
