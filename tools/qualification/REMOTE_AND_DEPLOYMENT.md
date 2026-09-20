# StageCore qualification — pinned Pi deployment and remote execution

## Exact candidate gate

GitHub `main` and the Draft qualification PR can be at different commits.
Choose a **reviewed, exact SHA** with successful exact-head CI and record it before
building release media. Do not assume the most recent repository commit has been
deployed. A GitHub CI success is not a Raspberry Pi physical qualification PASS.

The runner checks `go version -m /opt/stagecore/bin/stagecore-hub` over read-only
SSH and requires the installed `vcs.revision` to equal its local Git HEAD.
Missing/modified/mismatched build metadata BLOCKS physical evidence. This
guard does not itself install or modify the Pi. Use the supported F-010 update
path, rather than replacing binaries with `scp` or reinstalling into live data.

## Private remote access (one-time setup)

Use a private network path between the Mac and the Pi. One practical option is
Tailscale installed and authenticated on **both** machines: keep the existing
system OpenSSH on the Pi, reach it through its authenticated tailnet IP or
MagicDNS name, and use the dedicated StageCore qualification SSH key.

On the Pi, install Tailscale using the current official instructions for its
Linux distribution, then authorize it with `sudo tailscale up`. On the Mac,
install and sign in to the same tailnet. Check `tailscale status`, then verify
normal `ssh user@<pi-tailnet-ip>` from an external network. Only after
successful authentication, put that private IP/name into
`~/.config/stagecore/qualification.env` as `STAGECORE_PI_HOST`, with
`STAGECORE_PI_USER` set to the actual Pi user. No public port forwarding, open
WAN SSH or public Hub/qualification socket is required.

Alternative: a **dedicated** Cloudflare Access-protected SSH Tunnel with a
client-side `cloudflared` ProxyCommand in `~/.ssh/config`. An existing Home
Assistant Cloudflared add-on does not imply the StageCore Pi SSH service has
been published or authorized. Do not publish the Hub Operator/device gateway
or its root-only qualification Unix socket as a substitute for private SSH.

Remote execution requires that the Pi, its home internet, its SSH/private
network agent, and the Mac-side connection are all online. It cannot physically
switch a tablet/ESP32 on or inspect the lights/camera remotely by itself.

## Supported release and one-time qualification bootstrap

With the intended branch checked out on the Mac, clean working tree, and
exact-head CI PASS:

```bash
git rev-parse HEAD
git status --short
bash scripts/build-release.sh
```

Transfer `dist/stagecore-offline-media.tar.gz` over the private SSH path.
Unpack on the Pi into a new temporary directory. From the unpacked
`stagecore-offline-media` directory, use:

```bash
./stagecore-offline verify
./stagecore-offline info
./stagecore-offline update --dry-run
./stagecore-offline update
go version -m /opt/stagecore/bin/stagecore-hub
```

F-010 checks existing Doctor/SHOW state, protects the managed rollback
snapshot, and verifies readiness on update. Stop on any BLOCKED/error and
investigate before rerunning. Do not overwrite `/var/lib/stagecore`, replace
`stagecore.env`, force a migration, or manually delete rollback state.

After the candidate is installed, on the Mac run the one-time
`bash tools/qualification/setup-access.sh` and
`bash tools/qualification/bootstrap-pi-access.sh` to install the
bounded root-owned qualification helper. Bootstrap deliberately restarts
the Hub once, so do not use it during a show. Provide Operator credentials
using the existing hidden-prompt helper; never put passwords in Git/chat.

## Powered-off hardware is not a product defect

The local-only `qualification.env` supports:

```text
STAGECORE_TABLET_EXPECT_ON=0
STAGECORE_LIGHTING_EXPECT_ON=0
```

Use `0` **only** while the corresponding target is intentionally turned off.
With `0` or `unknown`, absent/offline/stale readiness becomes
`BLOCKED / DEFERRED_OFFLINE`, not FAIL. `unknown` is the safe default because
the Pi cannot independently observe actual device power. Configuration,
protocol and capability mismatches remain genuine failures when their
authoritative evidence exists.

After an operator confirms that the real device has been powered on, set
the corresponding value to `1` and run `retry`. Then missing/stale
ONLINE/READY evidence is a genuine FAIL. A matching online, healthy device
is tested even if its local flag still says `0`; the flag does not override
actual fresh device evidence. If a previously recorded FAIL arose from a
different baseline, explicitly invalidate affected gates and required
regression rather than silently relabeling it.

Use from the Mac on the StageCore qualification checkout, at home or remotely:

```bash
bash tools/qualification/qualify.sh run
bash tools/qualification/qualify.sh retry
bash tools/qualification/qualify.sh status
bash tools/qualification/qualify.sh issues
```

No device-output-changing action is armed automatically. Physically observe
any AUTO_PHYSICAL/MANUAL gate before approving it in campaign state. The
progress dashboard counts manifest gates exactly once and never considers
a deferred/offline device a PASS. If a Pi SHA changes, repin and invalidate
the affected acceptance gates; preserve old evidence in history.
