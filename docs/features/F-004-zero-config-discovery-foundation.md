# F-004 — Zero-configuration Discovery, Pairing & Reconnect

**Status:** Foundation slice in implementation  
**Feature ID:** F-004  
**Phase:** 2 — Installation, diagnostics, discovery, update, and extension operations

## Goal

A normal StageCore Companion should not require an operator to discover, type, remember, or repair a Hub IP address. The Hub advertises a stable service identity on the local Stage Network, the Companion discovers it, verifies the secure endpoint, keeps the existing explicit pairing/approval boundary, remembers the Hub identity, and reconnects to that identity after address changes.

Arabic operator definition:

> **الاكتشاف والاتصال التلقائي:** يجد StageCore الـHub على شبكة المسرح المحلية تلقائياً، يثبت هويته، ويحفظ ارتباط الجهاز به حتى لا يحتاج المستخدم إلى إدخال أو تعديل عنوان IP عند تغيّره.

The foundation is intentionally local-first. Internet/cloud service discovery is out of scope.

## Core safety rule

**Discovery is not trust.**

Bonjour/mDNS only says that a network participant is advertising a StageCore service. It must never by itself grant pairing, credentials, runtime authority, device assignment, or configuration changes.

The accepted lifecycle remains:

```text
Discover
  -> verify the Hub secure endpoint and advertised identity
  -> explicit Companion pairing request
  -> local Hub approval
  -> authenticated Companion session
  -> remembered Hub identity
  -> reconnect only to that remembered identity
```

This preserves the architecture rule `discover -> verify Hub identity -> approve -> issue credential`.

## Foundation network contract

### Local operator HTTP

The existing local Hub listener remains unchanged by this feature:

```text
STAGECORE_LISTEN=127.0.0.1:7840
```

F-004 does not expose the Operator Web UI over the Stage LAN merely to solve Companion discovery.

### Secure Device Gateway

A separate TLS-only device listener is introduced for Companion/device traffic:

```text
STAGECORE_DEVICE_LISTEN=0.0.0.0:7841
```

The Device Gateway exposes only the device-facing surface required by this slice:

- public Hub identity metadata;
- Companion pairing/authentication;
- authenticated Companion WebSocket runtime;
- authenticated Vault object delivery used by Companion media sync.

It does not become a second Operator Web server.

The gateway uses a self-signed StageCore device certificate whose key is the existing durable Hub Ed25519 identity key. The certificate bytes are deterministic for that Hub identity, so its SHA-256 pin is stable across normal restarts. This is a local identity/pinning mechanism, not a public Web PKI certificate.

### Bonjour / mDNS

The Hub advertises:

```text
_stagecore-hub._tcp.local.
```

TXT schema v1 carries bounded public metadata:

```text
v=1
hub_id=<stable Hub UUID>
name=<human-readable Hub name>
hub_fp=<existing Hub identity fingerprint>
tls_sha256=<SHA-256 of the deterministic device certificate DER>
host=<stable per-Hub .local hostname>
port=<secure device port>
api_path=/
runtime_path=/api/v1/companion/runtime
```

No secret, session token, pairing code, password, private key, project name, media path, or user information is advertised.

The initial Hub advertiser is bounded local IPv4 mDNS/DNS-SD. IPv6 discovery is a compatible later extension and must not change the identity model.

Discovery advertising is deliberately outside the critical show-control path. If multicast advertisement cannot start, the Hub remains operational and logs a diagnostic warning; manual/remembered addressing remains a recovery path.

## Hub identity verification

The Device Gateway exposes:

```text
GET /api/v1/hub/identity
```

with public fields only:

```json
{
  "schema_version": 1,
  "hub_id": "...",
  "display_name": "StageCore Hub",
  "fingerprint": "SHA256:...",
  "bootstrap_state": "CLAIMED"
}
```

A discovered Companion first pins the advertised device TLS certificate, then reads this endpoint and requires the returned Hub ID/fingerprint to match the discovery record. Pairing approval is still required before authenticated runtime use.

For an already remembered Hub, reconnect requires the same Hub ID, Hub fingerprint, and device-certificate pin. A service with the same display name but different identity is never silently substituted.

## Companion configuration model

Existing explicit configurations remain compatible:

```text
hubAPIBaseURL
hubRuntimeURL
```

F-004 adds an optional remembered binding:

```text
hubID
hubFingerprint
tlsCertificateSHA256
```

Behavior:

