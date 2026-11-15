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

`TestStopCancels32OverlappingDelayedCues` and
`TestEmergencyBlackoutCancels32OverlappingDelayedCues` provide integration
regressions at the largest measured safety-operation load. Each starts 32
simulated GO requests behind 60-second delays, invokes STOP or Emergency
Blackout, then requires all 32 command results and persisted Cue executions
to be terminal `CANCELLED`. The Blackout case also verifies the durable
blackout latch is set before its simulated safety callback and that the
callback is dispatched exactly once. Both focused tests passed normally and
under Race, and `go vet ./internal/runtimecontrol` passed. No physical
executor or output is registered in either test harness.

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
| Follow-up after Cue Engine fail-closed session-state gate | 295.62 ms | 2.929 ms | 4.050 ms | 4.462 ms | 1.122 ms | 14.80 | 0.245 ms | 39,208,563 B; 298,156 allocs |

The follow-up is a separate 20-session run on the same local M3-class host, not
a paired before/after trial. It adds no physical dispatch and does not indicate
a sustained rehearsal regression; the small allocation difference is within
the limits of this unpaired run. `/usr/bin/time -l` reported 7.08 seconds wall,
5.37 seconds user CPU, 3.35 seconds system CPU, and 220,299,264 bytes (about
210.0 MiB) maximum process-tree RSS, including test setup and migrations. This
is not Hub service CPU or memory and does not qualify ARM64 Pi performance.

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

The ARM64 qualification runs below used Raspberry Pi 4 Model B Rev 1.4,
Debian 13.6 (`linux/arm64`), Linux 6.18.39, and Go 1.27.1. The static test
binaries were cross-built from source commit `a90403b863009081447d1cb2ebbce88947ad1cf6`.
The Cue Engine binary SHA-256 is
`89fd092ce4d5c16b07ec38ab9fe55f6d97bc1d7854f09349eb8e3fd86f044c4a`; the
Runtime Control binary SHA-256 is
`15dc5c9beafb8c632b83c5f06f7c6edc217e623c6bd4490e43ba00b8841d60b5`.

These Pi measurements qualify source `a90403b`, not the current campaign
head. Since that binary was built, Cue Engine switched its Session read to
`GetSessionFoundation` and added a fail-closed start-state gate; Runtime
Control's bounded STOP wait changed from 2 to 5 seconds. The measurements
below remain a useful historical ARM64 baseline, but current-head P95/P99,
STOP/Blackout and CPU/RAM qualification must be rerun in an isolated Pi 4 lab.
Do not substitute the production `stagecore-pi` host or infer current-head
qualification from the older binaries.

Each test ran as an unprivileged process at `nice +19` inside private user,
network, mount, and PID namespaces. The sandbox mounted temporary filesystems
over `/dev`, `/opt`, `/var/lib`, `/home`, and `/run`, checked that the sound
device, production install/data paths, and Hub socket were hidden, and used a
private `/tmp` scratch directory. The host's CPU and memory controller were
not isolated; low scheduling priority was used to reduce competition with
other host work. No command was sent to a service or device. Cue Actions used
the Digital Twin, and guarded physical-executor counters remained zero.

Cue Engine times cover command reservation, Snapshot lookup, linked-Cue
resolution, scheduling, simulated dispatch, event persistence, and completion.
They exclude HTTP, ACL checks, and real adapter/device latency. Runtime Control
STOP/Blackout timings below include Runtime Control and SQLite work and use a
simulated emergency callback; they are separate from Cue Engine and ACL
measurements.

#### Mixed and linked Cue Engine completion

The 24-Action workload has 1,000 samples per row. Scale cases have 100 samples
per row. `-test.cpu=1,4` sets Go's `GOMAXPROCS`; every process remains low
priority. Latency jitter is P95 absolute deviation from the case median.
Schedule jitter compares the configured per-Cue start delay with the first
persisted Action start time relative to `cue.started`.

