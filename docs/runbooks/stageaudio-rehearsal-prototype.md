# StageAudio — two-layer local rehearsal prototype (SC-021)

**Status: developer prototype only. Not deployed, enabled, or physically qualified.** This is for the existing **Raspberry Pi 4 running StageCore Hub**, not a second Pi. Production StageCore GO, published Cues, macOS Companion, DMX and tablets are unchanged.

## Boundary and deliberate omissions

The `stagecore-audio` command is a separate, **opt-in local process**. It launches two audio-only mpv processes (Deck A and B). The only controller is a private **Unix socket** in a new mode-0700 directory; the socket has mode 0600. The program does **not** listen on TCP, expose browser endpoints, integrate with StageCore GO or start at boot.

StageCore will eventually own user authorization, file import and service life cycle, and only forward typed authorized deck operations. It must **never forward raw mpv JSON commands**. The mpv socket itself remains private; mpv IPC is not safe to put on the theatre network.

This prototype uses *local-only* files from a pre-existing media directory. It allows direct-child WAV/MP3/FLAC names only, rejects directories, symlinks, URLs, absolute paths and traversal, and bounds file sizes. Files are not uploaded through StageCore yet. The prototype sends fixed allowlisted commands (LOAD/PLAY/PAUSE/STOP/VOLUME/MASTER_VOLUME/LOOP/STATUS).

There is **no automatic Ableton fallback, no SHOW authorization, no fade/seek implementation, no real audio-level telemetry, and no automatic device selection**. An mpv IPC acknowledgement means only the local engine accepted a command; it does not prove that the Pi's physical output produced sound. Until SHOW safety and physical qualification exist, **do not run the service during production shows**.

## Physical validation later (attended, after explicit approval)

The **Raspberry Pi 4** has an on-board 3.5mm A/V analog jack that can be configured for stereo audio. Run analog out to a line-level mixer input with suitable cabling. Start with the mixer gain lowered. Inspect sound quality, output level, noise/ground-loop issues, clock drift, CPU usage, memory, temperature and GO latency during two simultaneous tracks.

ALSA's raw analogue device may not support concurrent process mixing by default; evaluate ALSA `dmix` or a verified PipeWire/PulseAudio mixer on the Pi image. Bluetooth A2DP is **not implemented** and will be tested separately (latency, dropouts, reconnect safety). A USB DAC may be an optional later upgrade if the analog output is too noisy.

The program currently initializes both mpv decks *paused* at effective 49% gain (Deck 70 × Master 70 / 100); no file is auto-loaded or auto-played. Changing Volume and Master has an audible effect only after an actual file is played.

If the operator explicitly approves physical qualification:
1. Work on a **non-show** Pi copy or stopped/isolated session, backed up first; do not replace running `stagecore-hub` or published snapshots.
2. Install a compatible headless `mpv` package after approval; make a restricted local directory of media files.
3. Build the opt-in executable: `go build -o /tmp/stagecore-audio ./cmd/stagecore-audio` (Go 1.26+).
4. Start explicitly from a terminal, not systemd: `/tmp/stagecore-audio --media-root /path/to/audio-files`.
5. Copy the **private socket path** printed by the program. A local Unix-socket client such as socat can send one newline-terminated JSON request and read one response. Do not expose the socket on LAN.
6. Use typed requests below; close by SIGINT/SIGTERM and verify both mpv players stop.

Example Unix-socket request (replace path with the printed path):

```bash
printf '%s\n' '{"operation":"LOAD","deck":"A","file":"music.mp3"}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"PLAY","deck":"A"}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"LOAD","deck":"B","file":"wind.wav"}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"PLAY","deck":"B"}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"VOLUME","deck":"B","volume":35}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"MASTER_VOLUME","volume":65}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"LOOP","deck":"B","loop":true}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"STOP","deck":"A"}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"STATUS"}' | socat - UNIX-CONNECT:/private/path/control.sock
```

