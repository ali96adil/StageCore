# Mac Live Video physical qualification — Q-LIVE-01..03

Status: **PREPARED, NOT PHYSICALLY QUALIFIED**. Campaign: Issue #148 / PR #225. Use the exact deployed candidate and record its SHA; do not infer Pi deployment from GitHub CI.

## Supported test source (important)

Use the Mac as the **network stream origin** and the native macOS Companion as the **rendering client**. Serve a short looping, visibly timecoded **HLS (.m3u8 over HTTP/HTTPS)** stream from the Mac (or another LAN host reachable by the Mac). AVPlayer-backed `NETWORK_STREAM` supports HTTP(S) media including HLS. A bare `file://` movie, `rtsp://`, an OBS RTMP endpoint, and an arbitrary HTTP MJPEG endpoint are **not qualified substitutes**. A local video file alone is not a network stream. If an existing Mac streaming tool cannot emit compatible HLS, first establish a supported stream and verify it opens in macOS AVPlayer/QuickTime; do not report a physical PASS.

The HLS stream must be looped/long enough for controlled loss and recovery, show a continuously changing timecode, and be hosted on a controlled LAN. Do not include its endpoint or credentials in public GitHub evidence. Record the stream generator name/version, output identity and Mac Companion build locally.

## Preconditions

1. Pin `STAGECORE_PROJECT_ID`, final candidate SHA, active Published Runtime Snapshot, Mac Companion identity/build, the actual named output (default `main`), and the intended LiveSource ID. Confirm native Visual Engine is **enabled** and the Companion advertises all five `video.source.*` capabilities. A state-only Companion is insufficient.
2. Configure exactly one `NETWORK_STREAM` source for the project with a Companion Machine Role execution placement. Check role assignment, Companion trust/heartbeat and required capability Preflight. The legacy `execution_device_id` field may be empty for a Machine Role source; do **not** incorrectly require it to be nonempty.
3. Check the real output is visible on the Mac/projector, not merely present in the Hub inventory. Keep a separate, safe media/blackout baseline for comparison. Run in REHEARSAL; no live SHOW.
4. Capture the existing `phase4-inventory.json` and `Q-LIVE-04.class.coverage.json` without endpoint URLs or secrets. Their milestone PASS proves configuration only.

## Bounded physical sequence

| Step | Action | Required evidence |
| --- | --- | --- |
| 0 | With source stopped, inspect/readiness baseline | Source identity/class/placement and fresh Companion state; do not label the source READY from configuration alone. |
| 1 | Start Mac-hosted HLS. Invoke `video.source.open` with `contract_version:1`, pinned `source_id`, `source_class:NETWORK_STREAM` and supported HTTP(S) `endpoint_ref`. | Terminal result, error code if any, timestamp; native open truth. |
| 2 | Invoke `video.source.select`, then `video.source.route` with pinned `layer_id` and `output_id`. Invoke `video.source.inspect`. | `open=true`, `selected=true`, `native_runtime=true`, `native_open=true`, `native_renderer_surface=true`, one matching `native_routes[]` entry with `renderer_attached=true`. A logical `routes[]` entry alone is not enough. |
| 3 | Watch the real Mac output for changing timecode and motion; inspect source and Operator readiness. | Operator-signed observation of real decoded frames, correct output/layer, no Hub frame transport. Save local redacted screenshots or a short local recording, not the private stream URL. |
| 4 | Isolate **only the stream origin**, keeping Mac Companion, Pi/Hub and their network connection alive. Observe at least one real source failure/readiness transition. | Timestamped error/readiness and inspect results; distinguish stale retained `open` state from **actual** frame delivery. If source loss is not detected, Q-LIVE-03 is BLOCKED/FAIL, not PASS. |
| 5 | Restore origin. Recheck source. If explicit close/open/route is necessary, perform it as a **new authorized command** and record it. | Actual resumed moving frames and fresh readiness. Verify no old `open/select/route` commands were replayed merely due to reconnect. |
| 6 | Explicit `video.source.close` and inspect. | No renderer attachment; visible layer removed. Record final safe output state. |

The native `video.source.inspect` contract is **not** itself a frame-delivery/health telemetry contract: `native_open` can remain true during upstream loss. A loss/recovery PASS therefore requires independent, timestamped physical playback observation **and** truthful source readiness/fault reporting. If the current client does not expose this, record a product gap in Issue #138 and leave Q-LIVE-03 open; do not manufacture a synthetic health event.

## Gate decision boundaries

- **Q-LIVE-01**: one real `NETWORK_STREAM` opened, selected, routed and visibly rendered on the intended Companion/output; both runtime and operator evidence. An inventory-only READY source does not qualify.
- **Q-LIVE-02**: verify capture/decode/render stays on the Mac Companion, and Hub carries only descriptors/commands/results. Document the observed Companion process/output and Hub control-path evidence. A CI architecture assertion alone does not constitute physical PASS.
- **Q-LIVE-03**: real origin loss, detection/classification, controlled recovery and no command replay. Absence of an observed fault is a blocker.
- **Q-LIVE-04**: after Q-LIVE-01 physical PASS, classify `NETWORK_STREAM PASS`; explicitly classify `LOCAL_CAMERA` and `USB_CAPTURE` as PASS only if physically tested, otherwise N/A **only when unavailable on the pinned hardware**. Use `campaign.sh qlive04-status` and `qlive04-ack`. Do not mark a configured-but-untested class N/A.

## Durable evidence and resume

Keep private local evidence under the pinned campaign state: exact SHA, project/snapshot/source/Companion/output/layer IDs, command result IDs and times, redacted inspect snapshots before/open/routed/loss/recovered/closed, physical observation notes and source availability. Do not publish URLs, tokens, network addresses, video frames or secrets. Use `physical-confirmations.py pending` to distinguish automatic preparation from physical confirmations.

If interrupted, inspect the real Companion state and last durable command before resuming; do not silently re-open, re-route or treat a newly started stream as proof of a prior loss/reconnect. Q-LIVE-01..03 stay BLOCKED until the corresponding real Mac observations exist.

## Offline redacted evidence review

`python3 tools/qualification/live-mac-evidence.py /path/to/private/mac-live-evidence.json` validates a bounded six-step operator evidence record. Run `python3 -m unittest discover -s tools/qualification -p 'test-live-mac-evidence.py'` for its deterministic negative cases. The checker never contacts the Hub or Companion, never updates the campaign, and prints `EVIDENCE_READY` rather than a gate PASS. Its input must contain exactly `identity` (project_id, runtime_snapshot_id, stagecore_sha, source_id, companion_id, output_id, layer_id), `steps` (baseline, opened, routed, loss, recovered, closed) and `observations` (moving_frames, companion_local_render, hub_control_only, source_loss_detected, moving_frames_recovered, no_implicit_replay). Each step records increasing `at_us`, pinned `source_id`, result, native_open, renderer_attached, readiness and a short note. Never include endpoint_ref, config or credentials. **This is a consistency check of operator-supplied evidence, not independent runtime verification.** Record physical gate results only after reviewing actual Companion command results and visual observations.