| Workload | Go CPUs | P50 / P95 / P99 | Latency jitter P95 | Schedule jitter P50 / P95 / P99 | Allocations per GO |
| --- | ---: | ---: | ---: | ---: | ---: |
| Flat Mixed, 24 Actions | 1 | 68.98 / 74.84 / 80.48 ms | 6.693 ms | — | 439,639 B; 8,211 allocs |
| Flat Mixed, 24 Actions | 4 | 73.73 / 84.60 / 114.2 ms | 10.866 ms | — | 454,732 B; 8,494 allocs |
| Linked compound, 6 Cues / 24 Actions | 1 | 71.15 / 77.06 / 92.55 ms | 7.038 ms | — | 454,732 B; 8,399 allocs |
| Linked compound, 6 Cues / 24 Actions | 4 | 76.88 / 106.8 / 120.0 ms | 29.970 ms | — | 473,973 B; 8,780 allocs |
| Scheduled linked compound, 6 Cues / 24 Actions | 1 | 72.68 / 79.51 / 97.39 ms | 7.925 ms | 6.008 / 21.464 / 26.639 ms | 460,335 B; 8,501 allocs |
| Scheduled linked compound, 6 Cues / 24 Actions | 4 | 79.42 / 98.36 / 122.7 ms | 18.942 ms | 1.270 / 1.695 / 2.252 ms | 474,730 B; 8,787 allocs |

| Workload (`-cpu=1`) | P50 / P95 / P99 | Latency jitter P95 | Schedule jitter P50 / P95 / P99 | GC/op; pause/op | Allocations per GO |
| --- | ---: | ---: | ---: | ---: | ---: |
| Flat Mixed, 50 Actions | 123.8 / 129.6 / 136.2 ms | 5.775 ms | — | 0.24; 23.057 µs | 894,265 B; 16,634 allocs |
| Flat Mixed, 100 Actions | 239.2 / 249.5 / 278.6 ms | 11.640 ms | — | 0.50; 48.743 µs | 1,770,294 B; 32,838 allocs |
| Flat Mixed, 200 Actions | 468.0 / 480.4 / 492.5 ms | 14.183 ms | — | 1.07; 103.327 µs | 3,524,029 B; 65,280 allocs |
| Linked, 25 Cues / 50 Actions | 130.3 / 143.1 / 235.3 ms | 12.812 ms | — | 0.28; 28.997 µs | 976,255 B; 17,379 allocs |
| Linked, 50 Cues / 100 Actions | 257.1 / 270.9 / 273.9 ms | 14.051 ms | — | 0.65; 64.508 µs | 1,943,144 B; 34,436 allocs |
| Linked, 100 Cues / 200 Actions | 500.1 / 531.8 / 588.1 ms | 31.718 ms | — | 1.35; 132.488 µs | 3,850,571 B; 68,129 allocs |
| Scheduled linked, 25 Cues / 50 Actions | 133.6 / 139.0 / 142.4 ms | 6.211 ms | 8.788 / 41.201 / 75.443 ms | 0.30; 32.771 µs | 1,002,050 B; 17,847 allocs |

#### Concurrent GO bursts

Each row contains 100 simultaneous bursts. Each GO executes a nested,
six-Cue/24-Action compound chain. GO latency is measured per request; burst
latency is from the shared release barrier until every GO completes. Launch
skew is reported separately from Cue schedule jitter.

| GO per burst / Go CPUs | GO P50 / P95 / P99 | GO jitter P95 | Burst P50 / P95 / P99 | Burst jitter P95 | Launch skew P50 / P95 / P99 | GC/burst; pause | Allocations per burst |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 4 / 1 | 256.7 / 289.0 / 315.7 ms | 84.253 ms | 280.7 / 307.1 / 317.7 ms | 26.339 ms | 7.285 / 31.598 / 33.357 ms | 0.55; 56.063 µs | 1,869,514 B; 34,604 allocs |
| 4 / 4 | 290.7 / 410.3 / 428.8 ms | 121.550 ms | 297.0 / 423.8 / 435.3 ms | 126.733 ms | 0.019 / 0.070 / 0.090 ms | 0.64; 133.543 µs | 1,903,650 B; 35,267 allocs |
| 8 / 1 | 485.4 / 573.1 / 584.2 ms | 232.499 ms | 577.7 / 593.5 / 639.0 ms | 29.829 ms | 8.798 / 62.640 / 75.138 ms | 1.16; 117.535 µs | 3,762,147 B; 69,675 allocs |
| 8 / 4 | 559.0 / 822.3 / 871.0 ms | 308.735 ms | 606.9 / 870.7 / 876.9 ms | 263.764 ms | 0.033 / 0.062 / 0.100 ms | 1.39; 306.132 µs | 3,808,898 B; 70,568 allocs |

