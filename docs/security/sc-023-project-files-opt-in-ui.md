# SC-023 / SC-024 Phase 4B — Opt-in read-only Project Files UI

**DRAFT ONLY.** Stacked on #525. No `main` merge, no Pi deployment,
no active SHOW change, and no existing Hub route / media flow rewritten.

## User experience

With the future opt-in installed and live `project.view` +
`media.read` granted by an enabled Owner:

- A **Files / الملفات** button appears within the selected Project;
- The read-only list shows logical Media Asset/Version records from the
  *same* Project, categories Audio/Video/Image/Other, names, sizes and
  storage policy; unrecognized formats are listed as Other;
- A Project with no media receives an empty view. Renaming a Project
  changes its display name, not its immutable ProjectID or Vault object
  paths. A copied Project has its own Media Asset/Version references;
- A new Project or account without media permission sees **no Files
  button**. Revoking permission makes the endpoint return the same 404 as
  an unknown Project and hides Files at the next probe/read;
- There is no upload, delete, rename, file picker, download or raw Vault
  link in this UI. Text from filenames is HTML-escaped.

## New API (explicit opt-in only)

`httpapi.WithScopedProjectFilesRead(auth, projectaccess)` is a standalone
server option, **not enabled by default** in any Hub assembler.

- `GET /api/v1/projects/{project_id}/files/availability` → 204 with
  current Project's `media.read` grant, otherwise 404 without Project
  metadata (401 unauthenticated, 426 insecure transport).
- `GET /api/v1/projects/{project_id}/files` → JSON
  `{workspace:{project_id,project_name,files:[...]}, read_only:true,
  scoped:true}`. No source filesystem paths or download credentials.
- Return 409 `PROJECT_FILES_PAGINATION_REQUIRED` instead of silently
  truncating when the unpaged workspace exceeds 500 entries. Paginated
  API and stable sorting are required before broad rollout.
- Both routes validate the browser session, enforce TLS/loopback,
  and rely on live SQLite grants. All foreign/missing Project lookups
  return indistinguishable 404s. `Cache-Control:no-store` on successful
  responses; eventual deployment should also apply this to error paths.
- The UI first probes availability for exactly the currently selected
  Project, hides its navigation on any error, and discards stale responses
  from Project-switch races.

## Why opt-in

Existing `/api/v1/projects`, `/api/v1/show-capsules`, Vault raw-hash
Companion download, Cue/runtime/GO and other endpoints still use the old
global role checks. Advertising the entire Hub as Project-private on the
strength of this new read-only route would be false.

Do **not** enable the new option for ordinary users until all sibling
Project, direct-ID, download and event routes are scoped atomically
(or an approved isolated pilot has demonstrated no cross-route leaks).

Any caller wiring the option must supply the *same* auth and projectaccess
DB used by the session and Owner-only grant APIs. It must not fall back
to a generic project.read role or a client-submitted identity.

Next phases: comprehensive capsule imports + owner restore policy, Event
stream isolation, accessible cursor pagination, media upload/cleanup API,
full grant migration dry run, all-route cutover with rollback, and attended
non-SHOW hardware tests. Do not enable SHOW-time privilege changes.
