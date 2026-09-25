#!/usr/bin/env python3
"""Read-only real network-fault evidence; never toggles connectivity."""
import argparse
import json
import os
import sqlite3
import sys
import time

def emit(value):
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))

def stop(message, code=1):
    emit({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message})
    raise SystemExit(code)

def bounded(value, name):
    if not isinstance(value, str) or not value.strip() or len(value) > 256 or any(ord(c) < 32 for c in value):
        stop("invalid " + name)
    return value.strip()

def target(conn, project, device):
    row = conn.execute(
        """SELECT d.project_id,d.device_kind,d.enabled,r.connection_state,r.readiness,r.last_seen_at_us
           FROM stage_devices d LEFT JOIN stage_device_runtime_state r ON r.device_id=d.device_id
           WHERE d.device_id=?""", (device,),
    ).fetchone()
    if row is None or row[0] != project or row[2] != 1:
        stop("target device absent, disabled or belongs to another project", 3)
    return row

def observations(conn, device, since):
    return conn.execute(
        """SELECT observation_id,observed_at_us,reachability,transport_state,error_code
           FROM network_observations
           WHERE target_kind='STAGE_DEVICE' AND target_id=? AND observed_at_us>=?
           ORDER BY observed_at_us,observation_id""", (device, since),
    ).fetchall()

def is_up(row):
    return row[2] == "REACHABLE" and row[3] == "WEBSOCKET_CONNECTED" and not row[4]

def is_down(row):
    return row[2] == "UNREACHABLE" or row[3] in ("WEBSOCKET_DISCONNECTED", "REVOKED", "IDENTITY_CONFLICT")

def main():
    parser = argparse.ArgumentParser(description="Physical network fault observation verifier")
    parser.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args = parser.parse_args()
    raw = sys.stdin.buffer.read(16385)
    if len(raw) > 16384:
        stop("request too large")
    try:
        req = json.loads(raw.decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        stop("invalid JSON")
    if not isinstance(req, dict):
        stop("request must be an object")
    mode = req.get("mode")
    if mode not in ("pre", "post") or set(req) != ({"mode","project_id","device_id"} if mode == "pre" else {"mode","project_id","device_id","baseline"}):
        stop("unsupported request fields")
    project = bounded(req["project_id"], "project_id")
    device = bounded(req["device_id"], "device_id")
    db = os.path.abspath(args.db)
    if not os.path.isfile(db):
        stop("StageCore database unavailable", 3)
    try:
        conn = sqlite3.connect("file:" + db + "?mode=ro", uri=True)
        try:
            row = target(conn, project, device)
            now = int(time.time() * 1000000)
            if mode == "pre":
                if row[3:5] != ("ONLINE","READY") or row[5] is None or not 0 <= now - row[5] <= 15_000_000:
                    stop("target is not freshly ONLINE/READY", 3)
                seen = observations(conn, device, 0)
                if not seen or not is_up(seen[-1]) or not 0 <= now - seen[-1][1] <= 15_000_000:
                    stop("fresh connected network baseline unavailable", 3)
                emit({"status":"PASS","mode":"pre","project_id":project,"device_id":device,
                      "captured_at_us":now,"last_observation_id":seen[-1][0],
                      "last_observed_at_us":seen[-1][1],"connection_state":row[3],"readiness":row[4]})
                return
            baseline = req["baseline"]
            if not isinstance(baseline,dict) or baseline.get("status")!="PASS" or baseline.get("mode")!="pre" or baseline.get("project_id")!=project or baseline.get("device_id")!=device:
                stop("pinned network baseline identity mismatch")
            since = baseline.get("captured_at_us")
            initial = baseline.get("last_observed_at_us")
            if (isinstance(since,bool) or not isinstance(since,int) or
                isinstance(initial,bool) or not isinstance(initial,int) or initial>since or since>now):
                stop("invalid pinned baseline timestamps")
            if row[3:5] != ("ONLINE","READY") or row[5] is None or not 0<=now-row[5]<=15_000_000:
                stop("target did not recover to fresh ONLINE/READY",3)
            events = observations(conn,device,since)
            # Only records created after the pinned baseline count. Same-microsecond
            # records are allowed only when different from the baseline observation ID.
            events = [r for r in events if r[0] != baseline.get("last_observation_id") and r[1] <= now]
            offline = next((r for r in events if is_down(r)),None)
            if offline is None:
                stop("no real disconnect observation after the pinned baseline",3)
            online = next((r for r in events if r[1]>offline[1] and is_up(r)),None)
            if online is None:
                stop("no real reconnect observation after disconnect",3)
            if not events or events[-1][0] != online[0] or not 0 <= now - online[1] <= 15_000_000:
                stop("latest real network observation is not a fresh recovery",3)
            reason = ("TARGET_UNREACHABLE" if offline[2]=="UNREACHABLE" else
                      ("IDENTITY_CONFLICT" if offline[3]=="IDENTITY_CONFLICT" else
                       ("REVOKED" if offline[3]=="REVOKED" else (offline[4] or "TRANSPORT_DISCONNECTED"))))
            emit({"status":"PASS","mode":"post","project_id":project,"device_id":device,
                  "baseline_observation_id":baseline["last_observation_id"],
                  "disconnect":{"observation_id":offline[0],"observed_at_us":offline[1],"reason_code":reason},
                  "reconnect":{"observation_id":online[0],"observed_at_us":online[1]},
                  "connection_state":row[3],"readiness":row[4],
                  "operator_cockpit_verified":False,
                  "limitations":"Database event ordering alone cannot prove Cockpit rendering or actionable warning."})
        finally:
            conn.close()
    except sqlite3.Error as exc:
        stop("read-only network evidence query unavailable: " + str(exc),3)

if __name__=="__main__":
    main()
