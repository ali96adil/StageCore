# 01 — Hub Identity & First-Run Bootstrap

## Stable Hub Identity

On first initialization the Hub creates:

- a random stable `hub_id`;
- a Hub asymmetric identity key pair;
- a human-readable display name;
- a public-key fingerprint;
- bootstrap state: `UNCLAIMED | CLAIMED`.

The private identity key is stored in protected Hub-local security storage and is never included in ordinary Project export.

Changing IP address, router or hostname does not change `hub_id`.

## First Owner Bootstrap

An unclaimed Hub exposes only a restricted setup flow. Reference MVP flow:

```text
start new Hub
 -> Hub enters UNCLAIMED
 -> operator obtains one-time setup code locally
 -> open setup page
 -> verify Hub name/fingerprint
 -> enter setup code
 -> create first OWNER account
 -> Hub becomes CLAIMED
 -> setup code is invalidated
```

For headless prototype hardware the code may be obtained with a local command such as `stagecore setup-code`. A future appliance can display/QR it physically.

## Setup Code Rules

- generated from secure random data;
- short-lived;
- single-use;
- rate-limited;
- never emitted to normal runtime logs;
- invalid after Hub is claimed unless an explicit local recovery flow resets bootstrap state.

## Hub Verification

Clients/Companions retain the trusted Hub ID/fingerprint after first trust. If a known hostname/IP later presents a different Hub identity, StageCore reports **IDENTITY MISMATCH** and does not silently trust the new endpoint.

## Recovery

Loss of Hub identity keys is a security recovery event, not a normal rename. Restoring a Full/System backup must either restore the original protected Hub identity deliberately or create a new Hub identity that requires Clients/Companions to re-establish trust.