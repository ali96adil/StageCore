"""Independent fixture checks for immutable Go-compatible F-010 snapshot audit."""
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import verify_rollback as rollback


def independent_digest(kind, relative, mode, data=b""):
    h = hashlib.sha256()
    h.update((kind + "\0" + relative + "\0" + format(mode, "o") + "\0").encode())
    h.update(data)
    h.update(b"\0")
    return h.hexdigest()


def fixture(root):
    root.mkdir(mode=0o700)
    snap = root / "20260920T120557Z"
    snap.mkdir(mode=0o700)
    items = []
    for name in ("managed-binaries", "deployment-config", "data-root", "systemd-unit"):
        path = snap / name
        content = ("private-test-" + name).encode()
        path.write_bytes(content)
        path.chmod(0o600)
        items.append({"name": name, "source": "/private/" + name,
                      "payload": name, "tree_sha256": independent_digest("F", ".", 0o600, content),
                      "regular_files": 1, "size_bytes": len(content)})
    (snap / "manifest.json").write_text(json.dumps({
        "schema_version": 1, "snapshot_id": snap.name,
        "items": items,
    }), encoding="utf-8")
    return snap


class RollbackAuditTest(unittest.TestCase):
    def test_byte_compatible_file_and_nested_directory_digest(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td) / "nested"
            root.mkdir(mode=0o700)
            (root / "a.txt").write_bytes(b"abc")
            (root / "a.txt").chmod(0o600)
            root.chmod(0o700)
            actual, files, size = rollback.digest_tree(root)
            h = hashlib.sha256()
            h.update(b"D\0.\0700\0\0")
            h.update(b"F\0a.txt\0600\0abc\0")
            self.assertEqual(actual, h.hexdigest())
            self.assertEqual((files, size), (1, 3))

    def test_verify_snapshot_and_no_restoration(self):
        with tempfile.TemporaryDirectory() as td:
            folder = fixture(Path(td) / "backup")
            data = rollback.verify_snapshot(folder)
            self.assertEqual(data["verified_items"], 4)
            self.assertEqual(data["status"], "TREE_DIGESTS_MATCH_NOT_RESTORE_TEST")
            self.assertEqual(rollback.latest(folder.parent)[1], 1)
            self.assertEqual(rollback.verify_snapshot(folder), data)

    def test_modified_payload_and_symlink_are_rejected(self):
        with tempfile.TemporaryDirectory() as td:
            folder = fixture(Path(td) / "backup")
            (folder / "data-root").write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "digest/size mismatch"):
                rollback.verify_snapshot(folder)
            (folder / "data-root").unlink()
            (folder / "data-root").symlink_to(folder / "systemd-unit")
            with self.assertRaisesRegex(ValueError, "symlink"):
                rollback.verify_snapshot(folder)

    def test_manifest_identity_missing_items_and_unsafe_root_fail_closed(self):
        with tempfile.TemporaryDirectory() as td:
            folder = fixture(Path(td) / "backup")
            manifest = folder / "manifest.json"
            data = json.loads(manifest.read_text())
            data["snapshot_id"] = "other"
            manifest.write_text(json.dumps(data))
            with self.assertRaisesRegex(ValueError, "identity mismatch"):
                rollback.verify_snapshot(folder)
            data["snapshot_id"] = folder.name
            data["items"].pop()
            manifest.write_text(json.dumps(data))
            with self.assertRaisesRegex(ValueError, "item inventory"):
                rollback.verify_snapshot(folder)
            with self.assertRaises(ValueError):
                rollback.latest(folder / "systemd-unit")


if __name__ == "__main__":
    unittest.main()
