#!/usr/bin/env python3
"""Read-only F-010 rollback checksum audit; NEVER restore, create or prune.

Implements internal/update/snapshot.go treeDigest exactly:
sorted entries; kind/NUL/relative/NUL/octal-mode/NUL, file bytes, trailing NUL.
Does not read or display snapshot contents, IDs, credentials or source paths.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import sys

DEFAULT_ROOT = Path("/var/backups/stagecore/updates")
EXPECTED_ITEMS = {"managed-binaries", "deployment-config", "data-root", "systemd-unit"}


def safe_regular(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode):
        raise ValueError("invalid file type in rollback payload")
    return info


def safe_dir(path):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode):
        raise ValueError("invalid directory type in rollback payload")
    return info


def digest_tree(root):
    """Byte-compatible with the Go Snapshotter treeDigest."""
    safe_root = Path(root)
    root_info = safe_root.lstat()
    if not stat.S_ISDIR(root_info.st_mode) and not stat.S_ISREG(root_info.st_mode):
        raise ValueError("non-regular snapshot root")
    nodes = [(safe_root, root_info)]
    if stat.S_ISDIR(root_info.st_mode):
        stack = [safe_root]
        while stack:
            directory = stack.pop()
            for child in directory.iterdir():
                info = child.lstat()
                if not stat.S_ISDIR(info.st_mode) and not stat.S_ISREG(info.st_mode):
                    raise ValueError("symlink or non-regular file in payload")
                nodes.append((child, info))
                if stat.S_ISDIR(info.st_mode):
                    stack.append(child)
    nodes.sort(key=lambda node: os.fsencode(str(node[0])))
    digest = hashlib.sha256()
    files = size = 0
    for path, info in nodes:
        rel = "." if path == safe_root else path.relative_to(safe_root).as_posix()
        kind = "D" if stat.S_ISDIR(info.st_mode) else "F"
        digest.update((kind + "\0" + rel + "\0" +
                       format(stat.S_IMODE(info.st_mode), "o") + "\0").encode())
        if kind == "F":
            with path.open("rb") as stream:
                while True:
                    block = stream.read(1024 * 1024)
                    if not block:
                        break
                    digest.update(block)
                    size += len(block)
            files += 1
        digest.update(b"\0")
    return digest.hexdigest(), files, size


def verify_snapshot(folder):
    folder = Path(folder)
    safe_dir(folder)
    raw = (folder / "manifest.json")
    info = safe_regular(raw)
    if info.st_size > 256 * 1024:
        raise ValueError("oversized rollback manifest")
    manifest = json.loads(raw.read_text(encoding="utf-8"))
    if not isinstance(manifest, dict) or manifest.get("schema_version") != 1:
        raise ValueError("invalid rollback manifest schema")
    if not isinstance(manifest.get("snapshot_id"), str) or not manifest["snapshot_id"]:
        raise ValueError("missing snapshot identity")
    if manifest["snapshot_id"] != folder.name:
        raise ValueError("rollback folder/manifest identity mismatch")
    items = manifest.get("items")
    if not isinstance(items, list) or len(items) != 4 or {x.get("name") for x in items if isinstance(x, dict)} != EXPECTED_ITEMS:
        raise ValueError("unexpected rollback item inventory")
    verified = 0
    total_bytes = 0
    for item in items:
        if not isinstance(item, dict) or item.get("payload") != item["name"]:
            raise ValueError("unsafe rollback item")
        payload = folder / item["payload"]
        if payload.is_symlink():
            raise ValueError("unsafe payload symlink")
        computed, files, size = digest_tree(payload)
        if (computed != item.get("tree_sha256") or
                files != item.get("regular_files") or size != item.get("size_bytes")):
            raise ValueError("rollback tree digest/size mismatch")
        verified += 1
        total_bytes += size
    return {"verified_items": verified, "verified_bytes": total_bytes,
            "status": "TREE_DIGESTS_MATCH_NOT_RESTORE_TEST"}


def latest(root):
    root = Path(root)
    safe_dir(root)
    snapshots = []
    for path in root.iterdir():
        if path.name.startswith("."):
            continue
        safe_dir(path)
        snapshots.append(path)
    if not snapshots:
        raise ValueError("no rollback snapshot directories")
    return max(snapshots, key=lambda path: path.stat().st_mtime), len(snapshots)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default=str(DEFAULT_ROOT))
    args = parser.parse_args()
    root = Path(args.root)
    newest, total = latest(root)
    result = verify_snapshot(newest)
    print("ROLLBACK_SNAPSHOT_COUNT:", total)
    print("LATEST_ROLLBACK:", result["status"])
    print("VERIFIED_ITEMS:", result["verified_items"])
    print("VERIFIED_PAYLOAD_BYTES:", result["verified_bytes"])
    print("RESTORE_TEST: NOT_EXECUTED")
    print("ROLLBACK_READONLY_AUDIT_COMPLETE: no Hub, state or payload mutation")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, TypeError, KeyError, UnicodeError) as exc:
        print("ROLLBACK_AUDIT_STOP:", type(exc).__name__, file=sys.stderr)
        raise SystemExit(3)
