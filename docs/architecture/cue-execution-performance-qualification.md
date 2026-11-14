# Cue execution performance qualification

## Scope and separation

This report measures completion of `cueengine.Engine.ExecuteCueGo` through its
terminal result. It includes command reservation, published Snapshot lookup and
Cue selection, linked-Cue resolution, Action scheduling, simulator dispatch,
execution/event persistence, and command completion. It excludes HTTP request
handling, browser authentication, Project ACL checks, and external device or
adapter latency. The authorization measurements below are a separate
component-only benchmark; do not add or substitute them for the full Cue
execution numbers.

The existing Pi 4 ACL baseline and its methodology are recorded in
[`sc024-authorization-qualification.md`](sc024-authorization-qualification.md).
The P1 command-to-internal-dispatch target in the performance plan measures a
different boundary and is not an acceptance limit for completion of a
24-Action Cue group.

## Workload and safety boundary

`BenchmarkMixedCueGroupExecution` compares three workloads with 1,000 samples
each on an Apple M3 Max (`darwin/arm64`, Go 1.27.1, `-cpu=1`):

- A flat Mixed Cue with 24 heterogeneous Actions.
- Six linked Cues with 24 Actions total. The root fans out into nested chains;
  every Cue is compound and uses parallel, barrier, and sequential Actions.
- The same linked workload with independent Cue start delays of 0, 4, 8, 12,
  16, and 20 ms.

Actions cover tablet media, lighting, OSC, and media capabilities. Each run
uses an isolated temporary SQLite database and a `SIMULATION` Session. The
`simulator.SessionExecutor` sends every Action to the Digital Twin. A guarded
physical executor recorded zero calls in the correctness test and every
benchmark iteration. No production service, production data, or real device
was used.

The P50/P95/P99 values use nearest-rank percentiles over 1,000 complete
executions. Latency jitter is the P95 absolute deviation from that scenario's
median. Schedule jitter is the absolute difference between the configured
start delay and the first Action's persisted start time relative to the
`cue.started` event; values are aggregated over the six Cue branches and all
scheduled samples.

| Workload | Mean | P50 | P95 | P99 | Latency jitter P95 | Schedule jitter P50 / P95 / P99 | Allocations per GO |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Flat Mixed Cue, 24 Actions | 10.109 ms | 9.945 ms | 11.02 ms | 11.64 ms | 1.308 ms | — | 439,088 B; 8,201 allocs |
| Six linked compound Cues, 24 Actions | 10.036 ms | 10.03 ms | 11.13 ms | 12.32 ms | 1.276 ms | — | 449,724 B; 8,301 allocs |
| Scheduled linked compound Cues, 24 Actions | 23.477 ms | 23.35 ms | 24.27 ms | 24.95 ms | 0.918 ms | 1.282 / 1.771 / 2.364 ms | 451,407 B; 8,324 allocs |

The scheduled case completes near the maximum configured 20 ms branch delay;
its added completion time is scheduling behavior, not ACL overhead. The
reported schedule jitter includes the Cue Engine work between `cue.started`
and Action start, as seen by the persisted timestamps.

The combined benchmark command took 56.69 seconds wall time (23.84 seconds
user CPU, 17.14 seconds system CPU). `/usr/bin/time -l` reported a peak RSS of
223,936,512 bytes (about 213.5 MiB). This is the local test-command process
tree, including test setup and migrations, not Hub service memory or a Pi
deployment measurement. The per-GO allocation values are measured inside each
benchmark scenario. This short run does not establish a soak or leak result.

## Larger Mixed Cue and safety-command loads · 2026-11-14

The scale benchmark adds 100 samples per case and executes the reference
100-Cue/200-Action project before its benchmark. Flat Mixed Cues and nested
linked chains run as `SIMULATION` sessions against a temporary SQLite
database. The physical executor remains guarded and must stay at zero. Latency
is full `Engine.ExecuteCueGo` completion; ACL, HTTP, and real adapter/device
latency are excluded.

