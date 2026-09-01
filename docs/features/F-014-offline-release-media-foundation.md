# F-014 — Offline Release Media / Package Path

**Status:** Foundation slice  
**Feature ID:** F-014  
**Phase:** 2 — Installation, diagnostics, discovery, update, and extension operations

## Goal

StageCore must be installable or safely updatable on a supported Linux Hub after one release-media archive has been copied to the host, with no WAN access, source checkout, Go toolchain, package registry, cloud account, or remote bootstrap service required.

F-014 does not create a second installer or updater. It packages and orchestrates the already-tested boundaries:

- F-005 owns fresh/repeat installation;
- F-010 owns update preflight, SHOW blocking, backup, candidate activation, postflight, and automatic rollback;
- F-014 owns offline release-media layout, verification, architecture selection, and delegation.

## Built media

`scripts/build-release.sh` continues to create the architecture-specific release bundles and additionally produces:

```text
stagecore-offline-media/
  RELEASE_REVISION
  MEDIA_CATALOG
  MEDIA_SHA256SUMS
  README.txt
  stagecore-offline
  bundles/
    stagecore-linux-amd64/
      stagecore
      stagecore-hub
      stagecore-osc-plugin
      stagecore-pairing
      stagecore-setup
      install.sh
      RELEASE_REVISION
      SHA256SUMS
    stagecore-linux-arm64/
      ...same canonical F-005 bundle shape...
```

and the portable archive:

```text
stagecore-offline-media.tar.gz
```

The archive is intended to be copied by USB/SSD/LAN file transfer or any other operator-controlled transport and then used locally.

## Catalog v1

`MEDIA_CATALOG` is deliberately line-oriented so the bootstrap path needs only POSIX shell/base Linux tools rather than `jq`, Python, Node, or a package manager.

```text
format=stagecore-offline-media-v1
revision=<git revision>
bundle.linux.amd64=bundles/stagecore-linux-amd64
bundle.linux.arm64=bundles/stagecore-linux-arm64
```

Bundle paths are fixed by the format contract. Absolute paths, traversal paths, alternate bundle roots, and symlinked bundle trees are rejected.

## Operator commands

After extraction:

```bash
./stagecore-offline verify
./stagecore-offline info
./stagecore-offline install
```

For an existing deployment:

```bash
./stagecore-offline update
```

Additional installer/updater options are passed through after media verification. For example:

```bash
./stagecore-offline install --dry-run
./stagecore-offline update --dry-run
```

## Verification boundary

Every command except help verifies the complete media before delegation.

Verification requires:

1. Linux host;
2. valid `stagecore-offline-media-v1` catalog;
3. top-level revision equals catalog revision;
4. amd64 and arm64 bundle paths exactly match the v1 layout;
5. both bundle revisions equal the top-level revision;
6. no symlinks exist anywhere under `bundles/`;
7. checksum-manifest entries are syntactically bounded and cannot use absolute or `..` traversal paths;
8. each nested F-005 `SHA256SUMS` verifies successfully;
9. `MEDIA_SHA256SUMS` verifies the catalog, revision, launcher, README, and every file contained in both release bundles.

The launcher selects only:

- `x86_64` / `amd64` -> `linux/amd64`;
- `aarch64` / `arm64` -> `linux/arm64`.

Other OS/architectures fail with an actionable error rather than guessing.

## Install delegation

`stagecore-offline install` verifies the entire media, selects the matching bundle, then executes that bundle's `install.sh`.

All service identity, paths, configuration preservation, checksum/ELF validation, systemd handling, readiness checks, and fresh/repeat-install semantics remain owned by F-005.

F-014 does not duplicate those rules.

## Update delegation

`stagecore-offline update` verifies the entire media, selects the matching bundle, elevates locally when required, and executes:

```text
stagecore-setup update --bundle <selected-bundle>
```

All update safety remains owned by F-010, including:

- preflight Doctor;
- active SHOW rejection;
- verified rollback snapshot;
- candidate installation;
- readiness/postflight validation;
- automatic rollback on failure.

F-014 must never bypass those gates.

## Offline guarantee

`stagecore-offline` contains no network downloader or package-manager path. It must not invoke:

- `curl`;
- `wget`;
- `apt` / `apt-get`;
- `dnf` / `yum`;
- a package registry;
- Git;
- a compiler/toolchain.

Once the release-media archive is present on a supported target host, StageCore product bytes needed for either supported architecture are already local.

The base supported Linux environment is still expected to provide ordinary system administration/core utilities used by the existing deployment contract, including POSIX shell, `awk`, `find`, `grep`, `sha256sum`, systemd, and `sudo` when the operator is not already root.

## Trust boundary

SHA-256 manifests provide corruption/substitution detection relative to the manifests carried on the same media. They do **not** authenticate OpenAI, GitHub, the repository owner, or a StageCore publisher.

F-014 therefore does not claim a signed supply-chain trust boundary. Publisher signing/key distribution can be added as a separate feature only when its trust lifecycle is explicitly designed and qualified.

## Data and runtime safety

The F-014 launcher itself does not open or mutate the StageCore database, Vault, Project state, security state, runtime state, or extension payloads.

It only verifies release media and delegates to F-005/F-010. Therefore:

- fresh/repeat installation data preservation remains F-005 policy;
- update SHOW gate and rollback remain F-010 policy;
- extension runtime isolation remains F-015 policy;
- no network access is introduced into the SHOW runtime path.

## Software acceptance

Foundation acceptance requires automated tests proving:

- valid offline media verifies successfully;
- product-byte tampering fails verification;
- symlinks under release bundles are rejected;
- checksum-manifest path traversal is rejected before checksum reads;
- an `aarch64` host selects the arm64 bundle;
- build-release creates both architecture entries, catalog, media checksum manifest, launcher, and portable archive;
- the launcher contains no network downloader/package-manager command;
- normal Core CI Test/Vet/Race and Linux ARM64 CGo-free product builds remain green.

## Physical acceptance

Later cumulative Raspberry Pi qualification should build the exact current `main` release, copy/extract `stagecore-offline-media.tar.gz`, and prove on the physical ARM64 host:

1. `./stagecore-offline verify` passes;
2. `./stagecore-offline info` reports the exact revision;
3. `./stagecore-offline install --dry-run` selects arm64 and remains non-mutating;
4. a supported offline update delegates to F-010 and preserves its SHOW/backup/rollback guarantees;
5. the workflow succeeds with target WAN access disabled.

Fresh-media destructive qualification still requires an external rollback copy before reformatting any currently qualified StageCore disk.

## Deliberately deferred

- publisher/signature trust chain;
- remote online release discovery/download;
- distro-native `.deb` / `.rpm` repositories;
- automatic removable-media hotplug execution;
- graphical first-run workflow (F-008);
- automatic update scheduling/policy beyond F-010.
