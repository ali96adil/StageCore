#!/usr/bin/env python3
"""Read-only four-viewer MJPEG qualification of a running StageCore Camera Relay.

Runs on the Pi or a Mac on the same trusted show LAN. This does not configure
the camera, alter the Hub, open ports or certify the four Android applications.
"""
import argparse
import concurrent.futures
import json
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


def request_health(url):
    try:
        with urllib.request.urlopen(url, timeout=4) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as exc:
        return exc.code, json.load(exc)


def read_jpeg(response):
    """Parse a bounded Content-Length MJPEG part (no image decoding required)."""
    while True:
        line = response.readline(256)
        if not line:
            raise EOFError("MJPEG stream ended")
        if line.strip() == b"--stagecore-relay-frame":
            break
    headers = {}
    while True:
        line = response.readline(512)
        if not line:
            raise EOFError("Incomplete part headers")
        if line in (b"\r\n", b"\n"):
            break
        key, separator, value = line.partition(b":")
        if not separator:
            raise ValueError("Malformed part header")
        headers[key.strip().lower()] = value.strip()
        if len(headers) > 16:
            raise ValueError("Too many part headers")
    if headers.get(b"content-type") != b"image/jpeg":
        raise ValueError("Expected image/jpeg part")
    length = int(headers[b"content-length"])
    if not 4 <= length <= 4 * 1024 * 1024:
        raise ValueError("JPEG length outside probe limit")
    frame = response.read(length)
    if len(frame) != length or not frame.startswith(b"\xff\xd8") or not frame.endswith(b"\xff\xd9"):
        raise ValueError("Invalid JPEG frame")
    return length


def viewer(number, stream_url, start, stop, ready):
    start.wait()
    count = 0
    byte_count = 0
    with urllib.request.urlopen(stream_url, timeout=8) as response:
        if response.status != 200:
            raise RuntimeError(f"Viewer {number}: HTTP {response.status}")
        mime = response.headers.get("Content-Type", "")
        if not mime.startswith("multipart/x-mixed-replace;"):
            raise RuntimeError(f"Viewer {number}: invalid content type")
        ready[number - 1].set()
        while not stop.is_set():
            byte_count += read_jpeg(response)
            count += 1
    return number, count, byte_count


def run(base_url, viewers, duration):
    base_url = base_url.rstrip("/")
    health_url = base_url + "/api/v0/health"
    stream_url = base_url + "/api/v0/stream"
    print("===== RELAY VIEWER QUALIFICATION =====", flush=True)
    print(f"Viewers: {viewers}; sample duration: {duration} s", flush=True)
    start = threading.Event()
    stop = threading.Event()
    ready = [threading.Event() for _ in range(viewers)]
    errors = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=viewers) as pool:
        futures = [
            pool.submit(viewer, i, stream_url, start, stop, ready)
            for i in range(1, viewers + 1)
        ]
        start.set()
        try:
            deadline = time.monotonic() + 12
            while time.monotonic() < deadline:
                if any(f.done() for f in futures):
                    break
                if all(event.is_set() for event in ready):
                    break
                time.sleep(0.1)
            if not all(event.is_set() for event in ready):
                raise RuntimeError("Not all viewers connected within 12 seconds")
            deadline = time.monotonic() + 6
            status_code, health = 0, {}
            while time.monotonic() < deadline:
                status_code, health = request_health(health_url)
                if health.get("viewers") == viewers and health.get("state") == "ready":
                    break
                time.sleep(0.2)
            print("HEALTH BEFORE:", json.dumps(health, sort_keys=True), flush=True)
            if status_code != 200 or health.get("viewers") != viewers:
                raise RuntimeError("Relay not ready with expected number of viewers")
            if not health.get("upstream_connected"):
                raise RuntimeError("Camera upstream is disconnected")
            before = health.get("frames_received", 0)
            time.sleep(duration)
            status_code, health_after = request_health(health_url)
            delta = health_after.get("frames_received", 0) - before
            print("HEALTH AFTER:", json.dumps(health_after, sort_keys=True), flush=True)
            print(f"RECEIVED IN {duration}s: {delta} frames", flush=True)
            if status_code != 200 or delta <= 0 or health_after.get("viewers") != viewers:
                raise RuntimeError("Relay lost viewer/source readiness or stopped receiving frames")
            if viewers == health_after.get("max_clients"):
                try:
                    with urllib.request.urlopen(stream_url, timeout=4) as response:
                        fifth_status = response.status
                except urllib.error.HTTPError as exc:
                    fifth_status = exc.code
                print("OVER-LIMIT VIEWER HTTP:", fifth_status, flush=True)
                if fifth_status != 503:
                    raise RuntimeError("Additional viewer was not rejected with HTTP 503")
        except Exception as exc:
            errors.append(f"CHECK FAILED: {type(exc).__name__}: {exc}")
        finally:
            stop.set()
        for future in futures:
            try:
                number, count, byte_count = future.result(timeout=12)
                print(f"VIEWER {number}: {count} JPEG frames, {byte_count} bytes", flush=True)
                if count == 0:
                    errors.append(f"Viewer {number} did not receive any JPEG frames")
            except Exception as exc:
                errors.append(f"VIEWER FAILED: {type(exc).__name__}: {exc}")
    for message in errors:
        print(message, flush=True)
    print("===== " + ("PASS" if not errors else "FAIL") + " =====", flush=True)
    return 0 if not errors else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:9081",
                        help="Relay origin only, e.g. http://192.168.3.130:9081")
    parser.add_argument("--viewers", type=int, default=4)
    parser.add_argument("--seconds", type=int, default=5)
    args = parser.parse_args()
    url = urllib.parse.urlsplit(args.base_url)
    if (url.scheme != "http" or not url.hostname or url.username is not None
            or url.password is not None or url.query or url.fragment or
            url.path not in ("", "/")):
        parser.error("--base-url must be an HTTP relay origin without credentials or path")
    if not 1 <= args.viewers <= 4 or not 1 <= args.seconds <= 30:
        parser.error("--viewers must be 1-4 and --seconds must be 1-30")
    return run(args.base_url, args.viewers, args.seconds)


if __name__ == "__main__":
    raise SystemExit(main())
