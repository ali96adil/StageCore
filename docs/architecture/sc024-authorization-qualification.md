# SC-024 authorization qualification

[Route inventory](sc024-http-route-inventory.tsv) contains all 274 Hub
registration expressions with the 18 indirect patterns resolved to their
actual paths, plus the three standalone camera relay routes. GET registrations
also serve HEAD through Go's ServeMux. Patterns are registration records,
including options used only by qualification tests. This is a coverage
inventory, not proof that a route uses the prospective policy.

| Surface | Qualified behavior | Remaining qualification before cutover |
| --- | --- | --- |
| Projects and Show Capsule | Explicit alternate assembly qualifies list, detail, dashboard, rename, preview, export and Owner-only creation, duplication and global import. | The qualification route registry additionally checks cue/draft/publish, project configuration, execution environments and runtime intent before service entry. Qualified browser GO and Jump recheck authorization at each action dispatch. Atomic coupling to the adapter and remaining mutations still need review. |
| Project Files and browser Vault | Scoped paginated listing and version-bound attachment downloads in the browser. Shared bytes do not grant a foreign project version. Downloads recheck both grant and session per chunk. | Activation and remaining execution-environment media and ticket flows. |
| Companion Vault | Optional role, project and published Snapshot scope. Revocation closes an in-flight transfer. | Capture upload qualification checks the current device ticket before and after each bounded read. Cached capture receipts also match Companion connection, environment, Machine Role, Snapshot and operation. New browser-issued tickets also fence issuance, transfer, completion, and receipt replay to the issuing browser session. Qualification requests recheck the project runtime and media grants. Existing tickets retain their original scope. Deployment qualification remains open. |
| SSE | The current feed emits a session-ready event and keepalive, with a fresh session check before delivery. It has no project payload. | Any future project event publisher must resolve project ownership and check delivery grants before writing. |
| Device | Staged device metadata reads require current audited V2 assignment and live project.view. Hub-wide inventory diagnostics are Owner-only in that assembly. | Qualification reserves raw commands, transfers, setup and firmware mutations for Owner, and checks project configuration grants on mapped device edits. Qualified Cue dispatch rechecks the originating grant at the Stage Device socket write. Device operations outside that Cue path still need review. |
| Plugin and extensions | Qualification-only Owner boundary for the 28 Hub-global extension and plugin administration registrations. | Qualified Cue dispatch rechecks the originating grant before the OSC plugin pipe write. Other extension lifecycle operations remain subject to their route policies. |
| Audio | Optional StageAudio project, Vault version and SHOW fence. Live grant/session checks before commands and after sidecar replies. | Unbound legacy audio is Owner-only in qualification. Hardware-side revocation during a device operation remains open. |
| Camera relay | Optional standalone bearer authorization separates viewing from flash control. Active viewers are disconnected on credential rotation, and protected LAN listeners require TLS. | Hub project grants and authenticated browser/tablet delivery are still open. |

The source guard rejects wiring the alternate Project/Capsule/Files options
into the production Hub before the coordinated ACL cutover. Migration 53 is
unchanged, and the legacy assembly continues to serve previously created shows.


Every registered project-ID browser pattern in the inventory now has an
explicit qualification policy, including legacy memory, simulation,
preflight, visual and tablet routes. The opt-in router rejects new project-ID
patterns without a policy. Legacy registrations for project listing, device
metadata and unbound StageAudio inherit the qualification context. This
coverage includes the live grant check at the Companion, Stage Device, and OSC plugin Cue send boundaries. Other asynchronous dispatch paths and project-grant integration with Camera Relay still need qualification.
