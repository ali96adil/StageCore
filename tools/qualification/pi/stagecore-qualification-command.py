#!/usr/bin/env python3
import argparse
import datetime as dt
import http.cookiejar
import json
import sqlite3
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

SAFE_COMMANDS = {"TABLET_PREPARE", "LIGHTING_STATE_READ", "LIGHTING_CONFIG_READ"}
PHYSICAL_COMMANDS = {
    "TABLET_PLAY", "TABLET_PAUSE", "TABLET_STOP",
    "TABLET_BLACKOUT", "TABLET_BLACKOUT_CLEAR",
    "TABLET_OVERLAY_PLAY", "TABLET_OVERLAY_CLEAR",
    "TABLET_LIVE_SHOW", "TABLET_LIVE_HIDE",
    "LIGHTING_CHANNELS_SET", "LIGHTING_CHANNELS_FADE", "LIGHTING_BLACKOUT",
}
TERMINAL = {"REJECTED", "COMPLETED", "FAILED", "TIMED_OUT", "CANCELLED"}
SENSITIVE_KEYS = {
    "password", "token", "csrf", "csrf_token", "authorization", "cookie",
    "secret", "session_token", "api_key", "apikey",
}


def redact(value):
    if isinstance(value, dict):
        out = {}
        for key, item in value.items():
            lowered = str(key).strip().lower()
            if lowered in SENSITIVE_KEYS or any(part in lowered for part in ("password", "secret", "token", "cookie", "authorization")):
                out[key] = "[REDACTED]"
            else:
                out[key] = redact(item)
        return out
    if isinstance(value, list):
        return [redact(item) for item in value]
    return value


def die(message, code=1):
    print(json.dumps({"status": "ERROR", "detail": message}, sort_keys=True))
    raise SystemExit(code)


def read_request():
    raw = sys.stdin.buffer.read(65537)
    if len(raw) > 65536:
        die("qualification command request too large")
    try:
        data = json.loads(raw.decode("utf-8"))
    except Exception:
        die("invalid qualification command request")
    if not isinstance(data, dict):
        die("qualification command request must be an object")
    return data


def bounded_text(value, name, maximum=256):
    value = str(value or "").strip()
    if not value or len(value) > maximum or any(ord(ch) < 32 for ch in value):
        die(f"invalid {name}")
    return value