#### STOP and Emergency Blackout under delayed GO load

These rows contain 100 operations each with 1, 8, or 32 active Cues delayed by
60 seconds. The command cancels the delayed GO work. Blackout invokes only the
test safety callback; it does not drive an adapter or fixture.

| Operation / delayed active Cues | P50 / P95 / P99 | Latency jitter P95 | GC/op; pause/op | Allocations per operation |
| --- | ---: | ---: | ---: | ---: |
| STOP / 1 | 8.898 / 14.04 / 15.18 ms | 5.142 ms | 0; 0 ns | 19,618 B; 446 allocs |
| STOP / 8 | 104.1 / 145.3 / 154.8 ms | 78.188 ms | 0; 0 ns | 221,648 B; 3,457 allocs |
| STOP / 32 | 656.0 / 998.0 / 1,134 ms | 386.602 ms | 0.28; 29.940 µs | 2,341,385 B; 20,276 allocs |
| Emergency Blackout / 1 | 10.30 / 14.70 / 16.05 ms | 5.036 ms | 0.01; 1.210 µs | 24,400 B; 568 allocs |
| Emergency Blackout / 8 | 103.1 / 141.2 / 148.5 ms | 76.282 ms | 0; 0 ns | 210,447 B; 3,355 allocs |
| Emergency Blackout / 32 | 650.0 / 1,028 / 1,059 ms | 396.044 ms | 0.07; 7.807 µs | 2,323,583 B; 20,233 allocs |

#### Sustained 100-Cue / 200-Action Rehearsal

`BenchmarkSustainedReferenceRehearsal` ran 20 sessions, each with 100
sequential GO requests and 200 completed simulated Actions. The 2,000 GO
samples had P50/P95/P99 `30.02/39.06/41.44 ms`, P95 latency jitter `10.016
ms`, and a mean complete-session time of `2.998 s`. Each 100-GO session used
`38,967,218 B` and `291,074` allocations, with 14.30 GC cycles and 1.426 ms
GC pause. Process maximum RSS was 32,524 KiB.

#### USB-DMX refresh under synthetic CPU contention

`BenchmarkManualContinuousRefreshJitter` runs the real `ManualOutput` 250 ms
refresh ticker against an in-memory `PacketPort`, records 100 intervals per
case, then cancels and verifies the port closes. It launches
`max(1, GOMAXPROCS-1)` CPU workers, so `-test.cpu=1,4` tests one and three
competing goroutines respectively. The test writes no serial device and never
discovers USB hardware.

Jitter is the absolute deviation of each refresh interval from its case
median. Each row has 100 samples; P99 is the highest or next-to-highest
nearest-rank sample and is a stress observation, not a stable tail guarantee.

| Go CPUs / competing workers | Refresh P50 / P95 / P99 | Jitter P50 / P95 / P99 | GC/op | Allocations / 100 intervals |
| --- | ---: | ---: | ---: | ---: |
| 1 / 1 | 250.3 / 253.7 / 253.7 ms | 0.454 / 4.015 / 4.017 ms | 0 | 12,720 B; 34 allocs |
| 4 / 3 | 250.3 / 253.7 / 253.8 ms | 0.581 / 4.015 / 4.207 ms | 0 | 1,552 B; 19 allocs |

The two-case process took 50.029 seconds wall, 148.686 user CPU seconds,
0.181 system CPU seconds, and peaked at 10,792 KiB RSS. CPU seconds exceed
wall time because the benchmark intentionally runs workers on multiple cores.
These timings characterize the ticker and Go scheduler with a fake port; they
do not include USB adapter writes, DMX bus refresh, or fixture response.

#### CPU/RAM envelope and profile

The sandbox helper recorded whole test-process user/system CPU time and peak
RSS. RSS includes test setup and SQLite migrations; it is not Pi service RSS.
The shell's `/proc` view was not used to infer service CPU.

