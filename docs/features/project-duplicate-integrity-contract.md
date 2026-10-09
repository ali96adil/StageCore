# Duplicate Project — data-integrity implementation contract

Status: **specification and coverage gate; not an implemented duplicate API**. See #472.

## Semantics

The operator chooses an existing Project and edits a prefilled new name ("Original Copy").
The source is never mutated. The destination has a new Project ID and a new editable
DRAFT revision. Duplication is atomic: either the complete supported authoring graph
is committed or no destination Project exists. It is not an import of run history.

## Required source-to-destination mapping

| Graph boundary | Copy requirement |
| --- | --- |
| projects | Copy name only from new-name input, preserve description, new ID/timestamps |
| project_revisions | Copy selected authoring revision to independent DRAFT, new ID, provenance note |
| project_device_aliases | Copy alias settings/notes and target references, fresh IDs, no device lease |
| cues | Copy display labels, names, order, enabled, notes_summary, policies, fresh IDs |
| actions | Preserve order, capability, parameters, error/timeout policies, remap cue IDs |
| cuegroup links | Remap linked/composite Cue IDs nested inside execution_policy_json |
| input_definitions | Copy definitions with new IDs |
| output_definitions | Copy definitions with new IDs |
| routes | Copy conditions, delays, debounce, error policy, source input remapping |
| route_actions | Remap referenced route, output and Cue IDs; preserve order/payload |
| execution_environment_manifests | Copy canonical authoring configuration with new ID |
| execution_environment_snapshots | Copy immutable payload as new Project-specific identity |
| execution_environment_rebuild_plans | Copy plan and remap environment/snapshot references |
| project lighting/visual/tablet configuration | Include all revision-scoped authoring settings after schema inventory; refuse incomplete copies |
| media/role references | Preserve reusable media identity; explicitly decide whether Project-specific requirements are cloned, never silently rewrite physical machine ownership |

## Runtime/state that must NOT be cloned

No active Sessions, runtime_snapshots with original identity, Cue execution history,
pending commands, device assignment/epoch, paired-device keys, authentication/session
tokens, blackouts, physical outputs, extension installations or local Hub trust secrets.
A cloned Project must not publish, assign, GO, start a Session or send any output
automatically. Referencing a media asset never duplicates or deletes its bytes.

## Implementation rules

1. Inventory all tables referencing project_id/revision_id, including indirect
   references, BEFORE inserting anything; this list is an acceptance baseline,
   not a license to silently omit tables added since this document.
2. Snapshot source revision and Project metadata consistently.
3. Allocate new IDs and explicitly rewrite every internal foreign key and
   linked-Cue policy ID, refusing dangling references.
4. Write in ONE SQLite transaction; roll back on constraint/validation failure.
5. Return both new Project ID and revision ID only after commit.
6. Keep original Project unaffected even if a clone fails.
7. UI Duplicate action requires ProjectEdit; validate the new name and show that
   device bindings must be deliberately assigned before operation.
8. Keep SHOW restrictions on source mutations; cloning must never silently
   inherit a SHOW authority scope.

## Required acceptance tests

- Draft source + validated/published source; cloned target is DRAFT.
- Project description and every Cue note preserved byte-for-byte, including newlines.
- Nested linked/composite Cue references resolve exclusively to new IDs.
- Route actions target cloned inputs/outputs/Cues, not originals.
- Lighting, tablets, video/OSC, aliases, machine roles and execution environments
  retain their authoring configuration or the clone refuses clearly and atomically.
- Source revisions/snapshots/notes unchanged after successful clone and rollback.
- No active Session, device assignment, pending output or command transferred.
- Forced failure partway through transaction leaves zero destination rows.
- An unauthorised user cannot duplicate a Project.
- Duplicate of same source twice yields independent graphs, not shared editable
  rows or reused internal IDs.
- Stress source graph with large Cue counts and media references.

This contract deliberately makes it **incorrect** to ship a simple
CreateProject + copy-cues implementation as a full Project Duplicate.
