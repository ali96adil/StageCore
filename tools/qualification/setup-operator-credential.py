#!/usr/bin/env python3
import argparse
import getpass
import json
import os
from pathlib import Path

parser = argparse.ArgumentParser(description="Store local StageCore qualification Operator credential")
parser.add_argument(
    "--output",
    default=str(Path.home() / ".config" / "stagecore" / "qualification-operator.json"),
)
parser.add_argument("--username", default="")
args = parser.parse_args()

username = args.username.strip() or input("StageCore Operator username: ").strip()
if not username:
    raise SystemExit("username is required")
password = getpass.getpass("StageCore Operator password: ")
if not password:
    raise SystemExit("password is required")

path = Path(args.output).expanduser()
path.parent.mkdir(parents=True, exist_ok=True)
os.chmod(path.parent, 0o700)
temp = path.with_name(path.name + ".tmp")
with open(temp, "w", encoding="utf-8") as fh:
    json.dump({"username": username, "password": password}, fh)
    fh.write("\n")
os.chmod(temp, 0o600)
os.replace(temp, path)
print(f"Stored local qualification credential: {path}")
