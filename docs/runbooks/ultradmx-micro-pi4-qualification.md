# SC-025: DMXking ultraDMX Micro (0084) — Pi 4 USB-DMX qualification

**Status: protocol-only Draft, NO live serial writes, NO device registration, NOT a SHOW-ready output.** The main StageCore Hub and the existing ESP32 DMX Node remain unchanged.

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

Only pure framing, strict bounds and read-only serial discovery with tests. There is **no** serial writer/reader, no `stty`, no udev rule, no USB port lock, no device pairing endpoint, no runtime registration, no automatic DMX output, no Pi deployment.
