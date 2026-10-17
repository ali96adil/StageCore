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

## Next reviewed PRs

- Establish headless Pi 4 output/mixer configuration and actual two-deck playback reliability; reject any option that competes with GO/lighting safety.
- Design Hub-owned, authenticated project media import with quota, checksum and durable file IDs.
- Build Operator browser controls (iPhone/tablet) with authorization and read-only actual backend diagnostics; add secure sidecar IPC integration without permitting arbitrary mpv commands.
- Implement explicit `REHEARSAL_AUDIO_MODE=LOCAL` cue mappings; no automatic Ableton retries or failback and no replay on reconnect.
- Fade In/Out, seek, output selection and Bluetooth only after qualification.

No auto-install/deploy or merge until approved. See [SC-021](https://github.com/ali96adil/StageCore/issues/502).