def bounded_level(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        die(f"{name} must be numeric")
    value = float(value)
    if value < 0 or value > 100:
        die(f"{name} must be within 0..100")
    return value


def request_json(opener, url, method, body, headers=None):
    encoded = json.dumps(body, separators=(",", ":")).encode("utf-8")
    req = urllib.request.Request(url, data=encoded, method=method)
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


def extract_command(response, command_type):
    if command_type.startswith("TABLET_"):
        results = response.get("results")
        if not isinstance(results, list) or len(results) != 1:
            die("tablet qualification command returned unexpected target count")
        item = results[0]
        if item.get("error"):
            die("tablet qualification command rejected: " + str(item.get("error")))
        command = item.get("command")
    else:
        command = response
    if not isinstance(command, dict):
        die("qualification command response missing command")
    envelope = command.get("envelope") or {}
    return bounded_text(envelope.get("command_id"), "command_id", 128)


def wait_result(db_path, command_id, timeout_seconds):
    uri = "file:" + db_path + "?mode=ro"
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        conn = sqlite3.connect(uri, uri=True)
        try:
            row = conn.execute(
                """
                SELECT command_type, status, result_json, issued_at_us, completed_at_us
                FROM stage_device_commands
                WHERE command_id = ?
                """,
                (command_id,),
            ).fetchone()
        finally:
            conn.close()
        if row and row[1] in TERMINAL:
            result = None
            if row[2]:
                try:
                    result = redact(json.loads(row[2]))
                except json.JSONDecodeError:
                    result = {"decode_error": True}
            return {
                "command_id": command_id,
                "command_type": row[0],
                "status": row[1],
                "issued_at_us": row[3],
                "completed_at_us": row[4],
                "lifecycle_ms": (
                    (row[4] - row[3]) / 1000.0
                    if isinstance(row[3], int) and isinstance(row[4], int) and row[4] >= row[3]
                    else None
                ),
                "result": result,
            }
        time.sleep(0.1)
    return {"command_id": command_id, "status": "WAIT_TIMEOUT", "result": None}


def validate_payload(command_type, payload):
    if payload is None:
        payload = {}
    if not isinstance(payload, dict):
        die("payload must be an object")

    if command_type in {"TABLET_PREPARE", "TABLET_PLAY"}:
        allowed_keys = {"media_number", "tablet_cue_id"}
        if set(payload) - allowed_keys:
            die(f"{command_type} payload contains unsupported fields")
        has_media = "media_number" in payload
        has_cue = bool(str(payload.get("tablet_cue_id") or "").strip())
        if has_media == has_cue:
            die(f"{command_type} requires exactly one media_number or tablet_cue_id")
        if has_media:
            media = payload.get("media_number")
            if isinstance(media, bool) or not isinstance(media, int) or media < 1 or media > 9999:
                die(f"{command_type} media_number must be 1..9999")
            return {"media_number": media}
        return {"tablet_cue_id": bounded_text(payload.get("tablet_cue_id"), "tablet_cue_id", 256)}

    if command_type == "TABLET_OVERLAY_PLAY":
        if set(payload) != {"media_number"}:
            die("TABLET_OVERLAY_PLAY qualification requires media_number")
        media = payload.get("media_number")
        if isinstance(media, bool) or not isinstance(media, int) or media < 1 or media > 9999:
            die("TABLET_OVERLAY_PLAY media_number must be 1..9999")
        return {"media_number": media}

    if command_type == "TABLET_LIVE_SHOW":
        if set(payload) != {"media_key"}:
            die("TABLET_LIVE_SHOW qualification requires media_key")
        return {"media_key": bounded_text(payload.get("media_key"), "media_key", 256)}

    if command_type in {
        "TABLET_PAUSE", "TABLET_STOP",
        "TABLET_BLACKOUT", "TABLET_BLACKOUT_CLEAR",
        "TABLET_OVERLAY_CLEAR", "TABLET_LIVE_HIDE",
        "LIGHTING_STATE_READ", "LIGHTING_CONFIG_READ",
    }:
        if payload:
            die(f"{command_type} requires an empty payload")
        return {}

    if command_type == "LIGHTING_CHANNELS_SET":
        if set(payload) != {"channels"} or not isinstance(payload.get("channels"), dict):
            die("LIGHTING_CHANNELS_SET qualification requires channels")
        if not 1 <= len(payload["channels"]) <= 12:
            die("LIGHTING_CHANNELS_SET qualification requires 1..12 channels")
        channels = {}
        for key, level in payload["channels"].items():
            channels[bounded_text(key, "channel_key", 64)] = bounded_level(level, "level")
        return {"channels": channels}

    if command_type == "LIGHTING_CHANNELS_FADE":
        if set(payload) != {"channels", "fade_ms"} or not isinstance(payload.get("channels"), dict):
            die("LIGHTING_CHANNELS_FADE qualification requires channels and fade_ms")
        if not 1 <= len(payload["channels"]) <= 12:
            die("LIGHTING_CHANNELS_FADE qualification requires 1..12 channels")
        fade_ms = payload.get("fade_ms")
        if isinstance(fade_ms, bool) or not isinstance(fade_ms, int) or fade_ms < 100 or fade_ms > 120000:
            die("fade_ms must be within 100..120000")
        channels = {}
        for key, level in payload["channels"].items():
            channels[bounded_text(key, "channel_key", 64)] = bounded_level(level, "level")
        return {"channels": channels, "fade_ms": fade_ms}

    if command_type == "LIGHTING_BLACKOUT":
        if payload == {} or payload == {"fade_ms": 0}:
            return {}
        if set(payload) != {"fade_ms"}:
            die("LIGHTING_BLACKOUT qualification accepts only optional fade_ms")
        fade_ms = payload.get("fade_ms")
        if isinstance(fade_ms, bool) or not isinstance(fade_ms, int) or fade_ms < 100 or fade_ms > 120000:
            die("blackout fade_ms must be within 100..120000")
        return {"fade_ms": fade_ms}

    die("unsupported qualification command")


def main():
    parser = argparse.ArgumentParser(description="Bounded StageCore physical qualification command helper")
    parser.add_argument("--hub-url", default="http://127.0.0.1:7840")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    parser.add_argument("--timeout-seconds", type=float, default=12.0)
    parser.add_argument("--allow-physical", action="store_true")
    parser.add_argument("--expect-error-code", default="")
    args = parser.parse_args()

    data = read_request()
    username = bounded_text(data.get("username"), "username", 128)
    password = bounded_text(data.get("password"), "password", 1024)
    device_id = bounded_text(data.get("device_id"), "device_id", 256)
    command_type = bounded_text(data.get("command_type"), "command_type", 64)

    if command_type in PHYSICAL_COMMANDS:
        if not args.allow_physical:
            die("physical command requires the dedicated physical-command helper")
    elif command_type not in SAFE_COMMANDS:
        die("command is outside the qualification allowlist")

    project_id = str(data.get("project_id") or "").strip()
    if command_type.startswith("TABLET_"):
        project_id = bounded_text(project_id, "project_id", 256)
    payload = validate_payload(command_type, data.get("payload"))
    expected_error_code = str(args.expect_error_code or "").strip()
    if expected_error_code and command_type != "TABLET_PREPARE":
        die("expected-error qualification is limited to TABLET_PREPARE")
    duration_ms = 0
    if command_type in {"LIGHTING_CHANNELS_FADE", "LIGHTING_BLACKOUT"}:
        duration_ms = int(payload.get("fade_ms", 0))
    wait_timeout_seconds = max(args.timeout_seconds, (duration_ms / 1000.0) + 10.0)
    wait_timeout_seconds = min(wait_timeout_seconds, 140.0)
    command_deadline_seconds = max(15.0, (duration_ms / 1000.0) + 10.0)

    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    base = args.hub_url.rstrip("/")
    csrf = None
    try:
        _, login = request_json(
            opener, base + "/api/v1/auth/login", "POST",
            {"username": username, "password": password},
        )
        csrf = bounded_text(login.get("csrf_token"), "csrf_token", 512)

        if command_type.startswith("TABLET_"):
            path = "/api/v1/projects/" + urllib.parse.quote(project_id, safe="") + "/tablet-controller/commands"
            body = {
                "device_ids": [device_id],
                "command_type": command_type,
                "priority": "P0" if "BLACKOUT" in command_type else "P1",
                "payload": payload,
            }
        else:
            path = "/api/v1/stage-devices/" + urllib.parse.quote(device_id, safe="") + "/commands"
            body = {
                "command_type": command_type,
                "priority": "P0" if command_type == "LIGHTING_BLACKOUT" else "P2",
                "payload": payload,
                "deadline_at": (
                    dt.datetime.now(dt.timezone.utc) + dt.timedelta(seconds=command_deadline_seconds)
                ).isoformat(),
            }

        _, response = request_json(
            opener, base + path, "POST", body,
            {"X-StageCore-CSRF": csrf},
        )
        command_id = extract_command(response, command_type)
        result = wait_result(args.db, command_id, wait_timeout_seconds)
    finally:
        if csrf:
            try:
                request_json(
                    opener, base + "/api/v1/auth/logout", "POST", {},
                    {"X-StageCore-CSRF": csrf},
                )
            except SystemExit:
                pass

    result["device_id"] = device_id
    result["qualification_command"] = command_type
    print(json.dumps(redact(result), sort_keys=True))
    if expected_error_code:
        nested = result.get("result") if isinstance(result.get("result"), dict) else {}
        error = nested.get("error") if isinstance(nested.get("error"), dict) else {}
        if result.get("status") in {"FAILED", "REJECTED"} and error.get("error_code") == expected_error_code:
            return 0
        if result.get("status") == "WAIT_TIMEOUT":
            return 3
        return 1
    if result["status"] == "COMPLETED":
        return 0
    if result["status"] == "WAIT_TIMEOUT":
        return 3
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