1. **Existing manual config without a binding:** preserve existing behavior.
2. **No config and one valid discovered Hub:** verify it, create a remembered binding, then enter the existing pairing flow.
3. **Remembered binding:** rediscover the same Hub identity on every launch and refresh URLs when its address changes.
4. **Connection loss:** the Companion keeps the existing bounded WebSocket retry behavior; after that bounded attempt fails, the app shell rediscovery loop resolves the remembered Hub again instead of requiring an IP edit.
5. **No matching Hub:** remain disconnected with an actionable error; never fall over to a different Hub merely because it has the same name.
6. **Multiple unpaired Hubs:** do not guess. A future native UI may present a safe chooser; the foundation reports that selection is required.

## Security boundaries

- LAN pairing/auth/runtime remain TLS-only in production.
- Existing Companion public-key pairing and signed challenge authentication are unchanged.
- Existing Hub approval/revocation/session expiry semantics are unchanged.
- mDNS records are treated as untrusted input and are length/format validated.
- A discovered certificate pin is verified before public Hub identity metadata is accepted.
- A remembered Hub binding is identity-based, not IP/hostname-based.
- Reconnect never replays a previous runtime command.
- Hub/device private keys and credentials never enter Bonjour records or Companion config.

## Failure behavior

The feature must distinguish:

- no Hub discovered;
- multiple first-pair candidates;
- malformed discovery record;
- remembered Hub not currently present;
- TLS certificate pin mismatch;
- Hub identity mismatch;
- secure endpoint unavailable;
- normal authenticated reconnect failure.

A discovery failure is never permission to downgrade to insecure LAN HTTP/WS.

## F-005 integration

The repeatable installer gains the managed setting:

```text
STAGECORE_DEVICE_LISTEN=0.0.0.0:7841
```

Repeated installs preserve an existing value when present. Older managed configurations without the key adopt the safe F-004 default rather than becoming invalid.

The Device Gateway does not require an Internet service, external CA, Avahi daemon, or manually managed certificate file.

## Localization contract

User-facing feature definition and principal actions ship in both required product languages in the same change:

| Stable meaning | English | العربية `ar-IQ` |
|---|---|---|
| feature | Automatic Hub Discovery | اكتشاف الـHub تلقائياً |
| discover | Find StageCore Hub | العثور على StageCore Hub |
| verify | Verify Hub identity | التحقق من هوية الـHub |
| pair | Pair this Companion | إقران هذا الـCompanion |
| reconnect | Reconnect automatically | إعادة الاتصال تلقائياً |
| no hub | No matching StageCore Hub found | لم يتم العثور على StageCore Hub مطابق |
| multiple hubs | More than one StageCore Hub was found | تم العثور على أكثر من StageCore Hub |
| identity mismatch | Hub identity does not match the remembered Hub | هوية الـHub لا تطابق الـHub المحفوظ |

This foundation adds no fake Operator Web button. When F-004 receives a browser/native selection or status UI, that surface must use keyed localization under the existing Feature Localization Contract.

## Software acceptance

Before merge:

- Hub device certificate is deterministic and tied to the durable Hub identity;
- public Hub identity endpoint returns only expected identity metadata;
- Device Gateway uses TLS and does not expose Operator Web routes;
- discovery record/TXT generation is deterministic and bounded;
- mDNS packet tests verify service PTR/SRV/TXT/A identity data;
- config validates the device listener;
- F-005 render/preserve tests cover `STAGECORE_DEVICE_LISTEN`;
- Companion discovery record parser rejects malformed/untrusted values;
- remembered Hub matching is identity-based;
- legacy manual Companion config remains decodable/usable;
- TLS pin helper has deterministic SHA-256 tests;
- Core CI Test/Vet/Race/ARM64 gates pass;
- Companion Core CI Linux build/tests and macOS acceptance remain green.

## Physical acceptance

Physical Stage LAN acceptance is intentionally separate from software merge. When the qualified Raspberry Pi and macOS Companion are available together:

1. install/deploy the build through the supported path;
2. confirm `_stagecore-hub._tcp` appears without manual IP entry;
3. pair from a clean Companion identity through the discovered secure gateway;
4. confirm the authenticated runtime path works;
5. change the Hub IP (or move between DHCP leases) without changing Hub identity;
6. confirm the remembered Companion rediscovers and reconnects to the same Hub;
7. confirm another fake/different Hub identity is not silently substituted.

This physical gate extends the existing M0–M6 qualification; it does not require rerunning unrelated hardware tests.

## Deliberately deferred

- native graphical Hub chooser for multiple first-pair candidates;
- Android/tablet discovery adoption (F-003 will consume the same service contract);
- StageCore Node discovery;
- IPv6 mDNS advertisement;
- certificate/key rotation UX;
- cross-subnet/routed discovery;
- cloud discovery;
- Device Profile auto-configuration (F-021);
- Stage Network Cockpit visualization (F-022).