| Command | Elapsed | User CPU | System CPU | Process maximum RSS |
| --- | ---: | ---: | ---: | ---: |
| Mixed/linked 24-Action, 1,000 samples, `-cpu=1,4` | 703.879 s | 615.378 s | 111.442 s | 50,132 KiB |
| Mixed/linked scale, 100 samples per case | 204.000 s | 185.478 s | 18.666 s | 37,092 KiB |
| GO burst, 100 bursts per case, `-cpu=1,4` | 188.988 s | 178.551 s | 20.970 s | 47,320 KiB |
| STOP/Blackout, 100 operations per case | 251.106 s | 238.317 s | 13.050 s | 38,840 KiB |
| Sustained Rehearsal, 20 sessions | 66.163 s | 60.697 s | 5.553 s | 32,524 KiB |
| USB-DMX refresh, 100 intervals at `-cpu=1,4` | 50.029 s | 148.686 s | 0.181 s | 10,792 KiB |
| 20-sample 100-Cue/200-Action CPU profile | 14.949 s | 13.718 s | 1.254 s | 38,404 KiB |

The sampled CPU profile shows SQLite execution and durable event persistence
as the largest observed Cue Engine cost: SQLite VDBE execution was 41.66% of
CPU samples; `Store.appendEvent` had 33.83% cumulative samples, and Linux
syscalls 7.16% flat. The allocation profile was captured for the whole test
process and includes fixture construction, so its large `readCueDraftStateTx`
allocation is excluded from the timed GO benchmark and is not attributed to
runtime execution. The per-GO `-benchmem` figures above cover the timed Engine
work. No snapshot cache, event batching, or durability change is justified
without an isolated timed profile and an equivalence test for audit ordering.

These results are performance evidence, not a completed release qualification:
P99 is the nearest-rank top sample for 100-sample scale, burst, safety, and
sustained cases. The 1,000-sample 24-Action cases provide a stronger tail
sample. Some delayed-32 STOP/Blackout P95 results are around one second, and
scheduled linked P99 jitter reached 75.443 ms in one scale case. No numeric
acceptance thresholds were supplied, and low-priority CPU scheduling is not
the production scheduler. Preserve these observations for an explicit timing
acceptance decision; do not call them hardware acceptance.

Correctness probes passed in the same sandbox for a six-Cue nested compound
24-Action chain, an eight-GO burst, a linked 100-Cue/200-Action reference
project, and a 100-Cue/200-Action Rehearsal. Every Action completed in the
Digital Twin and the physical-executor probe remained at zero. Production
services, ACL enforcement, data, Vault, and real outputs were not used.

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

The requested Pi 4 performance cases have already been measured in the tables
above: Mixed Cues from 24 through 200 Actions, linked compound chains through
100 Cues / 200 Actions, scheduled starts, 4/8-GO bursts, 100-Cue/200-Action
sustained Rehearsal, and STOP/Blackout against 1/8/32 delayed Cues. The runs
record P50/P95/P99, latency and schedule jitter, GC and allocation counts, and
process CPU/RSS. They used the device-hidden simulator sandbox described above.
Their source is `a90403b`; comparison with current branch HEAD `055e980`
confirms no Cue Engine, Runtime Control, simulator, domain, or publish runtime
source changes since that benchmark build. The separate Pi authorization
baseline measures `ValidateCSRF` and `CheckRuntimeIntent`; those benchmark and
check implementations are unchanged since their recorded source. Do not repeat
these successful measurements solely to refresh the commit label.

Performance evidence is extensive but does not establish a formal acceptance
pass: no full Cue-completion latency/jitter limits were specified, the Pi host
CPU and memory controllers were not isolated, and nearest-rank P99 from
100-sample scale/burst/safety cases is only a top-sample stress observation.
The recorded 32-delayed-Cue STOP/Blackout P95 reaches about one second and
scheduled linked-Cue P99 jitter reached 75.443 ms; these need an explicit
timing acceptance decision. A later read-only SSH attempt to
`stagecore-qualification` failed before host identity was established, so a
new Pi run cannot currently be started if Engine/Runtime code changes. Continue
software work while access is unavailable. Physical outputs remained simulated;
this report does not qualify hardware or production release. Keep production
services/data untouched and Production Project ACL disabled.
