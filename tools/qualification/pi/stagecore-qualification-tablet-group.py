#!/usr/bin/env python3
import argparse
import http.cookiejar
import json
import sqlite3
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

TERMINAL={"REJECTED","COMPLETED","FAILED","TIMED_OUT","CANCELLED"}


def emit(value):
    print(json.dumps(value,sort_keys=True))


def fail(message,code=1):
    emit({"status":"BLOCKED" if code==3 else "FAIL","detail":message})
    raise SystemExit(code)


def read_request():
    raw=sys.stdin.buffer.read(65537)
    if len(raw)>65536: fail("Tablet group request too large")
    try: value=json.loads(raw.decode())
    except Exception: fail("invalid Tablet group request")
    if not isinstance(value,dict): fail("Tablet group request must be an object")
    return value


def text(value,name,maximum=256):
    value=str(value or "").strip()
    if not value or len(value)>maximum or any(ord(ch)<32 for ch in value): fail(f"invalid {name}")
    return value


def request(opener,url,method,body,headers=None):
    raw=json.dumps(body,separators=(",",":")).encode()
    req=urllib.request.Request(url,data=raw,method=method)
    req.add_header("Content-Type","application/json"); req.add_header("Accept","application/json")
    for k,v in (headers or {}).items(): req.add_header(k,v)
    try:
        with opener.open(req,timeout=8) as res:
            payload=res.read(1<<20)
            return res.status,json.loads(payload.decode() or "{}") if payload else {}
    except urllib.error.HTTPError as exc:
        payload=exc.read(1<<20)
        try: detail=json.loads(payload.decode() or "{}")
        except Exception: detail={}
        fail(f"Hub HTTP {exc.code}: {detail.get('error','HTTP_ERROR')}")


def wait(db_path,command_id,timeout=15):
    deadline=time.monotonic()+timeout
    uri="file:"+db_path+"?mode=ro"
    while time.monotonic()<deadline:
        conn=sqlite3.connect(uri,uri=True)
        try:
            row=conn.execute("SELECT status,result_json FROM stage_device_commands WHERE command_id=?",(command_id,)).fetchone()
        finally:
            conn.close()
        if row and row[0] in TERMINAL:
            return {"command_id":command_id,"status":row[0]}
        time.sleep(.1)
    return {"command_id":command_id,"status":"WAIT_TIMEOUT"}


def main():
    parser=argparse.ArgumentParser(description="Bounded real Tablet group PLAY qualification helper")
    parser.add_argument("--hub-url",default="http://127.0.0.1:7840")
    parser.add_argument("--db",default="/var/lib/stagecore/data/db/stagecore.sqlite3")
    args=parser.parse_args()
    data=read_request()
    username=text(data.get("username"),"username",128); password=text(data.get("password"),"password",1024)
    project_id=text(data.get("project_id"),"project_id"); group_name=text(data.get("group_name"),"group_name",128)
    expected=data.get("expected_device_ids")
    if not isinstance(expected,list): fail("expected_device_ids must be a list")
    expected=[text(v,"device_id") for v in expected]
    if len(expected)<2 or len(expected)>10 or len(set(expected))!=len(expected): fail("group qualification requires 2..10 unique expected devices")
    media=data.get("media_number")
    if isinstance(media,bool) or not isinstance(media,int) or media<1 or media>9999: fail("media_number must be 1..9999")
    jar=http.cookiejar.CookieJar(); opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    base=args.hub_url.rstrip("/"); csrf=""
    try:
        status,login=request(opener,base+"/api/v1/auth/login","POST",{"username":username,"password":password})
        if status!=200: fail("qualification credential login failed",3)
        csrf=text(login.get("csrf_token"),"csrf_token",512)
        path="/api/v1/projects/"+urllib.parse.quote(project_id,safe="")+"/tablet-controller/commands"
        _,response=request(opener,base+path,"POST",{
            "group_name":group_name,"command_type":"TABLET_PLAY","priority":"P1",
            "payload":{"media_number":media},
        },{"X-StageCore-CSRF":csrf})
    finally:
        if csrf:
            try: request(opener,base+"/api/v1/auth/logout","POST",{},{"X-StageCore-CSRF":csrf})
            except Exception: pass
    results=response.get("results")
    if not isinstance(results,list): fail("Tablet group response missing results")
    actual=[str(item.get("device_id") or "").strip() for item in results]
    if set(actual)!=set(expected) or len(actual)!=len(expected):
        fail("group selector target set differs from qualified availability set")
    commands=[]
    for item in results:
        if item.get("error"): fail("Tablet group dispatch returned target error: "+str(item.get("error")))
        envelope=((item.get("command") or {}).get("envelope") or {})
        cid=text(envelope.get("command_id"),"command_id",128)
        commands.append((item["device_id"],cid))
    terminals=[{"device_id":device,**wait(args.db,cid)} for device,cid in commands]
    if any(item["status"]!="COMPLETED" for item in terminals):
        emit({"status":"FAIL","mode":"group-play","group_name":group_name,"device_ids":actual,"results":terminals})
        return 1
    emit({"status":"PASS","mode":"group-play","project_id":project_id,"group_name":group_name,"device_ids":actual,"correlation_id":response.get("correlation_id",""),"results":terminals})
    return 0


if __name__=="__main__":
    raise SystemExit(main())
