#!/usr/bin/env python3
import argparse
import json
import os
import sqlite3
import sys


def emit(value):
    print(json.dumps(value, sort_keys=True, separators=(",", ":")))


def fail(message, code=1):
    emit({"status": "BLOCKED" if code == 3 else "FAIL", "detail": message})
    raise SystemExit(code)


def read_request():
    raw=sys.stdin.buffer.read(65537)
    if len(raw)>65536:
        fail("draft evidence request too large")
    try:
        value=json.loads(raw.decode("utf-8"))
    except Exception:
        fail("invalid draft evidence request")
    if not isinstance(value,dict):
        fail("draft evidence request must be an object")
    return value


def text(value,name,maximum=256):
    value=str(value or "").strip()
    if not value or len(value)>maximum or any(ord(ch)<32 for ch in value):
        fail(f"invalid {name}")
    return value


def open_db(path):
    path=os.path.abspath(path)
    if not os.path.isfile(path):
        fail("StageCore database unavailable",3)
    try:
        return sqlite3.connect("file:"+path+"?mode=ro",uri=True)
    except sqlite3.Error as exc:
        fail("StageCore database open failed: "+str(exc),3)


def revision(conn, revision_id):
    return conn.execute(
        """SELECT revision_id,project_id,revision_number,status,parent_revision_id,created_at_us,created_by,change_note
           FROM project_revisions WHERE revision_id=?""",
        (revision_id,),
    ).fetchone()


def snapshot(conn, project_id):
    return conn.execute(
        """SELECT runtime_snapshot_id,revision_id,snapshot_version,created_at_us,created_by,content_hash,manifest_json,status
           FROM runtime_snapshots
           WHERE project_id=? AND status='PUBLISHED'
           ORDER BY snapshot_version DESC LIMIT 1""",
        (project_id,),
    ).fetchone()


def baseline(conn, project_id):
    project=conn.execute(
        "SELECT project_id,current_revision_id,lifecycle_state FROM projects WHERE project_id=?",
        (project_id,),
    ).fetchone()
    if project is None or not project[1]:
        fail("qualification Draft project unavailable",3)
    draft=revision(conn,project[1])
    if draft is None or draft[3]!="DRAFT" or not draft[4]:
        fail("qualification project current revision is not a discardable child Draft",3)
    parent=revision(conn,draft[4])
    if parent is None or parent[1]!=project_id or parent[3]!="VALIDATED":
        fail("Draft parent is not the validated recovery target")
    snap=snapshot(conn,project_id)
    if snap is None:
        fail("qualification project has no PUBLISHED Runtime Snapshot",3)
    if snap[1]!=parent[0]:
        fail("latest PUBLISHED Runtime Snapshot does not belong to Draft validated parent",3)
    audit=conn.execute(
        """SELECT COUNT(*),COALESCE(MAX(occurred_at_us),0)
           FROM security_audit_records
           WHERE event_type='project.draft.discard' AND resource_id=?""",
        (project_id,),
    ).fetchone()
    return {
        "project_id":project_id,
        "project_lifecycle_state":project[2],
        "draft":{
            "revision_id":draft[0],"revision_number":draft[2],"status":draft[3],
            "parent_revision_id":draft[4],"created_at_us":draft[5],"created_by":draft[6],
        },
        "parent":{
            "revision_id":parent[0],"revision_number":parent[2],"status":parent[3],
            "parent_revision_id":parent[4],"created_at_us":parent[5],"created_by":parent[6],
            "change_note":parent[7],
        },
        "published_snapshot":{
            "runtime_snapshot_id":snap[0],"revision_id":snap[1],"snapshot_version":snap[2],
            "created_at_us":snap[3],"created_by":snap[4],"content_hash":snap[5],
            "manifest_json":snap[6],"status":snap[7],
        },
        "audit_baseline":{"count":int(audit[0]),"max_occurred_at_us":int(audit[1] or 0)},
    }


def require_baseline(data):
    base=data.get("baseline")
    if not isinstance(base,dict):
        fail("baseline object is required")
    text(base.get("project_id"),"baseline.project_id")
    if not isinstance(base.get("draft"),dict) or not isinstance(base.get("parent"),dict) or not isinstance(base.get("published_snapshot"),dict):
        fail("baseline is incomplete")
    return base


def compare_unchanged(conn, base):
    project_id=base["project_id"]
    current=baseline(conn,project_id)
    for key in ("project_id","draft","parent","published_snapshot"):
        if current[key]!=base[key]:
            fail(f"Draft recovery baseline changed unexpectedly: {key}")
    success=conn.execute(
        """SELECT COUNT(*) FROM security_audit_records
           WHERE event_type='project.draft.discard' AND resource_id=? AND result='SUCCESS'
             AND occurred_at_us>?""",
        (project_id,int((base.get("audit_baseline") or {}).get("max_occurred_at_us",0))),
    ).fetchone()[0]
    if success:
        fail("unexpected successful Draft discard occurred during negative qualification probe")
    return current


def show_active(conn, base):
    project_id=base["project_id"]
    compare_unchanged(conn,base)
    row=conn.execute(
        """SELECT session_id,runtime_snapshot_id,started_at_us
           FROM sessions
           WHERE project_id=? AND session_type='SHOW' AND status='ACTIVE'
           ORDER BY started_at_us DESC LIMIT 1""",
        (project_id,),
    ).fetchone()
    if row is None:
        fail("active SHOW is required before the Q-DRAFT-03 negative discard probe",3)
    return {"session_id":row[0],"runtime_snapshot_id":row[1],"started_at_us":row[2]}


