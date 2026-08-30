package contracts

const (
	CueTimingObservedEventType = "cue.timing_observed"
	CueTimingCaptureVersion1   = 1
)

type CueTimingPathKind string

const (
	CueTimingPathStart       CueTimingPathKind = "START"
	CueTimingPathStartAtCue  CueTimingPathKind = "START_AT_CUE"
	CueTimingPathNext        CueTimingPathKind = "NEXT"
	CueTimingPathRepeat      CueTimingPathKind = "REPEAT"
	CueTimingPathForwardJump CueTimingPathKind = "FORWARD_JUMP"
	CueTimingPathBackJump    CueTimingPathKind = "BACK_JUMP"
)

const (
	CueTimingQualityRawUnassessed = "RAW_UNASSESSED"
	CueTimingClockHubUTCWall      = "HUB_UTC_WALL"
	CueTimingClockUnassessed      = "UNASSESSED"
	CueTimingIntervalSingleHub    = "SINGLE_HUB"
	CueTimingRequestEnvelopeUTC   = "COMMAND_ENVELOPE_UTC"
	CueTimingRequestUnavailable   = "UNAVAILABLE"
)

type CueTimingPath struct {
	Kind          CueTimingPathKind `json:"kind"`
	FromCueID     *string           `json:"from_cue_id"`
	ToCueID       string            `json:"to_cue_id"`
	SkippedCueIDs []string          `json:"skipped_cue_ids"`
}

type CueTimingClock struct {
	Basis         string `json:"basis"`
	Health        string `json:"health"`
	IntervalScope string `json:"interval_scope"`
	RequestBasis  string `json:"request_basis"`
}

type CueTimingObservation struct {
	CaptureVersion         int            `json:"capture_version"`
	Quality                string         `json:"quality"`
	SessionType            string         `json:"session_type"`
	SessionStartedAtUS     int64          `json:"session_started_at_us"`
	CueExecutionID         string         `json:"cue_execution_id"`
	CueID                  string         `json:"cue_id"`
	CueStartedAtUS         int64          `json:"cue_started_at_us"`
	TriggerSource          string         `json:"trigger_source"`
	ManualOverride         bool           `json:"manual_override"`
	RequestIssuedAtUS      *int64         `json:"request_issued_at_us"`
	RequestToStartUS       *int64         `json:"request_to_start_us"`
	SessionElapsedUS       int64          `json:"session_elapsed_us"`
	PreviousCueExecutionID *string        `json:"previous_cue_execution_id"`
	PreviousCueID          *string        `json:"previous_cue_id"`
	PreviousCueStartedAtUS *int64         `json:"previous_cue_started_at_us"`
	CueToCueElapsedUS      *int64         `json:"cue_to_cue_elapsed_us"`
	Path                   CueTimingPath  `json:"path"`
	Clock                  CueTimingClock `json:"clock"`
}
