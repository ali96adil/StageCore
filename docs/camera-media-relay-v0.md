# Camera Media Relay v0 — isolated Pi trial

Status: experimental. This is a standalone Go process, not part of StageCore Hub or the normal StageCore setup/update service. It never changes cues, lighting nodes, tablet APKs, or active service configuration.

## Source evidence

On 2026-09-22 the operator's Pi reached the ESP32-CAM over the show LAN: its health endpoint returned state ready, and a 256-byte read from the MJPEG endpoint returned HTTP 200, multipart/x-mixed-replace and a JPEG SOI marker. This is a reachability smoke PASS, not measured sustained streaming.

## Transport

ESP32-CAM (one HTTP multipart JPEG source) -> stagecore-camera-relay -> up to four downstream viewers.

The relay preserves compressed JPEG frame bytes without decode/transcode. An ingest worker owns one upstream connection and reconnects after errors. Each viewer has a bounded one-frame latest-only channel. Source JPEGs larger than 512 KiB, invalid JPEG markers and wrong MIME types are rejected; the viewer limit defaults to four. Upstream reads, HTTP header receipt, per-frame downstream writes and subscriber idleness are time-bounded. Health is provided separately from the stream.

## Trial build (no production installation)

Use a checkout of this exact PR and the Go version required by StageCore's go.mod; ensure the test/CI checks pass:

    go test ./internal/camerarelay ./cmd/stagecore-camera-relay
    go build -o /tmp/stagecore-camera-relay ./cmd/stagecore-camera-relay

Check the camera's IP on its local serial log or DHCP reservation first. Close the camera browser viewer before starting the relay; the firmware is one-upstream only. Run ONLY a loopback trial on the Pi:

    /tmp/stagecore-camera-relay -source http://<camera-ip>:81/api/v0/stream

The default listener is 127.0.0.1:9081; a non-loopback listen address requires an explicit -allow-lan flag (do not use it in this first test). In a separate Pi shell:

    curl --max-time 5 http://127.0.0.1:9081/api/v0/health

While the relay is up, measure the first downstream JPEG boundary without running a browser on the camera itself:

    python3 - <<'PY'
    import urllib.request
    with urllib.request.urlopen("http://127.0.0.1:9081/api/v0/stream", timeout=10) as response:
        data = response.read(256)
        print("HTTP:", response.status)
        print("Content-Type:", response.headers.get("Content-Type"))
        print("Received bytes:", len(data))
        print("JPEG SOI:", b"\xff\xd8" in data)
    PY

Use Ctrl+C to terminate the temporary relay. Do NOT modify the production stagecore-hub service or deploy systemd changes for this trial. Collect redacted logs, measured FPS, memory/CPU and source disconnect/reconnect observations before promoting this component.

## Security and limits

The source and downstream streams use plaintext unauthenticated HTTP. The listener stays loopback-only unless explicitly overridden. There is NO access control for an exposed LAN listener: do not forward this port to the internet or expose it to untrusted Wi-Fi. Before production, define client authentication, firewall segmentation, IP binding, health monitoring, rollback and stage-level readiness policies. A successful Pi loopback test does not establish four-tablet readiness or frame-accurate sync.


## Physical checkpoint — 2026-09-22

User-reported Pi-local trial (not automated CI hardware verification): relay health
returned state=ready, upstream_connected=true and last_frame_age_ms=47 after
1546 received source frames. A Pi-local MJPEG reader returned HTTP 200 and JPEG
SOI. In a subsequent four-reader test, health showed viewers=4, max_clients=4
and 49 additional source frames during five seconds. Four clients each received
52–53 valid JPEG frames; the fifth connection returned HTTP 503. These
measurements do **not** constitute physical four-tablet qualification.
One browser on the trusted show LAN also displayed live relay video;
no Android app playback has yet been tested.

The camera's DHCP address changed after power cycling. Use current DHCP
reservation/discovery instead of assuming an earlier address. Wi-Fi RSSI
ranged roughly -65 to -77 dBm during initial checks; performance, antenna
selection and reconnection require measurement in show conditions.

## Reproducible four-client test (without Android tablets)

