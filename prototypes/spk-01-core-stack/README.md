# SPK-01 — Core Stack Prototype

This is a deliberately small executable spike for the first StageCore runtime loop.

It proves, with no external services:

`Project -> Cue -> Publish -> GO -> simulated Action -> Event -> persistent history`

## Run

```bash
go test ./...
go run ./cmd/stagecore-spike -addr :8080 -data ./var/spike-state.json
```

Then open `http://localhost:8080`.

## What is intentionally temporary

The prototype file store is **not** the selected production persistence engine. It exists so the transport/runtime shape can be compiled and tested with the Go standard library only. The accepted implementation database for StageCore is SQLite in WAL mode; replacing this file persister with the SQLite repository is the next implementation slice.

The embedded HTML/JavaScript page is also only a spike harness. The product UI decision is TypeScript + React + Vite, served as built static assets by the Hub.
