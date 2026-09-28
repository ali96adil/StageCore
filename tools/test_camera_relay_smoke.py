"""Host-side integration coverage of the portable four-viewer smoke probe.

The fake endpoint implements only the relay's documented HTTP contract, not
the internal Go code. Run with python3 -m unittest discover -s tools -p
'test_camera_relay_smoke.py' on a machine without cameras or tablets.
"""
import http.server
import pathlib
import socket
import subprocess
import sys
import threading
import time
import unittest

PROBE = pathlib.Path(__file__).with_name("camera-relay-smoke.py")
JPEG = b"\xff\xd8\x42\xff\xd9"
PART = (
    b"--stagecore-relay-frame\r\n"
    b"Content-Type: image/jpeg\r\n"
    b"Content-Length: 5\r\n\r\n"
    + JPEG + b"\r\n"
)


class FakeRelay(http.server.ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, max_clients=4):
        super().__init__(("127.0.0.1", 0), Handler)
        self.lock = threading.Lock()
        self.viewers = 0
        self.max_clients = max_clients
        self.started = time.monotonic()


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, _format, *_args):
        pass

    def do_GET(self):
        if self.path == "/api/v0/health":
            with self.server.lock:
                viewers = self.server.viewers
                received = int((time.monotonic() - self.server.started) * 20)
            import json

            payload = json.dumps({
                "state": "ready",
                "upstream_connected": True,
                "frames_received": received,
                "viewers": viewers,
                "max_clients": self.server.max_clients,
                "last_frame_age_ms": 5,
            }).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return

        if self.path != "/api/v0/stream":
            self.send_error(404)
            return
        with self.server.lock:
            allowed = self.server.viewers < self.server.max_clients
            if allowed:
                self.server.viewers += 1
        if not allowed:
            self.send_error(503, "viewer limit reached")
            return
        try:
            self.send_response(200)
            self.send_header(
                "Content-Type",
                "multipart/x-mixed-replace;boundary=stagecore-relay-frame",
            )
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            while True:
                self.wfile.write(PART)
                self.wfile.flush()
                time.sleep(0.02)
        except (BrokenPipeError, ConnectionResetError, socket.timeout):
            pass
        finally:
            with self.server.lock:
                self.server.viewers -= 1


class CameraRelayProbeTest(unittest.TestCase):
    def run_with_fake(self, max_clients, extra=()):
        fake = FakeRelay(max_clients)
        thread = threading.Thread(target=fake.serve_forever, daemon=True)
        thread.start()
        try:
            process = subprocess.run(
                [
                    sys.executable,
                    str(PROBE),
                    "--base-url", f"http://127.0.0.1:{fake.server_port}",
                    "--seconds", "1",
                    *extra,
                ],
                text=True,
                capture_output=True,
                timeout=30,
                check=False,
            )
            return process
        finally:
            fake.shutdown()
            fake.server_close()
            thread.join(timeout=2)

    def test_four_clients_and_fifth_rejection(self):
        process = self.run_with_fake(4)
        self.assertEqual(0, process.returncode, process.stdout + process.stderr)
        self.assertIn("OVER-LIMIT VIEWER HTTP: 503", process.stdout)
        for number in range(1, 5):
            self.assertIn(f"VIEWER {number}:", process.stdout)
        self.assertIn("===== PASS =====", process.stdout)

    def test_client_limit_failure_is_reported(self):
        process = self.run_with_fake(3)
        self.assertNotEqual(0, process.returncode)
        self.assertIn("===== FAIL =====", process.stdout)

    def test_malformed_base_url_is_rejected(self):
        process = subprocess.run(
            [sys.executable, str(PROBE), "--base-url", "file:///etc/passwd"],
            text=True, capture_output=True, timeout=5, check=False,
        )
        self.assertNotEqual(0, process.returncode)
        self.assertIn("--base-url must be an HTTP relay origin", process.stderr)


if __name__ == "__main__":
    unittest.main()
