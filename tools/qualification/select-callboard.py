#!/usr/bin/env python3
"""Choose one real ready display from the redacted pinned-project inventory."""
import argparse
import json
import sys

REQUIRED={"display.message.show","display.countdown.show","display.alert.show","display.clear"}


def fail(message):
    print(message,file=sys.stderr)
    raise SystemExit(3)


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument("--input",required=True)
    parser.add_argument("--device-id",default="")
    args=parser.parse_args()
    data=json.load(open(args.input,encoding="utf-8"))
    if data.get("status")!="PASS" or not data.get("project_id"):
        fail("Phase 4 inventory is not a valid live baseline")
    rows=data.get("displays",[])
    eligible=[]
    for row in rows:
        caps=set(row.get("capabilities") or [])
        if not (row.get("enabled") and row.get("fresh") and row.get("connection_state")=="ONLINE"
                and row.get("readiness")=="READY" and row.get("protocol_version")=="stagecore.device/1"
                and REQUIRED.issubset(caps)):
            continue
        eligible.append(row)
    if args.device_id:
        eligible=[row for row in eligible if row["device_id"]==args.device_id]
    if len(eligible)!=1:
        fail("exactly one full-capability, freshly ONLINE/READY Stage Display target is required")
    row=eligible[0]
    print(row["device_id"]+"\t"+data["project_id"]+"\t"+
          ("1" if "display.chime.play" in row["capabilities"] else "0"))


if __name__=="__main__":
    main()