def post(conn, base):
    project_id=base["project_id"]
    draft=base["draft"]; parent=base["parent"]; snap0=base["published_snapshot"]
    project=conn.execute(
        "SELECT current_revision_id,lifecycle_state FROM projects WHERE project_id=?",
        (project_id,),
    ).fetchone()
    if project is None:
        fail("qualification project disappeared")
    if project[1] != base.get("project_lifecycle_state"):
        fail("project lifecycle state changed during Draft recovery")
    if project[0]==draft["revision_id"]:
        fail("Draft has not yet been discarded through the physical Operator UI",3)
    if project[0]!=parent["revision_id"]:
        fail("project current revision did not restore the validated parent")
    draft_now=revision(conn,draft["revision_id"])
    parent_now=revision(conn,parent["revision_id"])
    if draft_now is None or draft_now[3]!="SUPERSEDED":
        fail("abandoned Draft is not SUPERSEDED")
    if parent_now is None or parent_now[3]!="VALIDATED":
        fail("validated parent was mutated during discard")
    parent_exact={
        "revision_id":parent_now[0],"revision_number":parent_now[2],"status":parent_now[3],
        "parent_revision_id":parent_now[4],"created_at_us":parent_now[5],"created_by":parent_now[6],
        "change_note":parent_now[7],
    }
    if parent_exact != parent:
        fail("validated parent revision changed during discard")
    draft_count=conn.execute(
        "SELECT COUNT(*) FROM project_revisions WHERE project_id=? AND status='DRAFT'",
        (project_id,),
    ).fetchone()[0]
    if draft_count != 0:
        fail("clean recovery state still contains a Draft revision")
    latest=snapshot(conn,project_id)
    if latest is None or latest[0] != snap0["runtime_snapshot_id"]:
        fail("latest PUBLISHED Runtime Snapshot identity changed during discard")
    snap=conn.execute(
        """SELECT runtime_snapshot_id,revision_id,snapshot_version,created_at_us,created_by,content_hash,manifest_json,status
           FROM runtime_snapshots WHERE runtime_snapshot_id=?""",
        (snap0["runtime_snapshot_id"],),
    ).fetchone()
    if snap is None:
        fail("published Runtime Snapshot disappeared")
    snap_now={
        "runtime_snapshot_id":snap[0],"revision_id":snap[1],"snapshot_version":snap[2],
        "created_at_us":snap[3],"created_by":snap[4],"content_hash":snap[5],
        "manifest_json":snap[6],"status":snap[7],
    }
    if snap_now!=snap0:
        fail("validated/published Runtime Snapshot identity or immutable content changed")
    rows=conn.execute(
        """SELECT audit_id,occurred_at_us,actor_username,result,reason,metadata_json
           FROM security_audit_records
           WHERE event_type='project.draft.discard' AND resource_id=? AND result='SUCCESS'
             AND occurred_at_us>?
           ORDER BY occurred_at_us DESC""",
        (project_id,int((base.get("audit_baseline") or {}).get("max_occurred_at_us",0))),
    ).fetchall()
    matched=None
    for row in rows:
        try:
            metadata=json.loads(row[5] or "{}")
        except json.JSONDecodeError:
            continue
        if metadata.get("discarded") is True and metadata.get("restored_revision_id")==parent["revision_id"]:
            matched={"audit_id":row[0],"occurred_at_us":row[1],"actor_username":row[2],"result":row[3],"reason":row[4],"metadata":metadata}
            break
    if matched is None:
        fail("successful project.draft.discard audit record with exact restored revision was not found")
    return {
        "status":"PASS","mode":"post","project_id":project_id,
        "draft_revision_id":draft["revision_id"],"draft_status":draft_now[3],
        "restored_revision_id":project[0],"remaining_draft_count":draft_count,
        "published_snapshot":snap_now,
        "audit":matched,
    }


def main():
    parser=argparse.ArgumentParser(description="Read-only Draft recovery qualification evidence")
    parser.add_argument("--db",default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args=parser.parse_args()
    data=read_request()
    mode=text(data.get("mode"),"mode",32)
    project_id=text(data.get("project_id"),"project_id")
    conn=open_db(args.db)
    try:
        if mode=="baseline":
            out=baseline(conn,project_id)
            out.update({"status":"PASS","mode":"baseline"})
        elif mode=="unchanged":
            base=require_baseline(data)
            if base["project_id"]!=project_id: fail("baseline project mismatch")
            current=compare_unchanged(conn,base)
            out={"status":"PASS","mode":"unchanged","project_id":project_id,"draft_revision_id":current["draft"]["revision_id"],"published_snapshot_id":current["published_snapshot"]["runtime_snapshot_id"]}
        elif mode=="show-active":
            base=require_baseline(data)
            if base["project_id"]!=project_id: fail("baseline project mismatch")
            session=show_active(conn,base)
            out={"status":"PASS","mode":"show-active","project_id":project_id,"session":session}
        elif mode=="post":
            base=require_baseline(data)
            if base["project_id"]!=project_id: fail("baseline project mismatch")
            out=post(conn,base)
        else:
            fail("unsupported Draft evidence mode")
        emit(out)
    except sqlite3.Error as exc:
        fail("Draft evidence query failed: "+str(exc),3)
    finally:
        conn.close()


if __name__=="__main__":
    main()