| Workload | P50 | P95 | P99 | Latency jitter P95 | GC/op | GC pause/op | Allocations per GO |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Flat Mixed Cue, 50 Actions | 19.20 ms | 21.41 ms | 23.34 ms | 2.206 ms | 0.34 | 9.10 µs | 894,020 B; 16,629 allocs |
| Flat Mixed Cue, 100 Actions | 39.55 ms | 41.41 ms | 43.02 ms | 1.959 ms | 0.75 | 19.52 µs | 1,770,056 B; 32,835 allocs |
| Flat Mixed Cue, 200 Actions | 73.31 ms | 79.34 ms | 81.71 ms | 6.029 ms | 1.49 | 36.28 µs | 3,523,352 B; 65,270 allocs |
| Linked chain, 25 Cues / 50 Actions | 19.63 ms | 21.22 ms | 23.30 ms | 1.583 ms | 0.38 | 9.79 µs | 961,776 B; 17,092 allocs |
| Linked chain, 50 Cues / 100 Actions | 39.33 ms | 43.13 ms | 43.97 ms | 3.799 ms | 0.73 | 19.10 µs | 1,911,461 B; 33,804 allocs |
| Linked chain, 100 Cues / 200 Actions | 78.70 ms | 82.30 ms | 84.51 ms | 3.597 ms | 1.48 | 38.63 µs | 3,808,384 B; 67,308 allocs |
| Scheduled linked, 25 Cues / 50 Actions | 22.63 ms | 23.46 ms | 23.62 ms | 0.824 ms | 0.37 | 9.67 µs | 966,686 B; 17,144 allocs |

The scheduled linked case's start-delay jitter was P50/P95/P99
0.781/1.604/5.477 ms for configured delays from 0 through 20 ms. P99 is the
maximum sample in these 100-sample runs, so it is a stress indicator, not a
high-confidence tail estimate. `-benchmem` reports per-GO allocations; the
GC columns are measured from runtime memory statistics around each case.

### STOP and Emergency Blackout while GO is delayed

The Runtime Control benchmark admits 1, 8, or 32 independent GO requests with
60-second authored start delays, waits until every request is active, then
times the terminal STOP response. Emergency Blackout uses a local simulated
safety callback, latches the test session, cancels every delayed Cue, and
persists its result. No device adapter or physical output is connected. These
numbers include Runtime Control and SQLite work and are separate from Engine
completion and ACL/HTTP latency.

| Operation / delayed active Cues | P50 | P95 | P99 | Latency jitter P95 | GC/op | GC pause/op | Allocations per safety operation |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| STOP / 1 | 0.502 ms | 0.710 ms | 0.951 ms | 0.208 ms | 0 | 0 ns | 13,869 B; 336 allocs |
| STOP / 8 | 2.625 ms | 3.368 ms | 3.441 ms | 0.743 ms | 0.01 | 238 ns | 56,756 B; 1,276 allocs |
| STOP / 32 | 10.89 ms | 12.71 ms | 14.15 ms | 1.824 ms | 0.05 | 1.07 µs | 204,340 B; 4,505 allocs |
| Emergency Blackout / 1 | 0.618 ms | 0.904 ms | 1.159 ms | 0.287 ms | 0 | 0 ns | 18,736 B; 463 allocs |
| Emergency Blackout / 8 | 2.977 ms | 3.713 ms | 3.929 ms | 0.736 ms | 0.02 | 541 ns | 64,055 B; 1,431 allocs |
| Emergency Blackout / 32 | 11.17 ms | 12.49 ms | 12.99 ms | 1.316 ms | 0.06 | 1.40 µs | 208,886 B; 4,628 allocs |

For this combined local run, `/usr/bin/time -l` reported 54.45 seconds wall,
33.21 seconds user CPU, 15.94 seconds system CPU, and 223,526,912 bytes
(about 213.2 MiB) maximum RSS for the whole Go test process tree. This is not
Hub-only CPU/RAM and includes package startup, database migrations, and all
benchmark cases.

The isolated six-case safety benchmark took 23.33 seconds wall (18.92 user,
3.80 system CPU) with 218,726,400 bytes (about 208.6 MiB) maximum process-tree
RSS, including migration and setup. Only the timed operation window contributes
to its GC and allocation metrics.

### Simultaneous Mixed-Cue GO burst · 2026-11-14

`BenchmarkMixedCueBurst` starts 4 or 8 independent GO requests together, each
in its own `SIMULATION` Session against the same immutable Snapshot. Every GO
executes a six-Cue linked compound chain with 24 heterogeneous Actions. The
Digital Twin handles every Action; the guarded physical executor stayed at
zero. Each case used 100 bursts on the Apple M3 Max (`darwin/arm64`, Go 1.27.1)
with `-cpu=1,4`. The Engine boundary includes SQLite-backed reservations,
schedule, simulated Action execution, event persistence and terminal results;
HTTP, ACL and physical/external adapter time are excluded.

