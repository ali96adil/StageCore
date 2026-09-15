package store_test

import (
	"encoding/json"
	"testing"
)

func recoveryCheckpointTwinState(t *testing.T, runtimeSnapshotID, marker string) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"version":             1,
		"session_id":          "source-simulation",
		"runtime_snapshot_id": runtimeSnapshotID,
		"targets":             []any{},
		"faults":              []any{},
		"marker":              marker,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}
