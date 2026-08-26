# 04 — Clients, Web/API & Transport Security

## Network Exposure

StageCore is local-first and does not expose management/runtime control to the public Internet by default. WAN port forwarding, public cloud relay and remote Internet show control are outside v0.1.

The Hub should bind only to intended local interfaces/configuration. Discovery advertisements contain only non-secret metadata.

## HTTPS / Secure Transport

Production/field Web/API traffic should use HTTPS/TLS. Development may use plain HTTP only on explicit localhost/development profiles.

For local-stage deployment the Hub may generate its own local certificate. Native Clients/Companions additionally verify the stored Hub identity/fingerprint. Browser certificate onboarding is a product UX problem to improve later; it is not a reason to send production credentials unencrypted across the LAN.

## API Authentication

Protected HTTP/API calls require an authenticated user/device session. Authentication and authorization are checked at the Hub before calling domain services.

Realtime channels such as WebSocket must authenticate during establishment and close when the associated session/device trust is revoked.

## Browser Security

Reference requirements:

- same-origin Web UI by default;
- restrictive CORS rather than `*` for authenticated APIs;
- CSRF defense for cookie-authenticated state changes;
- secure cookie attributes;
- input/schema validation on the server;
- no secret/token values embedded in client-visible error traces;
- sensible request/body size limits and authentication rate limits.

## Native Client Security

Native apps store long-lived credentials in platform secure storage, validate the expected Hub identity and do not bypass normal permission checks simply because they are first-party StageCore apps.

## File Downloads

Software/media download authorization follows Storage/Vault policy. Public bootstrap packages may be available before user login only if explicitly classified as non-secret distributable content. Project media, backups and archives are authenticated/authorized resources.

## No Security by Hidden URL

Knowing `stagecore.local`, an API path, Project ID or file path never grants authority by itself.