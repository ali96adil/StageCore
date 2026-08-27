# 09 — Notes, Logs & Session Memory

## 1. Notes

MVP Notes are lightweight operational records, not a full collaborative document system.

Required fields:

- note ID;
- text;
- created timestamp;
- lifecycle: `OPEN | RESOLVED`;
- optional Cue reference;
- optional Session reference;
- optional category: `OPERATOR | DIRECTOR | TECHNICAL | OTHER`.

Required operations:

- create;
- edit while not archived;
- resolve/reopen;
- filter by Cue/session/status.

## 2. Rehearsal / Show Session Memory

Each Session records at minimum:

- project ID;
- Runtime Snapshot ID;
- mode;
- start/end/interrupted status;
- CueExecution records;
- ActionExecution records;
- runtime errors relevant to execution;
- operator Notes.

## 3. Execution Trace

CueExecution must show:

- Cue identity/label;
- requested/start/end timestamps;
- result;
- initiating command/correlation ID;
- child Action executions.

ActionExecution must show:

- target/capability;
- start/end;
- result;
- acknowledgement level when known;
- error code/message when failed.

## 4. Runtime Log vs Diagnostic Log

MVP separates operator-useful execution history from verbose diagnostic logs.

The operator view should not require reading raw log files. Raw structured logs may exist for development and troubleshooting.

## 5. Persistence

- completed execution records are append-oriented;
- reopening Project does not remove past Sessions;
- application restart should preserve completed Session records;
- if runtime terminates unexpectedly, active Session is marked interrupted/recoverable rather than cleanly completed.

## 6. Acceptance

- Run three Cues, restart StageCore, and inspect the same three execution records.
- Failed Action displays the actual failure in Session history.
- Note attached to Cue can be retrieved from that Cue.
- Resolve Note does not erase its historical identity.
