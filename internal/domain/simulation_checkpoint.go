package domain

import (
	"encoding/json"
	"time"
)

// SimulationCheckpointStateContractVersion1 is the durable F-024 Digital Twin
// checkpoint state contract understood by the current runtime. Recovery may
// advertise a checkpoint only when this version is supported.
const SimulationCheckpointStateContractVersion1 = 1

// SimulationCheckpoint is durable F-024 simulation state. TwinState is
// simulation-only truth; it must never be presented as physical device state.
type SimulationCheckpoint struct {
	ID                   string
	SourceSessionID      string
	ProjectID            string
	RuntimeSnapshotID    string
	StateContractVersion int
	CapturedAt           time.Time
	CurrentCueID         *string
	LastCompletedCueID   *string
	NextCueID            *string
	TwinState            json.RawMessage
	ContentHash          string
}

type SimulationRangeMetadata struct {
	EndCueID string `json:"end_cue_id"`
}

type SimulationCheckpointStartMetadata struct {
	CheckpointID string `json:"checkpoint_id"`
}