Run this standalone smoke probe from the Pi while the relay is running:

    python3 tools/camera-relay-smoke.py --base-url http://127.0.0.1:9081 --viewers 4 --seconds 5

Or run the exact same probe from the Mac on the trusted show LAN, only after
explicitly binding the relay to the Pi show-LAN IP with -allow-lan:

    python3 tools/camera-relay-smoke.py --base-url http://<pi-show-lan-ip>:9081 --viewers 4 --seconds 5

The probe opens four HTTP stream connections, reads full bounded JPEG frames,
samples health, confirms the source frame counter continues advancing and
expects the fifth viewer to return 503. It closes its own clients. Free the
four viewer slots by closing earlier test browser tabs first.

A phone and Mac, multiple browser windows, or different browser applications
on the same Mac can test multiple HTTP connections. Background browser tabs
can throttle and are not reliable independent display-performance measurements.
Four separate physical machines are NOT required for the relay fan-out
connectivity test. Mixed-browser tests do NOT certify the Android player APK,
real four-tablet Wi-Fi, display latency, cue readiness or synchronization.

Never aim this four-reader probe directly at the ESP32-CAM: the camera
firmware permits a single upstream connection. Never port-forward or expose
plaintext unauthenticated relay HTTP to untrusted networks. Do not modify
production Hub or systemd services for this trial.


## Stable camera addressing for reconnects

The relay reconnect loop re-dials the configured source URL after a source failure. A literal DHCP address therefore only remains valid while the camera keeps that lease.

For a camera firmware that advertises a stable `.local` hostname, the relay now resolves that hostname with a bounded IPv4 mDNS query on every new upstream dial. This is intentionally implemented inside the relay rather than relying on libc/NSS so the Linux ARM64 `CGO_ENABLED=0` build keeps the same static build shape.

Example:

    ./stagecore-camera-relay \
      -source http://stagecam-d44a4c.local:81/api/v0/stream \
      -listen 192.168.3.130:9081 \
      -allow-lan

The mDNS lookup uses a unicast-response query from an ephemeral UDP port and has a bounded timeout. A lookup failure keeps the relay in reconnecting state; it does not fall back to public DNS, an unrelated host, or a stale remembered IP.

This source-only change requires a separate attended physical qualification against the real ESP32-CAM and show LAN before replacing the already qualified literal-IP C3 binary.


## First-show service deployment

The release bundle now carries the relay binary, hardened systemd unit and a one-time installer:

- `stagecore-camera-relay`
- `install-camera-relay.sh`
- `camera-relay.service`
- `camera-relay.env.example`

The relay intentionally remains outside the transactional Hub F-010 updater so a failed Hub update cannot leave a partially rolled-back camera component. Install or refresh it explicitly from the exact qualified release bundle. On first install, provide the camera source; use the stable camera `.local` name so DHCP/subnet changes do not require a new source URL:

    ./install-camera-relay.sh \\
      --source http://stagecam-xxxxxx.local:81/api/v0/stream \\
      --flash-control http://stagecam-xxxxxx.local/api/v0/flash

Fresh installs listen on `0.0.0.0:9081` so Tablets can reach the relay on whichever isolated Stage-LAN subnet is active. The relay HTTP surface is unauthenticated by design: do not expose TCP/9081 to WAN, port forwarding or untrusted Wi-Fi.

If `/etc/stagecore/camera-relay.env` already exists, re-running the installer preserves every value not explicitly supplied. This makes network migration safe; for example, the current Pi can migrate only the listener without rewriting the camera source:

    ./install-camera-relay.sh --listen 0.0.0.0:9081

The installer verifies the relay binary against the release `SHA256SUMS`, installs the binary/unit, writes the environment atomically, then enables and restarts `stagecore-camera-relay.service`. After this one-time installation there is no per-boot shell step.

### Restart expectations

The systemd service uses `Restart=on-failure`; camera-source loss itself does **not** terminate the relay process because the relay retries upstream internally. A Pi reboot or unexpected relay process failure therefore restores the service without requiring an operator shell, while ordinary camera reconnects remain within the same relay process.
