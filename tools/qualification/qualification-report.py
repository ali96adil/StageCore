#!/usr/bin/env python3
"""Read-only, manifest-complete qualification dashboard and local issue register."""
import argparse
import csv
import hashlib
import io
import json
import os
import re
import tempfile
from collections import Counter
from pathlib import Path

RESULTS = ("PENDING", "PASS", "FAIL", "BLOCKED", "N/A")
SENSITIVE = re.compile(r"(?i)(password|secret|token|cookie|authorization|private.key)\s*[:=]\s*\S+")


def safe(value):
    """Keep private evidence local and avoid publishing credential-looking notes."""
    value = str(value or "").replace("\n", " ").replace("\r", " ").replace("|", "/")
    value = SENSITIVE.sub("[REDACTED]", value)
    return value[:300]


def percent(numerator, denominator):
    return round(100 * numerator / denominator, 1) if denominator else 0.0


def durable_write(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temp = tempfile.mkstemp(prefix="." + path.name + "-", dir=path.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(data)
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temp, 0o600)
        os.replace(temp, path)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def csv_value(value):
    value = safe(value)
    return "'" + value if value.startswith(("=", "+", "-", "@")) else value


def as_csv(fields, rows):
    output = io.StringIO()
    writer = csv.DictWriter(output, fieldnames=fields)
    writer.writeheader()
    for row in rows:
        writer.writerow({field: csv_value(row.get(field, "")) for field in fields})
    return output.getvalue()


def summarize(manifest, state):
    manifest_gates = {}
    group_names = {}
    group_issues = {}
    for group in manifest["groups"]:
        group_names[group["id"]] = group["title"]
        group_issues[group["id"]] = group["source_issue"]
        for gate in group["gates"]:
            gate_id = gate["id"]
            if gate_id in manifest_gates:
                raise ValueError("duplicate manifest gate: " + gate_id)
            manifest_gates[gate_id] = (group["id"], gate)
    if set(manifest_gates) != set(state.get("gates", {})):
        raise ValueError("campaign/manifest gate inventory differs; do not reuse old evidence")
    rows = []
    for gate_id, (group_id, definition) in manifest_gates.items():
        record = state["gates"][gate_id]
        status = record.get("status", "PENDING")
        if status not in RESULTS:
            raise ValueError("unknown gate status: " + gate_id)
        if record.get("method") != definition["method"]:
            raise ValueError("campaign/manifest method differs: " + gate_id)
        milestones = record.get("milestones", {})
        failed = sorted(k for k, item in milestones.items() if item.get("status") == "FAIL")
        blocked = sorted(k for k, item in milestones.items() if item.get("status") == "BLOCKED")
        rows.append({
            "gate_id": gate_id,
            "group": group_id,
            "source_issue": group_issues[group_id],
            "method": definition["method"],
            "acceptance": definition["acceptance"],
            "status": status,
            "failed_milestones": ", ".join(failed),
            "blocked_milestones": ", ".join(blocked),
            "evidence": ", ".join(str(x) for x in record.get("evidence", [])),
            "note": record.get("note", ""),
        })
    counts = Counter(row["status"] for row in rows)
    total = len(rows)
    resolved = counts["PASS"] + counts["N/A"]
    assessed = resolved + counts["FAIL"] + counts["BLOCKED"]
    summary = {
        "campaign_id": state.get("campaign_id"),
        "pins": state.get("pins", {}),
        "total": total,
        "counts": {key: counts[key] for key in RESULTS},
        "completed": resolved,
        "remaining": total - resolved,
        "completion_percent": percent(resolved, total),
        "assessed_percent": percent(assessed, total),
        "verified_percent": percent(counts["PASS"], total - counts["N/A"]),
        "groups": [],
    }
    for group_id, title in group_names.items():
        subset = [row for row in rows if row["group"] == group_id]
        group_counts = Counter(row["status"] for row in subset)
        summary["groups"].append({
            "id": group_id, "title": title, "total": len(subset),
            "completed": group_counts["PASS"] + group_counts["N/A"],
            "completion_percent": percent(group_counts["PASS"] + group_counts["N/A"], len(subset)),
            "counts": {key: group_counts[key] for key in RESULTS},
        })
    return summary, rows


def markdown(summary, rows):
    c = summary["counts"]
    lines = [
        "# StageCore qualification — cumulative progress", "",
        "Campaign: `" + safe(summary["campaign_id"]) + "`", "",
        "**Completed: " + str(summary["completed"]) + "/" + str(summary["total"]) +
        " (" + str(summary["completion_percent"]) + "%)**; remaining: " + str(summary["remaining"]),
        "",
        "- Verified applicable gates: " + str(summary["verified_percent"]) + "%",
        "- Assessed (including failures and blockers): " + str(summary["assessed_percent"]) + "%",
        "- PASS: " + str(c["PASS"]) + "; FAIL: " + str(c["FAIL"]) +
        "; BLOCKED: " + str(c["BLOCKED"]) + "; PENDING: " + str(c["PENDING"]) +
        "; N/A: " + str(c["N/A"]), "",
        "N/A is counted as completed only when an explicit permitted reason is recorded. "
        "FAIL and BLOCKED never count as completed. Percentages refer only to this manifest, "
        "not to unlisted features or physical checks.", "",
        "| Area | Completed | Total | Progress | FAIL | BLOCKED |",
        "| --- | ---: | ---: | ---: | ---: | ---: |",
    ]
    for group in summary["groups"]:
        counts = group["counts"]
        lines.append("| " + safe(group["title"]) + " | " + str(group["completed"]) +
                     " | " + str(group["total"]) + " | " + str(group["completion_percent"]) +
                     "% | " + str(counts["FAIL"]) + " | " + str(counts["BLOCKED"]) + " |")
    lines.extend(["", "## Failed gates / failed milestones", ""])
    defects = [row for row in rows if row["status"] == "FAIL" or row["failed_milestones"]]
    if defects:
        for row in defects:
            lines.append("- `" + row["gate_id"] + "` (#" + str(row["source_issue"]) +
                         "): " + safe(row["acceptance"]) +
                         (" — failed steps: " + safe(row["failed_milestones"]) if row["failed_milestones"] else ""))
    else:
        lines.append("None recorded.")
    lines.extend(["", "## Pending manual / physical confirmation", ""])
    physical = [row for row in rows if row["method"] in ("AUTO_PHYSICAL", "MANUAL") and row["status"] not in ("PASS", "N/A")]
    lines.append(str(len(physical)) + " physical/manual gates still require completion or documented N/A.")
    lines.extend(["", "Local files: defects.csv, blocked.csv, manual.csv, summary.json. "
                  "Keep evidence and notes private; no GitHub issue is created automatically.", ""])
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Read-only StageCore qualification progress and issue exports")
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--state", required=True)
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()
    raw = Path(args.manifest).read_bytes()
    manifest = json.loads(raw)
    state = json.loads(Path(args.state).read_text(encoding="utf-8"))
    if state.get("manifest_sha256") != hashlib.sha256(raw).hexdigest():
        parser.error("manifest hash mismatch: preserve the existing campaign and repin/migrate explicitly")
    summary, rows = summarize(manifest, state)
    fields = ("gate_id", "group", "source_issue", "method", "acceptance",
              "status", "failed_milestones", "blocked_milestones", "evidence", "note")
    defects = [r for r in rows if r["status"] == "FAIL" or r["failed_milestones"]]
    blocked = [r for r in rows if r["status"] == "BLOCKED" or r["blocked_milestones"]]
    manual = [r for r in rows if r["method"] in ("AUTO_PHYSICAL", "MANUAL")
              and r["status"] not in ("PASS", "N/A")]
    out = Path(args.output_dir)
    durable_write(out / "summary.json", json.dumps(summary, indent=2, sort_keys=True) + "\n")
    durable_write(out / "summary.md", markdown(summary, rows))
    durable_write(out / "defects.csv", as_csv(fields, defects))
    durable_write(out / "blocked.csv", as_csv(fields, blocked))
    durable_write(out / "manual.csv", as_csv(fields, manual))
    c = summary["counts"]
    print("QUALIFICATION " + str(summary["completed"]) + "/" + str(summary["total"]) +
          " (" + str(summary["completion_percent"]) + "%) completed; " +
          str(summary["remaining"]) + " remaining")
    print("PASS=" + str(c["PASS"]) + " FAIL=" + str(c["FAIL"]) +
          " BLOCKED=" + str(c["BLOCKED"]) + " PENDING=" + str(c["PENDING"]) +
          " N/A=" + str(c["N/A"]))
    print("ISSUES " + str(len(defects)) + " failed gates/steps; " +
          str(len(blocked)) + " blocked gates/steps; " + str(len(manual)) +
          " physical/manual gates remaining")
    print("REPORT " + str(out / "summary.md"))


if __name__ == "__main__":
    main()
