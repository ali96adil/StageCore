#!/usr/bin/env python3
import argparse
import datetime as dt
import http.cookiejar
import json
import math
import os
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

TERMINAL = {"REJECTED", "COMPLETED", "FAILED", "TIMED_OUT", "CANCELLED"}
INTERRUPTED_TERMINAL = {"FAILED", "TIMED_OUT", "CANCELLED"}
SENSITIVE_PARTS = ("password", "secret", "token", "cookie", "authorization", "csrf")


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


def emit_error(message, code):
    print(json.dumps({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message}, sort_keys=True))
    raise SystemExit(code)


def bounded_text(value, name, maximum=256):
    value = str(value or "").strip()
    if not value or len(value) > maximum or any(ord(ch) < 32 for ch in value):
        emit_error(f"invalid {name}", 1)
    return value


def bounded_level(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        emit_error(f"{name} must be numeric", 1)
    value = float(value)
    if not math.isfinite(value) or value < 0 or value > 100:
        emit_error(f"{name} must be within 0..100", 1)
    return value


def read_request():
    raw = sys.stdin.buffer.read(65537)
    if len(raw) > 65536:
        emit_error("Hub-restart qualification request too large", 1)
    try:
        value = json.loads(raw.decode("utf-8"))
    except Exception:
        emit_error("invalid Hub-restart qualification request", 1)
    if not isinstance(value, dict):
        emit_error("Hub-restart qualification request must be an object", 1)
    return value


def request_json(opener, url, method, body, headers=None):
    raw = json.dumps(body, separators=(",", ":")).encode("utf-8")
    request = urllib.request.Request(url, data=raw, method=method)
    request.add_header("Content-Type", "application/json")
    request.add_header("Accept", "application/json")
    for key, value in (headers or {}).items():
        request.add_header(key, value)
    try:
        with opener.open(request, timeout=8) as response:
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
        emit_error(f"Hub HTTP {exc.code}: {json.dumps(redact(detail), sort_keys=True)}", 1)
    except urllib.error.URLError as exc:
        emit_error(f"Hub request unavailable: {exc.reason}", 3)


def command_id_from(response):
    envelope = response.get("envelope") if isinstance(response, dict) else None
    if not isinstance(envelope, dict):
        emit_error("Hub-restart command response missing envelope", 1)
    return bounded_text(envelope.get("command_id"), "command_id", 128)


def db_connect(path):
    return sqlite3.connect("file:" + os.path.abspath(path) + "?mode=ro", uri=True)


def safe_json(raw):
    try:
        value = json.loads(raw or "{}")
        return value if isinstance(value, dict) else {}
    except json.JSONDecodeError:
        return {}


def command_row(db_path, command_id):
    conn = db_connect(db_path)
    try:
        row = conn.execute(
            """
            SELECT command_type, status, result_json, issued_at_us, completed_at_us
            FROM stage_device_commands WHERE command_id = ?
            """,
            (command_id,),
        ).fetchone()
        count = conn.execute(
            "SELECT COUNT(*) FROM stage_device_commands WHERE command_id = ?",
            (command_id,),
        ).fetchone()[0]
    finally:
        conn.close()
    if row is None:
        return {"command_id": command_id, "row_count": count}
    result = safe_json(row[2]) if row[2] else None
    return {
        "command_id": command_id,
        "row_count": count,
        "command_type": row[0],
        "status": row[1],
        "result": redact(result),
        "issued_at_us": row[3],
        "completed_at_us": row[4],
    }


def runtime_snapshot(db_path, device_id):
    conn = db_connect(db_path)
    try:
        row = conn.execute(
            """
            SELECT COALESCE(d.project_id,''), d.client_version,
                   r.connection_state, r.readiness, r.last_seen_at_us,
                   r.observed_state_json
            FROM stage_devices d
            LEFT JOIN stage_device_runtime_state r ON r.device_id=d.device_id
            WHERE d.device_id=?
            """,
            (device_id,),
        ).fetchone()
    finally:
        conn.close()
    if row is None:
        return None
    return {
        "project_id": row[0],
        "client_version": row[1],
        "connection_state": row[2],
        "readiness": row[3],
        "last_seen_at_us": row[4],
        "observed": safe_json(row[5]),
    }


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


def wait_terminal(db_path, command_id, timeout_seconds):
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        row = command_row(db_path, command_id)
        if row.get("status") in TERMINAL:
            return row
        time.sleep(0.1)
    return command_row(db_path, command_id)


def wait_active_fade(db_path, device_id, command_id, timeout_seconds):
    deadline = time.monotonic() + timeout_seconds
    last = None
    while time.monotonic() < deadline:
        last = runtime_snapshot(db_path, device_id)
        observed = (last or {}).get("observed") or {}
        active = observed.get("active_fade")
        if isinstance(active, dict) and active.get("command_id") == command_id:
            return last
        time.sleep(0.1)
    return last


def hub_ready(base):
    try:
        with urllib.request.urlopen(base + "/health/ready", timeout=2) as response:
            return 200 <= response.status < 300
    except Exception:
        return False


def wait_hub_ready(base, timeout_seconds):
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if hub_ready(base):
            return True
        time.sleep(0.25)
    return False


def safe_levels(observed):
    levels = observed.get("current_levels")
    if not isinstance(levels, dict) or not levels:
        return False, {}
    normalized = {}
    for key, value in levels.items():
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(float(value)):
            return False, {}
        normalized[str(key)] = float(value)
    return all(abs(value) <= 0.01 for value in normalized.values()), normalized


def safe_reconnect_snapshot(snapshot, pre, command_id, restart_started_us):
    if not isinstance(snapshot, dict):
        return False, "runtime snapshot missing", {}
    observed = snapshot.get("observed") or {}
    if snapshot.get("project_id") != pre.get("project_id"):
        return False, "project changed", {}
    if snapshot.get("client_version") != pre.get("client_version"):
        return False, "firmware client version changed", {}
    if snapshot.get("connection_state") != "ONLINE":
        return False, "device is not ONLINE", {}
    if snapshot.get("readiness") not in {"READY", "WARNING"}:
        return False, "device readiness is not READY/WARNING", {}
    last_seen = snapshot.get("last_seen_at_us")
    if not isinstance(last_seen, int) or last_seen < restart_started_us:
        return False, "observation is not newer than Hub restart", {}
    if observed.get("dmx_healthy") is not True:
        return False, "DMX is not healthy", {}
    if observed.get("authority") != "STAGECORE":
        return False, "authority did not return to STAGECORE", {}
    pre_observed = pre.get("observed") or {}
    pre_hash = str(pre_observed.get("configuration_hash") or "")
    post_hash = str(observed.get("configuration_hash") or "")
    if not pre_hash or pre_hash != post_hash:
        return False, "configuration hash changed", {}
    pre_uptime = pre_observed.get("uptime_seconds")
    post_uptime = observed.get("uptime_seconds")
    if not isinstance(pre_uptime, (int, float)) or isinstance(pre_uptime, bool):
        return False, "pre uptime missing", {}
    if not isinstance(post_uptime, (int, float)) or isinstance(post_uptime, bool):
        return False, "post uptime missing", {}
    if float(post_uptime) + 1 < float(pre_uptime):
        return False, "ESP32 rebooted during Hub restart", {}
    if observed.get("reset_reason") != pre_observed.get("reset_reason"):
        return False, "ESP32 reset reason changed", {}
    safe, levels = safe_levels(observed)
    if not safe:
        return False, "post-reconnect output is not blackout", levels
    if observed.get("active_fade") not in (None, {}, False):
        return False, "old/new fade is active after reconnect", levels
    if observed.get("last_accepted_command_id") == command_id:
        return False, "interrupted command was accepted again after reconnect", levels
    if observed.get("last_applied_command_id") == command_id:
        return False, "interrupted command was applied again after reconnect", levels
    return True, "", levels


def wait_safe_reconnect(db_path, device_id, pre, command_id, restart_started_us, timeout_seconds):
    deadline = time.monotonic() + timeout_seconds
    last = None
    detail = "device did not reconnect"
    levels = {}
    while time.monotonic() < deadline:
        last = runtime_snapshot(db_path, device_id)
        ok, detail, levels = safe_reconnect_snapshot(
            last, pre, command_id, restart_started_us
        )
        if ok:
            return last, levels
        time.sleep(0.25)
    emit_error("post-restart Stage Device evidence unavailable: " + detail, 3)


def main():
    parser = argparse.ArgumentParser(description="Bounded StageCore Hub-restart no-replay qualification helper")
    parser.add_argument("--hub-url", default="http://127.0.0.1:7840")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    parser.add_argument("--service", default="stagecore-hub.service")
    args = parser.parse_args()

    if args.service != "stagecore-hub.service":
        emit_error("unsupported service", 1)

    data = read_request()
    username = bounded_text(data.get("username"), "username", 128)
    password = bounded_text(data.get("password"), "password", 1024)
    device_id = bounded_text(data.get("device_id"), "device_id", 256)
    project_id = bounded_text(data.get("project_id"), "project_id", 256)
    channel_key = bounded_text(data.get("channel_key"), "channel_key", 64)
    start_level = bounded_level(data.get("start_level"), "start_level")
    target_level = bounded_level(data.get("target_level"), "target_level")
    fade_ms = data.get("fade_ms")
    activation_timeout_ms = data.get("activation_timeout_ms", 15000)

    if start_level == target_level:
        emit_error("Hub-restart gate requires distinct start and target levels", 1)
    if isinstance(fade_ms, bool) or not isinstance(fade_ms, int) or fade_ms < 20000 or fade_ms > 120000:
        emit_error("fade_ms must be within 20000..120000", 1)
    if isinstance(activation_timeout_ms, bool) or not isinstance(activation_timeout_ms, int) or activation_timeout_ms < 1000 or activation_timeout_ms > 20000:
        emit_error("activation_timeout_ms must be within 1000..20000", 1)

    base = args.hub_url.rstrip("/")
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    csrf = None

    _, login = request_json(
        opener, base + "/api/v1/auth/login", "POST",
        {"username": username, "password": password},
    )
    csrf = bounded_text(login.get("csrf_token"), "csrf_token", 512)

    pre_id = dispatch(
        opener, base, csrf, device_id, "LIGHTING_CHANNELS_SET",
        {"channels": {channel_key: start_level}}, 15,
    )
    precondition = wait_terminal(args.db, pre_id, 12)
    if precondition.get("status") != "COMPLETED":
        emit_error("Hub-restart precondition SET did not complete", 1)

    fade_id = dispatch(
        opener, base, csrf, device_id, "LIGHTING_CHANNELS_FADE",
        {"channels": {channel_key: target_level}, "fade_ms": fade_ms},
        (fade_ms / 1000.0) + 15,
    )
    pre = wait_active_fade(
        args.db, device_id, fade_id, activation_timeout_ms / 1000.0
    )
    observed = (pre or {}).get("observed") or {}
    active = observed.get("active_fade")
    if not isinstance(active, dict) or active.get("command_id") != fade_id:
        emit_error("target fade was not observed active before Hub restart", 3)
    if (pre or {}).get("project_id") != project_id:
        emit_error("device project changed before Hub restart", 1)

    row_before = command_row(args.db, fade_id)
    if row_before.get("row_count") != 1 or row_before.get("status") != "ACCEPTED":
        emit_error("active fade command was not a single persisted ACCEPTED row", 1)

    restart_started_us = int(time.time() * 1_000_000)
    try:
        subprocess.run(
            ["systemctl", "restart", args.service],
            check=True, timeout=25,
            stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True,
        )
    except subprocess.TimeoutExpired:
        emit_error("Hub service restart timed out", 3)
    except subprocess.CalledProcessError as exc:
        emit_error("Hub service restart failed: " + (exc.stderr or "").strip()[:300], 1)

    if not wait_hub_ready(base, 20):
        emit_error("Hub did not return READY after restart", 3)

    post, levels = wait_safe_reconnect(
        args.db, device_id, pre, fade_id, restart_started_us, 35
    )

    terminal = wait_terminal(args.db, fade_id, 8)
    if terminal.get("row_count") != 1:
        emit_error("interrupted command persistence count changed", 1)
    if terminal.get("status") not in INTERRUPTED_TERMINAL:
        emit_error(
            "interrupted command remained ambiguous or completed after restart: "
            + str(terminal.get("status")),
            1,
        )

    # Hold the safe state briefly after reconnect. A delayed stale replay must
    # not restart the old fade after the first reconnect observation.
    time.sleep(2.0)
    stable = runtime_snapshot(args.db, device_id)
    stable_ok, stable_detail, stable_levels = safe_reconnect_snapshot(
        stable, pre, fade_id, restart_started_us
    )
    if not stable_ok:
        emit_error("post-reconnect state did not remain safe: " + stable_detail, 1)

    result_error = (terminal.get("result") or {}).get("error")
    error_code = result_error.get("error_code") if isinstance(result_error, dict) else None
    result = {
        "status": "PASS",
        "operation": "HUB_RESTART_NO_REPLAY",
        "device_id": device_id,
        "project_id": project_id,
        "precondition_command_id": pre_id,
        "interrupted_command_id": fade_id,
        "active_before_restart": True,
        "command_status_before_restart": row_before.get("status"),
        "command_status_after_restart": terminal.get("status"),
        "command_terminal_error_code": error_code,
        "command_row_count": terminal.get("row_count"),
        "hub_ready_after_restart": True,
        "reconnected": True,
        "same_firmware": post.get("client_version") == pre.get("client_version"),
        "same_configuration_hash": (
            str((post.get("observed") or {}).get("configuration_hash") or "")
            == str((pre.get("observed") or {}).get("configuration_hash") or "")
        ),
        "no_esp_reboot": True,
        "no_stale_replay": True,
        "safe_blackout_after_reconnect": True,
        "post_levels": levels,
        "stable_levels_after_hold": stable_levels,
        "post_readiness": post.get("readiness"),
        "post_authority": (post.get("observed") or {}).get("authority"),
        "post_dmx_healthy": (post.get("observed") or {}).get("dmx_healthy"),
    }
    print(json.dumps(redact(result), sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
