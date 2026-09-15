package store

import (
	"encoding/json"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

// simulationCheckpointRecoverableForSnapshot validates the minimum durable
// F-024 Digital Twin payload shape required before a checkpoint may be
// advertised as restart reconstruction authority. Integrity alone is not
// enough: the embedded state contract and immutable Runtime Snapshot binding
// must also match the current runtime.
func simulationCheckpointRecoverableForSnapshot(checkpoint domain.SimulationCheckpoint, runtimeSnapshotID string) bool {
	if checkpoint.StateContractVersion != domain.SimulationCheckpointStateContractVersion1 {
		return false
	}
	var state struct {
		Version           int               `json:"version"`
		RuntimeSnapshotID string            `json:"runtime_snapshot_id"`
		Targets           []json.RawMessage `json:"targets"`
		Faults            []json.RawMessage `json:"faults"`
	}
	if err := json.Unmarshal(checkpoint.TwinState, &state); err != nil {
		return false
	}
	if state.Version != checkpoint.StateContractVersion {
		return false
	}
	if strings.TrimSpace(state.RuntimeSnapshotID) == "" || state.RuntimeSnapshotID != strings.TrimSpace(runtimeSnapshotID) {
		return false
	}
	// Nil slices are accepted because older canonical snapshots may omit an
	// empty collection. A wrong JSON type (for example an object) fails unmarshal.
	return true
}
