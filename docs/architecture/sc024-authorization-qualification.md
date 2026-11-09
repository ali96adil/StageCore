# SC-024 authorization qualification

[Route inventory](sc024-http-route-inventory.tsv) contains all 274 Hub
registration expressions with the 18 indirect patterns resolved to their
actual paths, plus the three standalone camera relay routes. GET registrations
also serve HEAD through Go's ServeMux. Patterns are registration records,
including options used only by qualification tests. This is a coverage
inventory, not proof that a route uses the prospective policy.

| Surface | Qualified behavior | Remaining qualification before cutover |
| --- | --- | --- |
| Projects and Show Capsule | Explicit alternate assembly qualifies list, detail, dashboard, rename, preview, export and Owner-only creation, duplication and global import. | Other project/revision/cue/session mutations and in-flight revocation across multi-stage writes. |
| Project Files and browser Vault | Scoped listing and version-bound attachment download. Shared bytes do not grant a foreign project version. Downloads recheck both grant and session per chunk. | Browser presentation and pagination; remaining execution-environment media and ticket flows. |
| Companion Vault | Optional role, project and published Snapshot scope. Revocation closes an in-flight transfer. | Rollout against real Companion provisioning and separate capture-upload review. |
| SSE | The current feed emits a session-ready event and keepalive, with a fresh session check before delivery. It has no project payload. | Any future project event publisher must resolve project ownership and check delivery grants before writing. |
| Device | Staged device metadata reads require current audited V2 assignment and live project.view. Hub-wide inventory diagnostics are Owner-only in that assembly. | Apply live browser grants to setup, maintenance, provisioning and actuation. Test scope changes during operations. |
| Plugin and extensions | Existing Hub-level RBAC. | Distinguish global installation/trust administration from any project use, and qualify ownership of project execution contexts. |
| Audio | Optional StageAudio project, Vault version and SHOW fence. Live grant/session checks before commands and after sidecar replies. | Legacy audio routes and revocation while a device operation is in flight. |
| Camera relay | Separate local service with its own health, stream and flash endpoints. | Qualify project viewing and control at the Hub/relay boundary; an inventory entry does not confer browser authority. |

The source guard rejects wiring the alternate Project/Capsule/Files options
into the production Hub before the coordinated ACL cutover. Migration 53 is
unchanged, and the legacy assembly continues to serve previously created shows.