**Note:** commands use a single line of JSON and close after a single response. The status reports the **commanded state**, not confirmed audible output. Two-deck Master changes are not physically atomic; a failed second deck reports `MASTER_VOLUME_PARTIALLY_APPLIED_RECONCILIATION_REQUIRED` rather than pretending both changed.

## Bluetooth A2DP and wired output selection (stacked Bluetooth prototype)

**StageAudio now has explicit support for BOTH the Pi 4 3.5 mm analog output and a paired Bluetooth A2DP speaker**, subject to actual available devices on the headless Pi audio server. The analog plug remains the low-latency rehearsal choice; Bluetooth can lag and unexpectedly disconnect. This is a **choose one output at a time** model, *not* synchronized dual output to wired + Bluetooth at once.

**Prerequisites to physically qualify later:**
- Pi 4's BlueZ controller / `bluetoothctl`, `mpv`, and working audio stack with A2DP support (`pipewire` + `wireplumber` and `pipewire-pulse`, or an equivalent correctly installed PulseAudio service); the final headless configuration must be inspected first. Do not install packages on the live Pi without approval.
- **Initial pairing is deliberately separate** from StageAudio's prototype socket commands, since Bluetooth pairing can require PIN/passkey or confirming a physical speaker. Pair once interactively with BlueZ under an authorized Pi account, e.g. using `bluetoothctl`: choose an agent, start a short scan, verify the speaker's Bluetooth address/identity, `pair <MAC>` with explicit confirmation, stop the scan. Trust changes are never performed by StageAudio automatically. The authenticated browser pairing wizard is a later PR.
- Both `mpv` decks need to be idle/**STOPPED** for routing or connecting a speaker. If a deck has any media loaded, issue `STOP` for A and B (not only `PAUSE`) before changing output; reload and play manually after switching.
- Bluetooth is not auto-selected after connecting; `OUTPUTS` must report the newly created **actual A2DP sink**. If it doesn't appear, display unavailable rather than silently choosing analog.

Typed local commands (use the same private Unix socket from the prior section):

```bash
printf '%s\n' '{"operation":"BT_DEVICES"}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"BT_CONNECT","address":"AA:BB:CC:DD:EE:FF"}' | socat - UNIX-CONNECT:/private/path/control.sock
printf '%s\n' '{"operation":"OUTPUTS"}' | socat - UNIX-CONNECT:/private/path/control.sock
# Copy the exact A2DP output_id from OUTPUTS, do not fabricate IDs from MACs.
printf '%s\n' '{"operation":"SELECT_OUTPUT","output_id":"pulse/bluez_output.AA_BB_CC_DD_EE_FF.a2dp-sink"}' | socat - UNIX-CONNECT:/private/path/control.sock
# Or select the actual analog/headphone output_id returned by OUTPUTS.
```

These examples are **illustrative output IDs only**; actual naming depends on PipeWire/PulseAudio versions, available profiles and mpv audio driver. The service queries `mpv` `audio-device-list`, never changes the system-wide default sink, and sets both audio decks explicitly only when stopped. Errors or a partially applied route are surfaced and require operator review. Bluetooth disconnect is **not** an authorization to fail over to 3.5 mm while playing. The player still is **not integrated with the Hub, phone UI or Cues** in this stacked PR. No SHOW qualification.

## Next reviewed PRs

- Establish headless Pi 4 output/mixer configuration and actual two-deck playback reliability, including verified Bluetooth A2DP and wired 3.5mm; reject any option that competes with GO/lighting safety.
- Design Hub-owned, authenticated project media import with quota, checksum and durable file IDs.
- Build Operator browser controls (iPhone/tablet) with authorization and read-only actual backend diagnostics; add secure sidecar IPC integration without permitting arbitrary mpv commands.
- Implement explicit `REHEARSAL_AUDIO_MODE=LOCAL` cue mappings; no automatic Ableton retries or failback and no replay on reconnect.
- Fade In/Out, seek, output selection and Bluetooth only after qualification.

No auto-install/deploy or merge until approved. See [SC-021](https://github.com/ali96adil/StageCore/issues/502).
