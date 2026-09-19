#!/usr/bin/env python3
import argparse
import http.cookiejar
import json
import sys
import urllib.error
import urllib.parse
import urllib.request


def emit(value):
    print(json.dumps(value,sort_keys=True,separators=(",",":")))


def fail(message,code=1):
    emit({"status":"BLOCKED" if code==3 else "FAIL","detail":message})
    raise SystemExit(code)


def read_request():
    raw=sys.stdin.buffer.read(32769)
    if len(raw)>32768: fail("Draft HTTP request too large")
    try: value=json.loads(raw.decode("utf-8"))
    except Exception: fail("invalid Draft HTTP request")
    if not isinstance(value,dict): fail("Draft HTTP request must be an object")
    return value


def text(value,name,maximum=512):
    value=str(value or "").strip()
    if not value or len(value)>maximum or any(ord(ch)<32 for ch in value):
        fail(f"invalid {name}")
    return value


def request(opener,url,method,body,headers=None):
    raw=json.dumps(body,separators=(",",":")).encode()
    req=urllib.request.Request(url,data=raw,method=method)
    req.add_header("Content-Type","application/json")
    req.add_header("Accept","application/json")
    for k,v in (headers or {}).items(): req.add_header(k,v)
    try:
        with opener.open(req,timeout=8) as res:
            payload=res.read(1<<20)
            return res.status,json.loads(payload.decode() or "{}") if payload else {}
    except urllib.error.HTTPError as exc:
        payload=exc.read(1<<20)
        try: detail=json.loads(payload.decode() or "{}")
        except Exception: detail={"error_code":"HTTP_ERROR"}
        return exc.code,detail
    except OSError as exc:
        fail("Hub request unavailable: "+str(exc),3)


def main():
    parser=argparse.ArgumentParser(description="Bounded negative Draft recovery HTTP probes")
    parser.add_argument("--hub-url",default="http://127.0.0.1:7840")
    parser.add_argument("--mode",choices=("owner-only","show-lock"),required=True)
    args=parser.parse_args()
    data=read_request()
    username=text(data.get("username"),"username",128)
    password=text(data.get("password"),"password",1024)
    project_id=text(data.get("project_id"),"project_id",256)
    base=args.hub_url.rstrip("/")
    jar=http.cookiejar.CookieJar()
    opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    csrf=""
    try:
        status,login=request(opener,base+"/api/v1/auth/login","POST",{"username":username,"password":password})
        if status!=200:
            fail("qualification credential login failed",3)
        csrf=text(login.get("csrf_token"),"csrf_token",512)
        path="/api/v1/projects/"+urllib.parse.quote(project_id,safe="")+"/configuration/draft"
        status,detail=request(opener,base+path,"DELETE",{"reason":"StageCore physical qualification negative probe"},{"X-StageCore-CSRF":csrf})
    finally:
        if csrf:
            try: request(opener,base+"/api/v1/auth/logout","POST",{},{"X-StageCore-CSRF":csrf})
            except Exception: pass
    expected_status=403 if args.mode=="owner-only" else 423
    expected_code="OWNER_REQUIRED" if args.mode=="owner-only" else "SHOW_CONFIGURATION_LOCKED"
    if status!=expected_status or detail.get("error_code")!=expected_code:
        emit({"status":"FAIL","mode":args.mode,"http_status":status,"error_code":detail.get("error_code","")})
        return 1
    emit({"status":"PASS","mode":args.mode,"project_id":project_id,"http_status":status,"error_code":expected_code})
    return 0


if __name__=="__main__":
    raise SystemExit(main())
