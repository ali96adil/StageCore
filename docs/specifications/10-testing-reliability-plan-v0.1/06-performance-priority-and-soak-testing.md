# 06 — Performance, Priority & Soak Testing

## Reference Performance Targets

On the documented reference hardware and local network:

- simple Route evaluation p95 <= 20 ms before adapter dispatch;
- accepted P1 Command -> internal dispatch p95 <= 50 ms;
- Hub runtime Event -> local operator UI update p95 <= 250 ms.

These are MVP engineering targets, not hard real-time certification. A miss is measured and investigated; it is not hidden inside external device latency.

## Priority Isolation Tests

Run P1 Cue traffic while intentionally creating P3 load:

- large media transfer;
- software download;
- backup job;
- archive/integrity work;
- verbose diagnostic logging within configured limits.

Expected:

- P1 queues remain bounded and responsive;
- bulk jobs yield/pause according to policy;
- entering SHOW pauses/rejects nonessential bulk work;
- memory/CPU growth stays bounded enough for the reference deployment.

## Burst Tests

Exercise:

- rapid operator GO attempts under normal guardrails;
- burst of test Inputs/OSC input events;
- repeated Route matches with debounce/rate limits;
- multiple parallel Actions in one Cue;
- repeated Plugin result/error Events.

Verify no unbounded queue growth, duplicate semantic execution or UI backlog that hides current runtime state.

## Soak Test

Minimum MVP soak:

- 2-hour Rehearsal Session;
- reference 100-Cue/200-Action project;
- periodic Routes/Notes/health updates;
- at least one controlled Companion disconnect/reconnect;
- Internet disconnected for part or all of the run.

Record CPU, memory, database/Vault growth, queue depths, reconnect counts, failed Actions and UI/runtime latency samples.

## Stability Rule

A memory leak, queue growth pattern, deadlock or progressive latency increase that threatens a normal rehearsal duration blocks the affected release gate even if short tests pass.