| Concurrent GO / Go CPUs | GO P50 / P95 / P99 | GO latency jitter P95 | Burst P50 / P95 / P99 | Burst jitter P95 |
| --- | ---: | ---: | ---: | ---: |
| 4 / 1 | 31.83 / 37.40 / 39.68 ms | 6.611 ms | 40.23 / 42.31 / 43.79 ms | 3.563 ms |
| 4 / 4 | 42.53 / 45.48 / 48.57 ms | 12.741 ms | 44.33 / 45.89 / 48.58 ms | 1.969 ms |
| 8 / 1 | 61.56 / 73.30 / 78.02 ms | 13.265 ms | 81.65 / 83.75 / 85.91 ms | 2.389 ms |
| 8 / 4 | 80.26 / 92.53 / 97.27 ms | 41.194 ms | 91.13 / 94.65 / 101.00 ms | 9.953 ms |

| Concurrent GO / Go CPUs | Start skew P50 / P95 / P99 | GC / burst | GC pause / burst | Allocations / burst |
| --- | ---: | ---: | ---: | ---: |
| 4 / 1 | 2.255 / 4.248 / 4.743 ms | 0.72 | 13.25 µs | 1,799,638 B; 33,200 allocs |
| 4 / 4 | 2.708 / 8.709 / 11.290 ms | 0.88 | 29.93 µs | 1,903,990 B; 35,272 allocs |
| 8 / 1 | 4.755 / 9.347 / 10.131 ms | 1.34 | 23.77 µs | 3,600,295 B; 66,424 allocs |
| 8 / 4 | 5.875 / 12.170 / 18.000 ms | 1.77 | 60.14 µs | 3,809,552 B; 70,550 allocs |

GO latency percentiles aggregate 400 samples at concurrency 4 and 800 at
concurrency 8. Burst percentiles use 100 samples per row, so P99 is the
nearest-rank top sample and is a tail-stress indicator, not a stable SLO
estimate. Start skew is worker start time relative to the shared release
barrier; authored per-Cue delay jitter is measured separately above. The
combined command took 27.33 seconds wall (13.31 user CPU, 12.17 system CPU)
and peaked at 224,460,800 bytes (about 214.1 MiB) process-tree RSS. These are
whole-command figures including fixture setup and database migrations, not
Hub service CPU or RAM. No optimization is justified by this Mac-only result.

The Pi 4 burst/sustained gate remains open. The qualification SSH alias failed
with `websocket: bad handshake` before opening a remote shell; no remote command
ran. This benchmark is an additional M3 baseline only and does not qualify
Raspberry Pi performance.

### Sustained Rehearsal sequence

`TestSustainedReferenceRehearsalRuns100Cues200Actions` and
`BenchmarkSustainedReferenceRehearsal` use an actual REHEARSAL Session with 100
sequential Cues and 200 Actions. Each Action is resolved only by the test
registry's `simulator.Adapter`; that registry has no physical or network
executor. The benchmark runs 20 complete rehearsals and records 2,000
individual GO latency samples. Per-operation benchmark cost is the complete
100-GO rehearsal; latency percentiles and jitter are per GO.

| Rehearsal workload | Session mean | GO P50 | GO P95 | GO P99 | GO jitter P95 | GC / 100-GO session | GC pause / session | Allocations / 100-GO session |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Before graph-validation fix | 414.01 ms | 4.121 ms | 5.324 ms | 6.028 ms | 1.409 ms | 84.9 | 1.458 ms | 217,224,530 B; 1,331,852 allocs |
| After graph-validation fix | 305.29 ms | 3.008 ms | 4.176 ms | 4.795 ms | 1.172 ms | 15.05 | 0.347 ms | 39,004,484 B; 292,064 allocs |

The sustained command took 9.42 seconds wall (7.42 user, 3.26 system CPU),
with 219,201,536 bytes (about 209.0 MiB) maximum RSS for the entire Go test
process tree, including fixture migration and setup. Per-session B/op and
allocation counts cover the 100 timed GO requests; they are not retained Hub
memory.

