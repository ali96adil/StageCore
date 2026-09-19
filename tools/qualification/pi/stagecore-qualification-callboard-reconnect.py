#!/usr/bin/env python3
"""Q-CALL-06: read-only real expiry/reconnect evidence. Never changes a device."""
import argparse
import json
import os
import sqlite3
import sys
import time

def emit(value):
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))

def stop(reason, code=3):
    emit({"status": "BLOCKED" if code == 3 else "FAIL", "detail": reason})
    raise SystemExit(code)

def name(value):
    if not isinstance(value, str) or not value.strip() or len(value) > 256 or any(ord(c) < 32 for c in value):
        stop("invalid identity", 1)
    return value.strip()

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--db", default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    a = p.parse_args()
    raw = sys.stdin.buffer.read(16385)
    if len(raw) > 16384:
        stop("request too large", 1)
    try:
        req = json.loads(raw)
    except (ValueError, UnicodeDecodeError):
        stop("invalid JSON", 1)
    if not isinstance(req, dict) or req.get("mode") not in ("pre", "post"):
        stop("invalid request", 1)
    mode = req["mode"]
    if set(req) != ({"mode","project_id","device_id"} if mode=="pre" else {"mode","project_id","device_id","baseline"}):
        stop("unexpected request fields", 1)
    project, device = name(req["project_id"]), name(req["device_id"])
    if not os.path.isfile(a.db):
        stop("StageCore database unavailable")
    try:
        with sqlite3.connect("file:" + os.path.abspath(a.db) + "?mode=ro",uri=True) as db:
            now = int(time.time()*1000000)
            row = db.execute("""SELECT d.project_id,d.device_kind,d.enabled,d.protocol_version,
                                r.connection_state,r.readiness,r.last_seen_at_us
                                FROM stage_devices d LEFT JOIN stage_device_runtime_state r ON r.device_id=d.device_id
                                WHERE d.device_id=?""",(device,)).fetchone()
            if not row or row[:4] != (project,"STAGE_DISPLAY",1,"stagecore.device/1"):
                stop("real selected display unavailable or out of scope")
            state = db.execute("SELECT mode,command_id,expires_at_us FROM stage_display_state WHERE device_id=?",(device,)).fetchone()
            if not state or state[0]!="IDLE" or not state[1] or state[2] is not None:
                stop("completed DISPLAY_CLEAR to durable IDLE required")
            count = db.execute("SELECT COUNT(*) FROM stage_device_commands WHERE project_id=? AND device_id=?",(project,device)).fetchone()[0]
            if mode=="pre":
                if row[4:6] != ("ONLINE","READY") or row[6] is None or not 0<=now-row[6]<=15000000:
                    stop("display is not freshly ONLINE/READY")
                net = db.execute("""SELECT observation_id,observed_at_us,reachability,transport_state,error_code
                                    FROM network_observations WHERE target_kind='STAGE_DEVICE' AND target_id=?
                                    ORDER BY observed_at_us DESC,observation_id DESC LIMIT 1""",(device,)).fetchone()
                if not net or net[2:4]!=("REACHABLE","WEBSOCKET_CONNECTED") or net[4] or not 0<=now-net[1]<=15000000:
                    stop("fresh connected display observation unavailable")
                clear = db.execute("""SELECT completed_at_us FROM stage_device_commands
                                      WHERE command_id=? AND project_id=? AND device_id=?
                                      AND command_type='DISPLAY_CLEAR' AND status='COMPLETED'""",
                                   (state[1],project,device)).fetchone()
                alert = db.execute("""SELECT command_id,completed_at_us,payload_json FROM stage_device_commands
                                      WHERE project_id=? AND device_id=? AND command_type='DISPLAY_ALERT' AND status='COMPLETED'
                                      ORDER BY issued_at_us DESC LIMIT 1""",(project,device)).fetchone()
                if not clear or clear[0] is None or not alert or alert[1] is None or alert[1]>=clear[0]:
                    stop("completed alert followed by completed clear required")
                try:
                    duration=json.loads(alert[2])["duration_seconds"]
                except (ValueError,TypeError,KeyError):
                    stop("real alert duration unavailable",1)
                if isinstance(duration,bool) or not isinstance(duration,(int,float)) or not 0<duration<=60:
                    stop("invalid bounded alert duration",1)
                expired=alert[1]+int(duration*1000000)
                if now<=expired:
                    stop("real transient alert has not expired")
                emit({"status":"PASS","mode":"pre","project_id":project,"device_id":device,
                      "captured_at_us":now,"command_count":count,"display_command_id":state[1],
                      "alert_command_id":alert[0],"alert_expired_at_us":expired,
                      "network_id":net[0],"network_at_us":net[1],"physical_observation":False})
                return
            b=req["baseline"]
            if not isinstance(b,dict) or b.get("status")!="PASS" or b.get("mode")!="pre" or b.get("project_id")!=project or b.get("device_id")!=device:
                stop("pinned baseline identity mismatch",1)
            start=b.get("captured_at_us")
            prev_count=b.get("command_count")
            if any(isinstance(x,bool) or not isinstance(x,int) for x in (start,prev_count)) or not 0<start<=now or prev_count<1:
                stop("invalid pinned baseline",1)
            if not b.get("network_id") or not b.get("display_command_id") or not b.get("alert_command_id") or not isinstance(b.get("network_at_us"),int):
                stop("incomplete pinned baseline",1)
            if count!=prev_count or state[1]!=b["display_command_id"]:
                stop("new production command or changed durable display state after reconnect",1)
            if row[4:6]!=("ONLINE","READY") or row[6] is None or not 0<=now-row[6]<=15000000:
                stop("display did not recover to fresh ONLINE/READY")
            events=db.execute("""SELECT observation_id,observed_at_us,reachability,transport_state FROM network_observations
                                WHERE target_kind='STAGE_DEVICE' AND target_id=? AND observed_at_us>=?
                                ORDER BY observed_at_us,observation_id""",(device,b["network_at_us"])).fetchall()
            events=[e for e in events if e[0]!=b["network_id"] and e[1]<=now]
            down=next((e for e in events if e[2]=="UNREACHABLE" or e[3]=="WEBSOCKET_DISCONNECTED"),None)
            up=next((e for e in events if down and e[1]>down[1] and e[2:4]==("REACHABLE","WEBSOCKET_CONNECTED")),None)
            if not down or not up:
                stop("real ordered disconnect/reconnect observations unavailable")
            if events[-1][0]!=up[0] or not 0<=now-up[1]<=15000000:
                stop("latest network observation is not fresh recovery")
            emit({"status":"PASS","mode":"post","project_id":project,"device_id":device,
                  "alert_command_id":b["alert_command_id"],"command_count_unchanged":True,
                  "disconnect_observation_id":down[0],"reconnect_observation_id":up[0],
                  "physical_observation":False,
                  "limitations":"Operator must separately verify expired alert/chime did not visibly replay."})
    except sqlite3.Error:
        stop("read-only Stage Display evidence unavailable")

if __name__=="__main__":
    main()
