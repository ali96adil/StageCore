#!/usr/bin/env python3
import argparse, http.cookiejar, json, os, sqlite3, sys, urllib.error, urllib.request

def emit(v): print(json.dumps(v, sort_keys=True, separators=(",", ":")))
def stop(msg, code=1):
    emit({"status":"BLOCKED" if code==3 else "FAIL","detail":msg})
    raise SystemExit(code)
def text(v,name,maximum=256):
    v=str(v or "").strip()
    if not v or len(v)>maximum or any(ord(c)<32 for c in v): stop("invalid "+name)
    return v
def request(opener,url,method,body=None,headers=None):
    raw=None if body is None else json.dumps(body,separators=(",",":")).encode()
    req=urllib.request.Request(url,data=raw,method=method); req.add_header("Accept","application/json")
    if raw is not None: req.add_header("Content-Type","application/json")
    for k,v in (headers or {}).items(): req.add_header(k,v)
    try:
        with opener.open(req,timeout=8) as response:
            data=response.read(1<<20); return response.status,json.loads(data.decode() or "{}")
    except urllib.error.HTTPError as exc:
        data=exc.read(1<<20)
        try: detail=json.loads(data.decode() or "{}")
        except Exception: detail={}
        stop("Hub HTTP %s %s"%(exc.code,detail.get("error","")),3)
    except OSError as exc: stop("Hub unavailable: "+str(exc),3)
def latest(conn):
    out={}
    for row in conn.execute("""SELECT target_kind,target_id,observed_at_us,reachability,transport_state,
                                      latency_ms,jitter_ms,error_code
                               FROM network_observations
                               ORDER BY observed_at_us DESC,observation_id DESC"""):
        key=(row[0],row[1])
        if key in out: continue
        out[key]={"reachability":row[3],"transport_state":row[4],"latency_ms":row[5],"jitter_ms":row[6],"error_code":row[7] or ""}
    return out
def main():
    p=argparse.ArgumentParser(); p.add_argument("--db",default="/var/lib/stagecore/data/db/stagecore.sqlite3"); p.add_argument("--hub-url",default="http://127.0.0.1:7840"); a=p.parse_args()
    try: data=json.loads(sys.stdin.buffer.read(32769))
    except Exception: stop("invalid request")
    username=text(data.get("username"),"username",128); password=text(data.get("password"),"password",1024); project=text(data.get("project_id"),"project_id")
    if not os.path.isfile(a.db): stop("database unavailable",3)
    conn=sqlite3.connect("file:"+os.path.abspath(a.db)+"?mode=ro",uri=True)
    try:
        devices=[r[0] for r in conn.execute("SELECT device_id FROM stage_devices WHERE project_id=? AND enabled=1 ORDER BY device_id",(project,))]
        sources=[r[0] for r in conn.execute("SELECT source_id FROM live_video_sources WHERE project_id=? AND desired_enabled=1 ORDER BY source_id",(project,))]
        db=latest(conn)
    finally: conn.close()
    companions=sorted(k[1] for k in db if k[0]=="COMPANION")
    if not devices: stop("project has no enabled Stage Device",3)
    if not sources: stop("project has no desired-enabled Live Video source",3)
    if not companions: stop("no real Companion network observation is present",3)
    jar=http.cookiejar.CookieJar(); opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar)); csrf=""; base=a.hub_url.rstrip("/")
    try:
        status,login=request(opener,base+"/api/v1/auth/login","POST",{"username":username,"password":password})
        if status!=200: stop("login failed",3)
        csrf=text(login.get("csrf_token"),"csrf_token",512)
        status,payload=request(opener,base+"/api/v1/network/cockpit","GET")
        if status!=200: stop("network cockpit unavailable",3)
    finally:
        if csrf:
            try: request(opener,base+"/api/v1/auth/logout","POST",{},{"X-StageCore-CSRF":csrf})
            except BaseException: pass
    items=payload.get("targets")
    if not isinstance(items,list): stop("cockpit targets missing")
    api={}
    for item in items:
        if isinstance(item,dict):
            key=(str(item.get("target_kind") or ""),str(item.get("target_id") or ""))
            if all(key): api[key]=item
    expected=[("STAGE_DEVICE",x) for x in devices]+[("LIVE_SOURCE",x) for x in sources]+[("COMPANION",x) for x in companions]
    missing=[{"target_kind":k,"target_id":i} for k,i in expected if (k,i) not in api]
    if missing: stop("cockpit missing expected real targets: "+json.dumps(missing,separators=(",",":")),3)
    metrics=[]; numeric=[]; targets=[]
    for key in expected:
        if key not in db: stop("database lacks latest observation for %s/%s"%key,3)
        persisted=db[key]; item=api[key]; obs=item.get("observation") or {}
        for field in ("reachability","transport_state","latency_ms","jitter_ms"):
            if obs.get(field)!=persisted.get(field): stop("cockpit/database mismatch for %s/%s %s"%(key[0],key[1],field))
        measured=persisted["latency_ms"] is not None or persisted["jitter_ms"] is not None
        if measured: numeric.append({"target_kind":key[0],"target_id":key[1]})
        metrics.append({"target_kind":key[0],"target_id":key[1],"latency_ms":persisted["latency_ms"],"jitter_ms":persisted["jitter_ms"],"presentation":"NUMERIC_MS" if measured else "NOT_MEASURED"})
        targets.append({"target_kind":key[0],"target_id":key[1],"readiness":item.get("readiness"),"reason_code":item.get("reason_code"),"reachability":obs.get("reachability"),"transport_state":obs.get("transport_state")})
    result={"status":"PASS","project_id":project,"device_ids":devices,"source_ids":sources,"companion_ids":companions,"expected_target_count":len(expected),"targets":targets,"metrics":metrics,"numeric_metric_targets":numeric}
    if numeric:
        result["metric_status"]="PROVENANCE_REQUIRED"; emit(result); raise SystemExit(3)
    result["metric_status"]="NOT_MEASURED_PRESERVED"; emit(result)
if __name__=="__main__": main()
