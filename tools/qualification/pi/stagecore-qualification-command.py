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

ALLOWED = {"TABLET_PREPARE", "LIGHTING_STATE_READ", "LIGHTING_CONFIG_READ"}
TERMINAL = {"REJECTED", "COMPLETED", "FAILED", "TIMED_OUT", "CANCELLED"}


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
            return response.status, json.loads(payload.decode("utf-8") or "{}")
    except urllib.error.HTTPError as exc:
        payload = exc.read(1 << 20)
        try:
            detail = json.loads(payload.decode("utf-8") or "{}")
        except Exception:
            detail = {"error_code": "HTTP_ERROR"}
        die(f"hub HTTP {exc.code}: {json.dumps(detail, sort_keys=True)}")


def extract_command(response, command_type):
    if command_type == "TABLET_PREPARE":
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
    command_id = bounded_text(envelope.get("command_id"), "command_id", 128)
    return command_id


def wait_result(db_path, command_id, timeout_seconds):
    uri = "file:" + db_path + "?mode=ro"
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        conn = sqlite3.connect(uri, uri=True)
        try:
            row = conn.execute(
                """
                SELECT command_type, status, result_json, completed_at_us
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
                    result = json.loads(row[2])
                except json.JSONDecodeError:
                    result = {"decode_error": True}
            return {
                "command_id": command_id,
                "command_type": row[0],
                "status": row[1],
                "completed_at_us": row[3],
                "result": result,
            }
        time.sleep(0.1)
    return {
        "command_id": command_id,
        "status": "WAIT_TIMEOUT",
        "result": None,
    }


def main():
    parser = argparse.ArgumentParser(description="Bounded StageCore physical qualification command helper")
    parser.add_argument("--hub-url", default="http://127.0.0.1:7840")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    parser.add_argument("--timeout-seconds", type=float, default=12.0)
    args = parser.parse_args()

    data = read_request()
    username = bounded_text(data.get("username"), "username", 128)
    password = bounded_text(data.get("password"), "password", 1024)
    device_id = bounded_text(data.get("device_id"), "device_id", 256)
    command_type = bounded_text(data.get("command_type"), "command_type", 64)
    if command_type not in ALLOWED:
        die("command is outside the qualification allowlist")
    project_id = str(data.get("project_id") or "").strip()
    payload = data.get("payload")
    if payload is None:
        payload = {}
    if not isinstance(payload, dict):
        die("payload must be an object")

    if command_type == "TABLET_PREPARE":
        project_id = bounded_text(project_id, "project_id", 256)
        allowed_keys = {"media_number", "tablet_cue_id"}
        if set(payload) - allowed_keys:
            die("TABLET_PREPARE payload contains unsupported fields")
        has_media = "media_number" in payload
        has_cue = bool(str(payload.get("tablet_cue_id") or "").strip())
        if has_media == has_cue:
            die("TABLET_PREPARE requires exactly one media_number or tablet_cue_id")
        if has_media:
            media = payload.get("media_number")
            if isinstance(media, bool) or not isinstance(media, int) or media < 1 or media > 9999:
                die("TABLET_PREPARE media_number must be 1..9999")
        else:
            payload = {"tablet_cue_id": bounded_text(payload.get("tablet_cue_id"), "tablet_cue_id", 256)}
    elif payload:
        die("lighting read qualification commands require an empty payload")

    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    base = args.hub_url.rstrip("/")
    csrf = None
    try:
        _, login = request_json(
            opener,
            base + "/api/v1/auth/login",
            "POST",
            {"username": username, "password": password},
        )
        csrf = bounded_text(login.get("csrf_token"), "csrf_token", 512)

        if command_type == "TABLET_PREPARE":
            path = "/api/v1/projects/" + urllib.parse.quote(project_id, safe="") + "/tablet-controller/commands"
            body = {
                "device_ids": [device_id],
                "command_type": command_type,
                "priority": "P1",
                "payload": payload,
            }
        else:
            path = "/api/v1/stage-devices/" + urllib.parse.quote(device_id, safe="") + "/commands"
            body = {
                "command_type": command_type,
                "priority": "P2",
                "payload": {},
                "deadline_at": (
                    dt.datetime.now(dt.timezone.utc) + dt.timedelta(seconds=10)
                ).isoformat(),
            }

        _, response = request_json(
            opener,
            base + path,
            "POST",
            body,
            {"X-StageCore-CSRF": csrf},
        )
        command_id = extract_command(response, command_type)
        result = wait_result(args.db, command_id, args.timeout_seconds)
    finally:
        if csrf:
            try:
                request_json(
                    opener,
                    base + "/api/v1/auth/logout",
                    "POST",
                    {},
                    {"X-StageCore-CSRF": csrf},
                )
            except SystemExit:
                pass

    result["device_id"] = device_id
    result["qualification_command"] = command_type
    print(json.dumps(result, sort_keys=True))
    if result["status"] == "COMPLETED":
        return 0
    if result["status"] == "WAIT_TIMEOUT":
        return 3
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
