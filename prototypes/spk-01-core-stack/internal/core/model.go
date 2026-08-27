package core

import "time"

type State struct {
	Version         int                 `json:"version"`
	ActiveProjectID string              `json:"active_project_id,omitempty"`
	Projects        map[string]*Project `json:"projects"`
}

type Project struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	DraftCues       []Cue       `json:"draft_cues"`
	Published       *Snapshot   `json:"published,omitempty"`
	CurrentCueIndex int         `json:"current_cue_index"`
	Executions      []Execution `json:"executions"`
}

type Cue struct {
	ID      string `json:"id"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type Snapshot struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Cues      []Cue     `json:"cues"`
}

type Execution struct {
	ID          string    `json:"id"`
	CueID       string    `json:"cue_id"`
	CueName     string    `json:"cue_name"`
	SnapshotID  string    `json:"snapshot_id"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

type Event struct {
	ID        string         `json:"event_id"`
	Type      string         `json:"event_type"`
	Occurred  time.Time      `json:"occurred_at"`
	ProjectID string         `json:"project_id,omitempty"`
	Snapshot  string         `json:"runtime_snapshot_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}
