#!/usr/bin/env python3
import argparse
import datetime as dt
import http.cookiejar
import json
import math
import os
import re
import sqlite3
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

CHANNEL_KEY = re.compile(r"^[a-z][a-z0-9_]{0,63}$")
SENSITIVE_PARTS = ("password", "secret", "token", "cookie", "authorization", "csrf")
TERMINAL = {"REJECTED", "COMPLETED", "FAILED", "TIMED_OUT", "CANCELLED"}


def redact(value):
    if isinstance(value, dict):
        out = {}
        for key, item in value.items():
            lowered = str(key).lower()
            out[key] = "[REDACTED]" if any(part in lowered for part in SENSITIVE_PARTS) else redact(item)
        return out
    if isinstance(value, list):
        return [redact(item) for item in value]
    return value


def die(message, code=1):
    print(json.dumps({"status": "ERROR", "detail": message}, sort_keys=True))
    raise SystemExit(code)


def bounded_text(value, name, maximum=256):
    value = str(value or "").strip()
    if not value or len(value) > maximum or any(ord(ch) < 32 for ch in value):
        die(f"invalid {name}")
    return value


def level(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        die(f"{name} must be numeric")
    value = float(value)
    if not math.isfinite(value) or value < 0 or value > 100:
        die(f"{name} must be within 0..100")
    return value


def read_request():
    raw = sys.stdin.buffer.read(65537)
    if len(raw) > 65536:
        die("qualification supersession request too large")
    try:
        value = json.loads(raw.decode("utf-8"))
    except Exception:
        die("invalid qualification supersession request")
    if not isinstance(value, dict):
        die("qualification supersession request must be an object")
    return value


def request_json(opener, url, method, body, headers=None):
    raw = json.dumps(body, separators=(",", ":")).encode("utf-8")
    req = urllib.request.Request(url, data=raw, method=method)
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "application/json")
    for key, value in (headers or {}).items():
        req.add_header(key, value)
    try:
        with opener.open(req, timeout=8) as response:
            payload = response.read(1 << 20)
            if response.status == 204 or not payload:
                return response.status, {}
            return response.status, json.loads(payload.decode("utf-8"))
    except urllib.error.HTTPError as exc:
        payload = exc.read(1 << 20)
        try:
            detail = json.loads(payload.decode("utf-8") or "{}")
        except Exception:
            detail = {"error_code": "HTTP_ERROR"}
        die(f"hub HTTP {exc.code}: {json.dumps(redact(detail), sort_keys=True)}")


def command_id_from(response):
    envelope = response.get("envelope") if isinstance(response, dict) else None
    if not isinstance(envelope, dict):
        die("qualification supersession response missing command envelope")
    return bounded_text(envelope.get("command_id"), "command_id", 128)


def open_db(path):
    return sqlite3.connect("file:" + os.path.abspath(path) + "?mode=ro", uri=True)


def command_state(db_path, command_id):
    conn = open_db(db_path)
    try:
        row = conn.execute(
            "SELECT command_type, status, issued_at_us, completed_at_us FROM stage_device_commands WHERE command_id = ?",
            (command_id,),
        ).fetchone()
    finally:
        conn.close()
    if not row:
        return None
    return {
        "command_id": command_id,
        "command_type": row[0],
        "status": row[1],
        "issued_at_us": row[2],
        "completed_at_us": row[3],
    }


def wait_terminal(db_path, command_id, timeout_seconds):
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        state = command_state(db_path, command_id)
        if state and state.get("status") in TERMINAL:
            return state
        time.sleep(0.05)
    state = command_state(db_path, command_id) or {"command_id": command_id}
    state["wait_timeout"] = True
    return state


def observation(db_path, device_id, channel_key):
    conn = open_db(db_path)
    try:
        row = conn.execute(
            "SELECT observed_state_json FROM stage_device_runtime_state WHERE device_id = ?",
            (device_id,),
        ).fetchone()
    finally:
        conn.close()
    try:
        value = json.loads((row or ["{}"])[0] or "{}")
    except json.JSONDecodeError:
        value = {}
    if not isinstance(value, dict):
        value = {}
    active = value.get("active_fade")
    active_id = active.get("command_id") if isinstance(active, dict) else None
    levels = value.get("current_levels")
    return {
        "active_fade_command_id": active_id,
        "last_accepted_command_id": value.get("last_accepted_command_id"),
        "last_applied_command_id": value.get("last_applied_command_id"),
        "current_level": levels.get(channel_key) if isinstance(levels, dict) else None,
    }


def wait_observation(db_path, device_id, channel_key, predicate, timeout_seconds):
    deadline = time.monotonic() + timeout_seconds
    last = {}
    while time.monotonic() < deadline:
        last = observation(db_path, device_id, channel_key)
        if predicate(last):
            return True, last
        time.sleep(0.05)
    return False, last


