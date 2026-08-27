# SPK-05 — Vault & Large File Transfer Prototype

This executable spike validates the first StageCore managed-file/Vault transfer boundary.

## What it proves

- managed import writes to `staging/`, calculates SHA-256 while streaming, then atomically promotes the verified object;
- content identity is checksum-based, not filename-based;
- identical bytes resolve to the same object identity/path;
- HTTP object delivery supports `Range: bytes=<offset>-` and resumable download;
- the client keeps a `.part` file, verifies full SHA-256, then atomically promotes the local cache file;
- corrupted/incomplete content never becomes the final cache file;
- bulk transfer uses bounded buffers instead of loading the full media object into memory;
- entering `SHOW` pauses active bulk transfer at chunk boundaries while `/runtime/ping` remains responsive;
- leaving `SHOW` resumes the paused transfer;
- capacity admission can reject a bulk write that would breach the configured runtime storage reserve.

## Run

```bash
go test ./...
go test -race ./internal/transfer ./internal/vault
go build -o /tmp/spk05-demo ./cmd/vault-demo
SPK05_SIZE_MB=256 /tmp/spk05-demo
```

The manual 256 MiB run completed import + HTTP transfer + checksum verification with a measured process peak RSS of roughly 8.5 MiB in the validation environment. This is evidence for bounded streaming behavior, not a final production memory guarantee.

## Important Limitations

The repository spike uses a Go test client, not the final Swift Companion media-cache implementation. The HTTP Range/checksum contract is intentionally language-neutral and will be consumed by CompanionCore later.

A 2 GiB interrupted-transfer test remains required by the Storage & Vault acceptance specification on the selected Hub storage hardware. This spike validates the mechanism with smaller repeatable tests plus a 256 MiB manual run so normal CI does not allocate multi-gigabyte temporary files.
