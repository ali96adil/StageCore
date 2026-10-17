# SC-025: DMXking ultraDMX Micro (0084) — Pi 4 USB-DMX qualification

**Status: Draft transport prototype with a deliberately unconnected local Linux serial writer; NO hardware opened by default, NO device registration, NOT a SHOW-ready output.** The main StageCore Hub and the existing ESP32 DMX Node remain unchanged.

## Hardware identification and protocol

Product pictured by the operator: **DMXking.com ultraDMX Micro**, model **0084**, 3-pin **female XLR**, single DMX512 universe (512 channels), USB-A, FTDI virtual COM port (VCP) on Linux. Supports DMX output *or* input (not simultaneous), but SC-025 first phase intends **output only**. Unlike a raw/open-DMX bitbang adapter, the Micro uses an onboard microcontroller and **ENTTEC USB Pro Widget API 1.44**-compatible framed serial messages. The device is discontinued/replaced by ultraDMX MAX, but original units should still be identifiable and qualified individually.

Official docs:
- https://shop.dmxking.com/ultraDMX-Micro_p_47.html
- https://dmxking.com/downloads/ultraDMX%20Micro%20User%20Manual%20%28EN%29.pdf
- https://cdn.enttec.com/pdf/assets/70304/70304_DMX_USB_PRO_API.pdf

`internal/usbdmx.EncodeDMXFrame` creates **518 bytes**:
`7e 06 01 02 00 <512 DMX channel values> e7`, i.e. start delimiter 7E, send-label 6, payload length 0x0201 (513), DMX start-code 00, channels 1..512, end delimiter E7. The interface firmware is responsible for break/mab timing and repeated DMX output; this packet is NOT a direct 250k baud BREAK/MAB stream.

`ListSerialCandidates` is **read-only**: looks at Linux `/dev/serial/by-id/` and explicitly marks all discovered serial candidates as **UNVERIFIED** until the user selects and verifies the actual hardware. It does NOT open any device or send bytes. FTDI USB serial adapters may be entirely unrelated to DMX lighting (or the StageLaser / StageCam). Never automatically bind arbitrary `/dev/ttyUSB0` to a Project or grant it lighting authority.

## Planned StageCore integration

The real extension must be a new output type `USB_DMX_MICRO` alongside the existing `ESP32_DMX_LIGHTING_NODE`. They can coexist in one Project but have independent universe assignments, device identities, and fault statuses. Do not send two controllers into the same electrically connected physical DMX512 line.

A hardware-owned USB DMX transmit process needs:
- explicit OWNER pairing and Project assignment; security-scoped, snapshot-bound commands;
- exclusive port ownership, stable serial device ID, bounded framed output, restart quarantine, no automatic nonzero frame replay after unplug/reboot;
- a GO-independent bounded queue so FTDI stalls cannot hold back GO, other lighting nodes, tablets, or audio;
- an explicitly selected, permitted DMX transmitter mode and health check;
- stopped or disconnected USB should display UNAVAILABLE, not GREEN or READY; physical output when USB is lost is unknown until measured;
- blackout, channel set and fade semantics matched to current Lighting Node, with published snapshot / current Project authority and safe validation before dispatch.

**Note:** Sending a "blackout" frame is only an instruction, *not proof* that physical fixtures are dark. Characterize device timing and behavior on unplug, hub shutdown, lack of updates, and active output mode before SHOW qualification.

## Later attended checks on Raspberry Pi 4

Do NOT run these commands on a live production SHOW, and do not install/change packages without explicit approval. Non-actuating diagnostics when physically on-site:

```bash
lsusb
ls -l /dev/serial/by-id/
dmesg --ctime | tail -n 30
id
```

Verify the true FTDI device, stable `by-id` path, serial port ownership/permissions and power. Connect a safe single fixture or isolated DMX test load; then specifically test CH1, CH3, CH512, zero frame/blackout, full-frame update, fades, USB hot unplug, reconnect, local Pi reboot and CPU stress. Confirm DMX interface starts/stays in TX mode, outputs correctly with USB Pro label=6, and does **not** surprise-restore a nonzero scene after reconnect. Only after these checks design/install the supervised runtime service and Operator device integration.

## Changes introduced by this Draft

Phase 1 provides pure framing, strict bounds and read-only serial discovery. **This stacked Phase 2 Draft** adds an explicitly invoked Linux port driver and fail-closed packet transmitter, but no CLI/Hub wiring, no automatic USB port opening, no device pairing endpoint, no runtime registration or active output. Nothing is deployed to the Pi.

## Stacked phase 2: isolated serial transmitter and fail-closed gate

A new `OpenVerifiedLinuxPort(ctx, byID, approval)` is available **only as internal Go code** for a future separately installed, hardware-qualified service. It does not run when StageCore starts. It accepts a symlink under `/dev/serial/by-id/` resolving to a `/dev/ttyUSB*` device and requires a non-nil approval callback to independently verify the actual selected DMXking FTDI product/serial identity. Approval is a required caller contract; automatic generic FTDI discovery is **never** sufficient.

If a future attended software test opts in, the port uses `O_NOCTTY`, `O_NONBLOCK`, `O_CLOEXEC`, exclusive tty ownership `TIOCEXCL`, Linux raw mode at host VCP 57600 (USB Pro API notes host serial baud is effectively a dummy setting), and a maximum **800 ms write deadline**. It emits the previously validated USB Pro label-6 frame without shell utilities or bitbanging DMX BREAK/MAB. It does not read input, flash firmware, modify timing settings or claim a physical light ACK.

The `Transmitter` starts cold/disarmed and cannot be armed without a **fresh authorization callback**. Arm first requests an all-zero universe. Nonzero `Send` requires authorization on every call and transmits a complete 512-channel frame only; no stale frame is cached. A serial I/O failure permanently quarantines the instance, closes the FD and prevents automatic replay on reconnect. Explicit `Disarm` attempts a zero-frame request and closes; if the zero write fails, status must report **physical state unknown**, never guaranteed blackout.

CI tests use fake packet ports and deliberately invalid paths; tests verify cold zero, refusal of unauthorized commands, loss-of-authorization during SHOW, full frame boundaries, repeated USB failure, concurrent call serialization and quarantine. They **do not** open or send packets to the owner's physical adapter.

### Critical remaining safety barriers before enabling

- The current `Authorize` callback is a caller-provided contract, **not** yet implemented with StageCore Snapshot/project ACL and transactional GO/SHOW fencing. This driver must never be directly exposed through Operator HTTP.
- A committed, supervised separate USB output process and bounded independent queue are still needed. USB writes may take up to 800 ms and must never execute synchronously on the Hub GO critical path.
- USB-Pro output packets provide no physical fixture ACK. The device can retain/repeat a previous DMX state after a cable disconnect or process crash; an attempted zero is **not** proof of actual blackout. Measure hotplug/loss-of-power behavior with isolated fixtures before qualifying.
- On-site tests must identify real hardware in `lsusb`, confirm exact `/dev/serial/by-id`, FTDI permissions, and no other application (QLC+ included) owning the adapter.
- Persistent output authority, active snapshot + project assignment, idle/SHOW lock, blackout reconciliation and manual output selection are deferred and must be integrated before any live show deployment.

