# M6 S1 — Authenticated Browser Transport — COMPLETE

Date: 2026-08-27
Branch: `m6/operator-security-closure`
Parent milestone: Issue #25 / PR #28

## Proven

- local username/password login against Argon2id hashes created by SEC0;
- bounded opaque browser sessions stored by SHA-256 digest;
- `HttpOnly` + `SameSite=Strict` session cookie, `Secure` when TLS is active;
- separate CSRF token with server-side digest validation for state-changing cookie-authenticated requests;
- server-side OWNER / TECHNICIAN / OPERATOR / VIEWER permission bundles;
- unauthorized, wrong-role, bad-CSRF, bad-origin, expired/revoked-session and rate-limit denial paths;
- basic failed-login throttling keyed by peer IP rather than source port;
- authenticated SSE browser channel that revalidates session authority and closes after revocation;
- Hub ID/fingerprint/status exposed as non-secret local identity metadata;
- plaintext browser auth accepted only on loopback development transport; non-loopback requires TLS/security transport baseline;
- no browser session bearer token returned in JSON.

## Verification

Core CI #174 — PASS:

- Go 1.26 tests — PASS
- Go 1.26 vet — PASS
- Go 1.26 race — PASS
- Linux ARM64 CGo-free product builds — PASS
- Go 1.27 tests/vet — PASS
- module lock — PASS

S1 does not add Operator product UI. That begins in S2.
