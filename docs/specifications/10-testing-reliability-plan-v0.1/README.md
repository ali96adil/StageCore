# 10 — Testing & Reliability Plan — v0.1

**Document Type:** Executable Verification, Failure & Release-Gate Plan  
**Status:** Initial implementation baseline  
**Based on:** 02 System Architecture + 04 Event & Command Contracts + 05 MVP Product Specification + 06 Plugin Contract + 07 Companion Specification + 08 Storage & Vault Specification + 09 Security Model

## Core Principle

StageCore is not considered rehearsal-ready because the happy path works once. It must repeatedly survive ordinary stage failures without corrupting Project state, inventing success, replaying non-idempotent Actions, or making Internet connectivity part of the local show-control path.

Testing is organized as release gates. Each implementation slice must prove normal behavior and its failure path before the next layer is treated as reliable.

## Files

- [00 — Goals, Reliability Model & Release Gates](00-goals-reliability-model-and-release-gates.md)
- [01 — Test Environments & Reference Fixtures](01-test-environments-and-reference-fixtures.md)
- [02 — Automated Test Layers & Deterministic Simulation](02-automated-test-layers-and-simulation.md)
- [03 — Network, WAN & Connectivity Faults](03-network-wan-and-connectivity-faults.md)
- [04 — Companion, Plugin & Device Failure Injection](04-companion-plugin-and-device-failures.md)
- [05 — Storage, Process Crash & Power-Loss Recovery](05-storage-crash-and-power-loss-recovery.md)
- [06 — Performance, Priority & Soak Testing](06-performance-priority-and-soak-testing.md)
- [07 — Security Regression & Trust Failure Tests](07-security-regression-and-trust-failures.md)
- [08 — Backup, Restore & Recovery Drills](08-backup-restore-and-recovery-drills.md)
- [09 — CI, Test Evidence & Defect Policy](09-ci-test-evidence-and-defect-policy.md)
- [10 — First Rehearsal Qualification](10-first-rehearsal-qualification.md)
- [11 — Acceptance Criteria](11-acceptance-criteria.md)

## Release Question

> Can StageCore run a representative rehearsal locally, preserve trustworthy state and history, and recover predictably when expected components fail?

If that answer is not demonstrated by repeatable tests, the milestone is not complete.