# F-020 HA-A3 — Authenticated Witness Transport

Status: implementation in progress — software qualification only.

Physical/product qualification remains deferred under GitHub Issue #148.

## Goal

Expose the HA-A2 durable lease authority over a narrow authenticated network boundary without turning transport reachability into automatic Hub promotion authority.

HA-A1 already provides the single physical-dispatch gate inside each Hub. HA-A2 already provides durable holder/epoch/expiry serialization. HA-A3 adds a witness process and mutually authenticated transport so a later Hub-side controller can consume that authority safely.

## Security model

The transport uses TLS 1.3 with explicit StageCore identity pinning rather than public-Web PKI trust or a shared bearer secret.

Each peer owns an Ed25519 transport key. A short-purpose self-signed certificate carries a StageCore identity URI:

- `stagecore://witness/<witness-id>` for the witness;
- `stagecore://hub/<hub-id>` for a Hub.

Trust comes from an explicitly configured SHA-256 fingerprint of the raw Ed25519 public key plus the expected StageCore identity/role. The self-signature is not treated as an external certification authority.

The witness keeps its private key under its own data root with restrictive filesystem permissions. Its stable witness ID and public-key fingerprint survive process restart.

## Hub authorization

The witness starts only with at least one explicit Hub allowlist entry:

`hub_id=SHA256:<public-key-fingerprint>`

The TLS handshake proves possession of the corresponding private key. The server then binds the authenticated certificate identity to the configured Hub ID and fingerprint.

The HTTP request body never supplies lease holder identity. `holder_id` is derived only from the authenticated TLS peer certificate. A caller therefore cannot request a lease on behalf of another configured Hub by changing JSON.

## Witness API

The authenticated API is intentionally small:

- `GET /v1/lease` — inspect current durable lease truth;
- `POST /v1/lease/acquire` — acquire for the authenticated Hub when HA-A2 permits it;
- `POST /v1/lease/renew` — renew the exact authenticated Hub + fencing epoch;
- `POST /v1/lease/release` — release the exact authenticated Hub + fencing epoch.

Requests use bounded bodies and strict JSON decoding. The witness maps HA-A2 conflicts into explicit fail-closed result codes such as `LEASE_HELD`, `LEASE_NOT_HELD`, and `LEASE_EXPIRED`.

Responses expose:

- current holder ID;
- fencing epoch;
- witness-clock expiry;
- active/inactive lease state;
- witness response time.

They do not grant command replay authority.

## Hub-side client trust

The client requires:

- an HTTPS witness URL;
- the local Hub transport certificate;
- expected witness ID;
- expected witness public-key fingerprint.

The client disables ambient proxy routing and pins the witness identity during TLS verification. A wrong witness identity, wrong public-key pin, missing certificate, or transport failure fails closed.

## Clock and expiry safety

HA authority must not depend on synchronized Hub and witness wall clocks.

For each successful lease observation, the witness returns both `witness_time` and `expires_at`. The client computes the remaining lease interval entirely in witness time:

`remaining = expires_at - witness_time`

It then adds that duration to the local request-start instant. This produces a conservative local deadline because the local request began before the witness processed and timestamped the response. Network latency therefore cannot extend the local authority window beyond the witness's lease merely because the two machines have different wall clocks.

A later Hub-side HA controller must stop returning `LEADER` authority at or before this conservative local deadline. Witness errors or an expired local observation must fail closed to non-leader authority.

## Witness product binary

`stagecore-ha-witness` is a standalone optional process. It is deliberately separate from either Hub so the two candidate Hubs do not serialize leadership through one candidate's local runtime database.

Default listen address:

`0.0.0.0:7842`

Supported configuration:

- `--data-root` / `STAGECORE_HA_WITNESS_DATA_ROOT`;
- `--listen` / `STAGECORE_HA_WITNESS_LISTEN`;
- `--lease-duration` / `STAGECORE_HA_WITNESS_LEASE_DURATION`;
- repeated `--authorized-hub hub_id=SHA256:fingerprint`;
- `STAGECORE_HA_WITNESS_AUTHORIZED_HUBS`, with entries separated by `;`.

The lease duration remains witness-owned and bounded by HA-A2. The process refuses to start without an authorized Hub allowlist.

The witness persists:

- its Ed25519 transport identity under its own security directory;
- the HA-A2 lease database under its own database directory.

At startup it logs the witness ID and public-key fingerprint so an operator can configure the future Hub-side trust pin deliberately.

## Software acceptance

HA-A3 is software-acceptable only when CI proves:

1. witness identity and fingerprint survive process restart;
2. witness private-key permissions fail closed when unsafe;
3. authorized Hub certificates can acquire/renew/release HA-A2 leases;
4. only one Hub may hold the active lease;
5. holder identity cannot be spoofed through request JSON;
6. stale fencing epochs remain rejected;
7. unauthorized Hub certificates fail TLS authentication;
8. a wrong witness identity/fingerprint pin fails closed on the client;
9. conservative lease deadlines do not require synchronized wall clocks;
10. witness configuration refuses an empty Hub allowlist and invalid lease duration;
11. the `stagecore-ha-witness` product builds in the Linux ARM64 CGo-free CI gate;
12. exact-head and exact-main Core CI are green.

## Explicit non-goals

HA-A3 does **not** implement:

- Hub product HA configuration or role UI;
- a Hub-side lease-renewal supervisor;
- wiring the witness client into `dispatchauthority.Gate`;
- automatic standby promotion;
- automatic command replay after role change;
- Runtime Snapshot/Session replication between Hubs;
- downstream device enforcement of fencing epochs;
- physical deployment or product qualification.

Those remain later HA slices. In particular, the presence of a secure witness transport does not by itself authorize a standby Hub to take over a live show.
