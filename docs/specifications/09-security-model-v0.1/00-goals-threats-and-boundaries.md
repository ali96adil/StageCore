# 00 — Goals, Threats & Boundaries

## Goals

StageCore security must:

- work fully on a local stage network without Internet;
- prevent an unknown LAN device from becoming an operator or Companion automatically;
- keep Project/SHOW authority on the Hub;
- make trust and revocation explicit;
- protect passwords, API credentials and private keys from normal project files/logs;
- enforce permissions server-side for Web and native Clients;
- isolate Plugin permissions from Core authority;
- preserve audit evidence for security-sensitive changes;
- avoid security workflows that interrupt normal Cue execution during SHOW.

## Threats Covered in v0.1

The implementation must reasonably defend against:

- an unpaired computer connected to the stage LAN;
- a user attempting an operation above their role;
- a stolen/replaced Companion reconnecting with revoked credentials;
- a rogue endpoint pretending to be a previously trusted Hub;
- replay of stale runtime requests after reconnect;
- a Plugin requesting filesystem/network/secret access it was not granted;
- credentials accidentally appearing in logs, exports or error messages;
- a browser/API session used after explicit revocation;
- accidental exposure of StageCore management interfaces to wider networks.

## Explicit Non-Goals

v0.1 does not claim protection against every physically invasive attack, compromised operating-system administrator/root access, nation-state attackers or certified safety-system threats.

StageCore MVP is not a life-safety system. E-Stops, certified interlocks and required hardware safety layers remain external.

## Trust Rule

`Same LAN` does not mean `Trusted`.

Discovery may reveal non-secret Hub metadata, but any state-changing authority requires authenticated identity plus permission. Physical IP/hostname is routing information, not identity.

## Security vs Availability

Security services used by runtime must remain local and bounded. Cue execution must not call external cloud authentication, online license servers or remote secret services in the P0/P1 path.