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

- `TestMixedCueNestedCompoundChainRunsOnlyInSimulation` passed: all six linked
  Cues and 24 Action executions completed, with zero physical-executor calls.
- The full `internal/cueengine` test package passed, including its local UDP
  loopback OSC tests.
- The focused `internal/httpapi` SecurityOperation revocation and SHOW-gate
  tests passed. They also verify that a rejection before mutation remains
  marked `effect_may_have_been_committed=false` if the session is revoked while
  the request body is read.
- The separate `internal/projectaccess` authorization benchmark passed.
- `git diff --check` passed.

## Resume checkpoint

Continue on `integration/stagecore-rc` and Draft PR #534. This report is the
checkpoint at the branch tip after the local Mixed Cue performance work and
the preserved two-file security-operation change. The previous remote head
`a1c2d68a5b2799058025fba31b7117477b97bdfe` completed Core CI successfully
(run `43510385247`) before the next push. Recheck local and remote heads and
the new exact-head CI after the normal fast-forward push.

Next performance gates are full-engine ARM64/Pi 4 measurements in an isolated
temporary database, then burst and sustained Rehearsal tests for the
reference 100-Cue/200-Action project. Keep production services/data untouched
and Production Project ACL disabled.
