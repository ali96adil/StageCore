# 10 — First Rehearsal Qualification

## Purpose

This is the practical gate before using StageCore in the first real rehearsal. It is not a production certification; it proves the defined MVP can be trusted for a controlled rehearsal on the selected reference setup.

## Pre-Rehearsal Checklist

### Hub

- StageCore build/version recorded;
- database and Vault writable/healthy;
- sufficient runtime storage reserve;
- active Project opens cleanly;
- expected Published Runtime Snapshot selected;
- verified recent Project/state backup exists;
- Hub security state claimed/healthy.

### Network

- dedicated stage LAN operational;
- Hub reachable locally without Internet;
- Companion/client resolve/connect through intended stage network;
- WAN/Internet can be disconnected without breaking local control;
- if dual-WAN exists, failover has been tested separately and is not required for runtime.

### Companion / Targets

- required Companion trusted and assigned to `VIDEO-MAIN`;
- Snapshot/config hash matches;
- required media verified locally;
- required macOS permissions/capabilities available;
- real OSC test target receives expected test command;
- no blocking Preflight item remains.

## Qualification Run

Perform at least:

1. start Rehearsal Session;
2. execute a representative sequence of Cues;
3. trigger at least one Route/Input path;
4. verify Session/Action results and Notes;
5. disconnect Internet/WAN and continue local operation;
6. disconnect/reconnect Companion during controlled idle window;
7. verify no duplicate Action on reconnect;
8. intentionally create one safe adapter/target failure and confirm truthful non-success;
9. stop Rehearsal;
10. restart Hub/application;
11. reopen Project and verify Snapshot + Session history;
12. create/verify post-run backup.

## No-Go Conditions

Do not use the build for the rehearsal if any of these remain unresolved:

- known duplicate Cue/Action execution;
- corrupted/lost committed Project data;
- inability to reopen Published Snapshot/history after restart;
- required Companion/Snapshot mismatch hidden as READY;
- required media mismatch;
- false success on known failed Action;
- unauthenticated/unauthorized runtime control;
- critical storage state;
- local runtime stops when Internet is removed.

## Qualification Record

Record date, build/commit, Hub hardware/storage, router/network topology, Companion version, Project/Snapshot ID, checklist result, known non-blocking issues and operator who performed the run.