# F-015 — Dependency Install Plan v1

**Status:** Implementation slice on `f015/dependency-install-plan` pending final CI/merge.  
**Scope:** Read-only dependency resolution and lifecycle-safe prerequisite gating for extension installation.  
**Not yet:** automatic multi-package plan execution, permission approval, activation, health/readiness, update/rollback, repair/remove, catalogs, or Operator Manager UI.

## Goal

Make extension dependencies truthful and actionable before StageCore adds activation or one-click lifecycle controls.

A registered manifest may already declare required and optional dependencies. This slice turns those declarations into a deterministic install plan and prevents the normal lifecycle installation path from materializing a root package while required dependencies are absent or incompatible.

## Safety boundary

This slice does not activate or execute any extension.

The existing low-level `Installer.Install` remains the verified storage materializer used by crash/idempotency tests and by future ordered plan execution. The lifecycle-safe entrypoint is `Installer.InstallPlanned`, which:

1. enforces the authoritative SHOW mutation lock before planning;
2. computes a read-only dependency plan;
3. refuses the root install when required prerequisites are not already installed;
4. refuses blocked/conflicting plans;
5. delegates to the existing verified materializer only when the plan is `READY`.

The Operator API uses only `InstallPlanned` for installation.

## Plan states

### `READY`

All required dependencies are already installed at compatible versions. The root package can be installed immediately, or is already installed.

### `REQUIRES_DEPENDENCIES`

The dependency graph is resolvable from registered compatible Library packages, but one or more required dependencies must be installed first.

The returned steps are dependency-first and end with the root package when the root is not already installed.

### `BLOCKED`

StageCore cannot produce a safe plan without an explicit corrective action. Examples include:

- a required dependency is absent from the Library;
- no compatible Library candidate satisfies the version constraints;
- multiple required paths create an impossible version range;
- an already-installed dependency version conflicts with the requested range;
- the dependency graph contains a cycle;
- a different root version is already installed and update/rollback semantics are required.

## Version rules

Dependency `min_version` and `max_version` use the existing StageCore semantic `x.y.z` / prerelease form.

This slice adds deterministic semantic comparison and rejects manifests where a dependency declares `min_version > max_version`.

Ranges are inclusive.

## Resolver behavior

The resolver is read-only and operates from the verified Extension Library plus verified installed state.

For required dependencies it:

- accumulates constraints from all selected parents;
- treats a verified installed version as authoritative and never silently replaces it;
- searches compatible registered Library candidates when the dependency is not installed;
- orders candidates by highest semantic version, then deterministic tie-breakers;
- performs global backtracking when a candidate's transitive requirements later conflict;
- detects cycles after candidate selection so an alternate candidate may be tried before the graph is declared blocked;
- produces a dependency-first topological install order.

This means the planner is not a greedy "first package wins" resolver. A higher candidate that creates a later conflict can be abandoned in favor of a lower compatible candidate.

## Installed-version policy

F-015 currently permits only one installed package version per extension.

If an installed dependency does not satisfy a new required range, the planner returns `DEPENDENCY_INSTALLED_VERSION_CONFLICT`. It does not choose a different Library package because that would imply update/rollback behavior that has not been implemented yet.

## Optional dependencies

Optional dependencies never block installation and are not automatically selected into the required install steps.

The plan reports advisory warnings instead:

- `OPTIONAL_DEPENDENCY_AVAILABLE` when a compatible optional package exists but is not installed;
- `OPTIONAL_DEPENDENCY_UNAVAILABLE` when no compatible candidate exists;
- `OPTIONAL_DEPENDENCY_VERSION_MISMATCH` when an installed optional dependency falls outside the requested range.

A compatible installed optional dependency produces no warning.

## Integrity behavior

Installed dependency state is read through the existing Installer verification path. A tampered payload, unsafe managed path, changed permissions, hash mismatch, or other installed-payload integrity failure aborts planning rather than being treated as a satisfied dependency.

Library candidates continue to use the existing immutable Software Repository / Vault binding and compatibility checks.

## Candidate selection

For candidates satisfying all accumulated constraints:

1. higher semantic version wins;
2. for the same version, production-ready metadata wins;
3. for any remaining tie, source rank is `OFFICIAL`, then `LOCAL`, then `COMMUNITY`;
4. package ID provides the final deterministic tie-break.

Source is therefore not allowed to override a semantic-version difference in this slice; provenance remains visible in every returned install step and will feed the later permission/review UX.

## Operator API

Read-only plan route:

- `GET /api/v1/extensions/packages/{package_id}/install-plan`

It uses the existing operator read permission and remains available during SHOW because it performs no mutation.

Lifecycle install route remains:

- `POST /api/v1/extensions/packages/{package_id}/install`

It still requires `plugin.manage` and CSRF, and now uses `InstallPlanned`.

When dependencies must be installed first, the API returns HTTP 409 with:

- `EXTENSION_DEPENDENCIES_REQUIRED`;
- the computed plan in the response.

When resolution is blocked, it returns HTTP 409 with:

- `EXTENSION_DEPENDENCY_PLAN_BLOCKED`;
- the computed blockers and plan in the response.

SHOW still takes precedence for mutation attempts and returns `SHOW_CONFIGURATION_LOCKED`.

## Verification in this slice

Tests cover:

- semantic version ordering including prereleases;
- rejection of impossible min/max ranges;
- transitive dependency resolution;
- global backtracking away from a higher conflicting candidate;
- dependency-first install step ordering;
- optional dependency advisory behavior;
- lifecycle rejection before required dependencies are installed;
- successful dependency-by-dependency installation followed by root installation;
- cycle detection;
- installed-version conflict behavior;
- Viewer read access to the plan;
- Owner install rejection with a structured dependency plan when blocked.

Final merge still requires the normal Core CI gate on the final HEAD: module lock, Test, Vet, Race, and Linux ARM64 CGo-free product builds, followed by post-merge Core CI on `main`.

## Deliberately incomplete

This slice does not automatically execute a multi-package plan. That will require a separately tested mutation contract covering partial-failure behavior, storage admission for the whole transaction/sequence, audit semantics, and recovery.

The next F-015 work should build on this plan for permission review and activation/readiness rather than treating `INSTALLED` as executable authority.

The design rule remains:

**Library presence is knowledge; a dependency plan is intent; installation is controlled storage mutation; activation is runtime authority.**
