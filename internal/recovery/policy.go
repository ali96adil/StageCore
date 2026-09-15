package recovery

import (
	"encoding/json"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

// Disposition is the restart-time action StageCore is allowed to take for a
// runtime Session. The vocabulary is intentionally small and fail-closed.
type Disposition string

const (
	DispositionPreserve           Disposition = "PRESERVE"
	DispositionAbort              Disposition = "ABORT"
	DispositionManualConfirmation Disposition = "MANUAL_CONFIRMATION_REQUIRED"
)

// TimecodeAuthority describes only what can be proven from the immutable
// Runtime Snapshot. It does not probe or infer live transport health.
type TimecodeAuthority string

const (
	TimecodeAuthorityInternal  TimecodeAuthority = "INTERNAL"
	TimecodeAuthorityExternal  TimecodeAuthority = "EXTERNAL"
	TimecodeAuthorityMissing   TimecodeAuthority = "MISSING"
	TimecodeAuthorityAmbiguous TimecodeAuthority = "AMBIGUOUS"
	TimecodeAuthorityInvalid   TimecodeAuthority = "INVALID"
)

const (
	ReasonCleanInternalTimecodeRehearsal = "CLEAN_INTERNAL_TIMECODE_REHEARSAL"
	ReasonShowRestartFailClosed           = "SHOW_RESTART_FAIL_CLOSED"
	ReasonSimulationRestartFailClosed     = "SIMULATION_RESTART_FAIL_CLOSED"
	ReasonSimulationCheckpointManual      = "SIMULATION_CHECKPOINT_MANUAL_RECONSTRUCTION"
	ReasonUnsupportedSessionType          = "UNSUPPORTED_SESSION_TYPE"
	ReasonSessionLifecycleNotActive       = "SESSION_LIFECYCLE_NOT_ACTIVE"
	ReasonExternalTimecodeAuthority       = "EXTERNAL_TIMECODE_AUTHORITY"
	ReasonMissingTimecodeAuthority        = "MISSING_TIMECODE_AUTHORITY"
	ReasonAmbiguousTimecodeAuthority      = "AMBIGUOUS_TIMECODE_AUTHORITY"
	ReasonInvalidTimecodeAuthority        = "INVALID_TIMECODE_AUTHORITY"
	ReasonInFlightExecutionInterrupted    = "IN_FLIGHT_EXECUTION_INTERRUPTED"
)

// RestartContext contains only authoritative facts needed to classify a Hub
// restart. Recovery policy must not guess device state or replay intent.
type RestartContext struct {
	SessionType          domain.SessionType
	LifecycleState       domain.SessionLifecycleState
	TimecodeAuthority    TimecodeAuthority
	HasInFlightWork      bool
	HasTrustedCheckpoint bool
}

// Decision is durable policy evidence. Reconnect/restart never authorizes live
// command replay by itself. ManualConfirmationRequired means StageCore can
// point at trusted recovery state, but an operator must deliberately start a
// new compatible runtime path before that state is used.
type Decision struct {
	Disposition                Disposition
	ReasonCode                 string
	Automatic                  bool
	ReplayAllowed              bool
	ManualConfirmationRequired bool
}

// EvaluateRestart preserves existing fail-closed restart behavior while making
// trusted simulation checkpoint recovery visible. A checkpoint never resumes a
// crashed SIMULATION automatically: the interrupted Session ends and the
// operator may explicitly start a new SIMULATION from that checkpoint.
func EvaluateRestart(input RestartContext) Decision {
	abort := func(reason string) Decision {
		return Decision{
			Disposition:   DispositionAbort,
			ReasonCode:    reason,
			Automatic:     true,
			ReplayAllowed: false,
		}
	}
	manual := func(reason string) Decision {
		return Decision{
			Disposition:                 DispositionManualConfirmation,
			ReasonCode:                  reason,
			Automatic:                   false,
			ReplayAllowed:               false,
			ManualConfirmationRequired: true,
		}
	}

	switch input.SessionType {
	case domain.SessionShow:
		return abort(ReasonShowRestartFailClosed)
	case domain.SessionSimulation:
		if input.LifecycleState == domain.SessionLifecycleActive && input.HasTrustedCheckpoint {
			return manual(ReasonSimulationCheckpointManual)
		}
		return abort(ReasonSimulationRestartFailClosed)
	case domain.SessionRehearsal:
		// Continue below.
	default:
		return abort(ReasonUnsupportedSessionType)
	}
	if input.LifecycleState != domain.SessionLifecycleActive {
		return abort(ReasonSessionLifecycleNotActive)
	}

	switch input.TimecodeAuthority {
	case TimecodeAuthorityInternal:
		// Continue below.
	case TimecodeAuthorityExternal:
		return abort(ReasonExternalTimecodeAuthority)
	case TimecodeAuthorityMissing:
		return abort(ReasonMissingTimecodeAuthority)
	case TimecodeAuthorityAmbiguous:
		return abort(ReasonAmbiguousTimecodeAuthority)
	default:
		return abort(ReasonInvalidTimecodeAuthority)
	}
	if input.HasInFlightWork {
		return abort(ReasonInFlightExecutionInterrupted)
	}
	return Decision{
		Disposition:   DispositionPreserve,
		ReasonCode:    ReasonCleanInternalTimecodeRehearsal,
		Automatic:     true,
		ReplayAllowed: false,
	}
}

// ClassifyTimecodeAuthority classifies immutable snapshot configuration. The
// result is deliberately conservative: exactly one valid TIMECODE_SOURCE is
// required before INTERNAL authority can be claimed.
func ClassifyTimecodeAuthority(raw []byte) TimecodeAuthority {
	var manifest struct {
		Targets []struct {
			LogicalType   string          `json:"logical_type"`
			Configuration json.RawMessage `json:"configuration"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return TimecodeAuthorityInvalid
	}
	matches := make([]json.RawMessage, 0, 1)
	for _, target := range manifest.Targets {
		if strings.EqualFold(strings.TrimSpace(target.LogicalType), "TIMECODE_SOURCE") {
			matches = append(matches, target.Configuration)
		}
	}
	if len(matches) == 0 {
		return TimecodeAuthorityMissing
	}
	if len(matches) != 1 {
		return TimecodeAuthorityAmbiguous
	}
	var cfg struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(matches[0], &cfg); err != nil || strings.TrimSpace(cfg.Kind) == "" {
		return TimecodeAuthorityInvalid
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Kind), "INTERNAL") {
		return TimecodeAuthorityInternal
	}
	return TimecodeAuthorityExternal
}