CPU and allocation profiles of the 100-Cue/200-Action case show SQLite-backed
event and execution persistence as the main sampled CPU cost: SQLite syscalls
and `database/sql` `ExecContext` account for most CPU samples. The first
allocation profile exposed repeated full-manifest indexing in
`nestedCueIDs`: it called `resolveCueGroup` once per Cue, and every call copied
the complete Cue index. `nestedCueIDs` now validates parent links and cycles
with one index and one graph traversal. The same 20-rehearsal benchmark
reduced allocations by 82% (217.2 MB to 39.0 MB per 100-GO session), GC count
by 82% (84.9 to 15.05), and session time by 26% (414 to 305 ms). In the
post-fix allocation profile, `resolveCueGroup` fell from 3.55 GiB to 41.5 MB
over 20 sessions. `GetRuntimeSnapshot`/SQLite row decoding is now the largest
remaining allocation site in that profile. This measured result justifies
the bounded graph-validation change; it does not yet justify a snapshot cache
or a persistence semantic change.

The profile and benchmark use the same single-CPU M3 Mac workload. Their
process-tree maximum RSS was 225,034,240 bytes (about 214.6 MiB) including
database migrations and fixture setup. It is not a service-only RAM value or
a Raspberry Pi measurement.

### Pi 4 qualification state

The local machine is Apple M3 Max (`darwin/arm64`), not Raspberry Pi. A
read-only connection attempt to the explicitly named `stagecore-qualification`
SSH alias failed during handshake before host identity was established. No
remote command, service, test database, or device action ran. These scale and
safety results are a Mac baseline only. The Pi 4 full-engine and
burst/sustained rehearsal gates remain open and must not be inferred from
GitHub ARM64 cross-builds or this Mac result.

## Separate ACL component measurements

The live-session and Project authorization benchmark was run separately on the
same M3 Max, with 500 samples per case and one Go CPU:

| Authorization component | Mean | P95 | P99 | Allocations per check |
| --- | ---: | ---: | ---: | ---: |
| SHOW GO authorization boundary | 0.127 ms | 0.138 ms | 0.387 ms | 4,544 B; 130 allocs |
| Four sequential authorization checks | 0.395 ms | 0.453 ms | 0.510 ms | 18,305 B; 520 allocs |

These are authorization-only checks. They do not include Action dispatch or
full Cue completion. The earlier Pi 4 figures remain in the SC-024
authorization report and are not inferred from this Mac run.

## Verification

- `go test ./internal/cueengine ./internal/runtimecontrol -count=1` passed.
- `go test -race ./internal/cueengine ./internal/runtimecontrol -count=1` passed
  (cueengine 59.987 s; runtimecontrol 64.641 s).
- `go vet ./internal/cueengine ./internal/runtimecontrol` passed.
- `TestNestedCueIDsValidatesManifestGraph` passed for valid nested links,
  missing/disabled children, multiple parents and cycles.
- `TestMixedCueNestedCompoundChainRunsOnlyInSimulation` passed: all six linked
  Cues and 24 Action executions completed, with zero physical-executor calls.
- `TestMixedCueBurstRunsOnlyInSimulation` passed with eight independent
  sessions, 192 completed Action executions, and a zero physical-executor
  tripwire. `go test -race ./internal/cueengine -run
  '^TestMixedCueBurstRunsOnlyInSimulation$' -count=1` is the focused race gate
  for this concurrent workload.
- `TestReference100Cue200ActionLinkedChainRunsOnlyInSimulation` passed with
  99 linked children and 200 completed persisted Actions; the physical
  executor tripwire stayed at zero.
- `TestSustainedReferenceRehearsalRuns100Cues200Actions` passed with 100
  completed Cues and 200 completed Actions using only `simulator.Adapter`.
- The focused `internal/httpapi` SecurityOperation revocation and SHOW-gate
  tests passed. They also verify that a rejection before mutation remains
  marked `effect_may_have_been_committed=false` if the session is revoked while
  the request body is read.
- The separate `internal/projectaccess` authorization benchmark passed.
- `git diff --check` passed.

## Resume checkpoint

Continue on `integration/stagecore-rc` and Draft PR #534. The performance
batch is pushed at `814cf84b38a6b33379cfcd79270a5ba414e3d9d2`. Its exact-head
Core CI run `43529309969` passed all five checks, including Go 1.26/1.27,
isolated tests/ARM64 and both Swift Companion checks.
The report, benchmarks and matrix do not imply a Pi performance pass.
The run is complete; subsequent pushes must still wait for their exact-head CI.

Remaining performance gates are Pi 4 ARM64 execution in a verified isolated
qualification environment, plus burst/sustained Pi runs. An explicit
read-only SSH attempt to `stagecore-qualification` failed before host identity
was established, so do not substitute the ambiguous Pi alias or infer a Pi
result from CI cross-builds. Continue software work while Pi access is blocked.
Keep production services/data untouched, physical outputs simulated, and
Production Project ACL disabled.