def dispatch(opener, base, csrf, device_id, command_type, payload, deadline_seconds):
    body = {
        "command_type": command_type,
        "priority": "P2",
        "payload": payload,
        "deadline_at": (
            dt.datetime.now(dt.timezone.utc) + dt.timedelta(seconds=deadline_seconds)
        ).isoformat(),
    }
    _, response = request_json(
        opener,
        base + "/api/v1/stage-devices/" + urllib.parse.quote(device_id, safe="") + "/commands",
        "POST",
        body,
        {"X-StageCore-CSRF": csrf},
    )
    return command_id_from(response)


def main():
    parser = argparse.ArgumentParser(description="Bounded StageCore fade-supersession qualification helper")
    parser.add_argument("--hub-url", default="http://127.0.0.1:7840")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args = parser.parse_args()

    data = read_request()
    username = bounded_text(data.get("username"), "username", 128)
    password = bounded_text(data.get("password"), "password", 1024)
    device_id = bounded_text(data.get("device_id"), "device_id", 256)
    channel_key = bounded_text(data.get("channel_key"), "channel_key", 64)
    if not CHANNEL_KEY.fullmatch(channel_key):
        die("invalid channel_key")
    start_level = level(data.get("start_level"), "start_level")
    target_level = level(data.get("target_level"), "target_level")
    replacement_level = level(data.get("replacement_level"), "replacement_level")
    fade_ms = data.get("fade_ms")
    activation_timeout_ms = data.get("activation_timeout_ms", 5000)
    if isinstance(fade_ms, bool) or not isinstance(fade_ms, int) or fade_ms < 1000 or fade_ms > 120000:
        die("fade_ms must be within 1000..120000")
    if isinstance(activation_timeout_ms, bool) or not isinstance(activation_timeout_ms, int) or activation_timeout_ms < 250 or activation_timeout_ms > 10000:
        die("activation_timeout_ms must be within 250..10000")

    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    base = args.hub_url.rstrip("/")
    csrf = None
    result = {
        "operation": "LIGHTING_FADE_SUPERSESSION",
        "device_id": device_id,
        "channel_key": channel_key,
        "active_fade_seen": False,
    }

    try:
        _, login = request_json(
            opener, base + "/api/v1/auth/login", "POST",
            {"username": username, "password": password},
        )
        csrf = bounded_text(login.get("csrf_token"), "csrf_token", 512)

        pre_id = dispatch(
            opener, base, csrf, device_id, "LIGHTING_CHANNELS_SET",
            {"channels": {channel_key: start_level}}, 15,
        )
        pre = wait_terminal(args.db, pre_id, 12)
        result["precondition"] = pre
        if pre.get("status") != "COMPLETED":
            result["status"] = "FAIL"
            print(json.dumps(redact(result), sort_keys=True))
            return 1

        fade_id = dispatch(
            opener, base, csrf, device_id, "LIGHTING_CHANNELS_FADE",
            {"channels": {channel_key: target_level}, "fade_ms": fade_ms},
            (fade_ms / 1000.0) + 15,
        )
        result["superseded_command_id"] = fade_id

        seen, active_obs = wait_observation(
            args.db, device_id, channel_key,
            lambda obs: obs.get("active_fade_command_id") == fade_id,
            activation_timeout_ms / 1000.0,
        )
        result["active_fade_seen"] = seen
        result["active_observation"] = active_obs

        replacement_id = dispatch(
            opener, base, csrf, device_id, "LIGHTING_CHANNELS_SET",
            {"channels": {channel_key: replacement_level}}, 15,
        )
        result["replacement_command_id"] = replacement_id

        first = wait_terminal(args.db, fade_id, max(12.0, (fade_ms / 1000.0) + 10.0))
        second = wait_terminal(args.db, replacement_id, 12)
        final_seen, final_obs = wait_observation(
            args.db, device_id, channel_key,
            lambda obs: (
                obs.get("last_accepted_command_id") == replacement_id
                and obs.get("last_applied_command_id") == replacement_id
                and obs.get("active_fade_command_id") != fade_id
            ),
            5.0,
        )

        result["superseded"] = first
        result["replacement"] = second
        result["final_observation_seen"] = final_seen
        result["final_observation"] = final_obs
        success = (
            seen
            and first.get("status") == "CANCELLED"
            and second.get("status") == "COMPLETED"
            and final_seen
            and fade_id != replacement_id
        )
        result["status"] = "PASS" if success else "FAIL"
        print(json.dumps(redact(result), sort_keys=True))
        return 0 if success else 1
    finally:
        if csrf:
            try:
                request_json(
                    opener, base + "/api/v1/auth/logout", "POST", {},
                    {"X-StageCore-CSRF": csrf},
                )
            except SystemExit:
                pass


if __name__ == "__main__":
    raise SystemExit(main())
