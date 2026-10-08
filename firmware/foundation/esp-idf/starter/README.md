# New StageCore ESP-IDF device starter

1. Copy/consume `firmware/foundation/esp-idf/components/stagecore_foundation`.
2. Initialize NVS before constructing `stagecore::FoundationStore`.
3. Choose a stable device NVS namespace (15 characters or fewer) and never change it
   after deployment.
4. Load/create `DeviceIdentity`.
5. Use the device's own Wi-Fi provisioning/recovery code, but obtain the effective AP
   password from `FoundationStore::EffectiveSetupAPPassword()`.
6. Discover the Hub with `discover_and_verify_hub(&store, &hub)`.
7. Build a `FoundationDeviceDescriptor` and authenticate with
   `ensure_paired_and_authenticate(...)`.
8. Advertise `stagecore::kSetupAPPasswordCapability` only when the exact
   authenticated `stagecore.device/2` maintenance command is implemented.
9. Keep all GPIO/output/Cue behavior in device code. Foundation maintenance must not
   arm, enable or mutate show outputs.
10. Add CI proving identity persistence, fail-closed trust binding, SET/RESET_DEFAULT,
    no password disclosure, recovery AP behavior and the device-specific safe state.

Minimal descriptor example:

```cpp
stagecore::FoundationDeviceDescriptor descriptor{
    .hostname_prefix = "stagecore-example-",
    .platform = "esp32",
    .architecture = "xtensa",
    .firmware_version = STAGECORE_FW_VERSION,
    .capabilities = {stagecore::kSetupAPPasswordCapability},
};
```

For ESP32-C3 use the real architecture string used by that firmware rather than
copying `xtensa`.
