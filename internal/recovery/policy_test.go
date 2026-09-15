package recovery

import (
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestEvaluateRestartPreservesOnlyCleanInternalRehearsal(t *testing.T) {
	decision := EvaluateRestart(RestartContext{
		SessionType:       domain.SessionRehearsal,
		LifecycleState:    domain.SessionLifecycleActive,
		TimecodeAuthority: TimecodeAuthorityInternal,
	})
	if decision.Disposition != DispositionPreserve || decision.ReasonCode != ReasonCleanInternalTimecodeRehearsal {
		t.Fatalf("decision=%+v", decision)
	}
	if !decision.Automatic || decision.ReplayAllowed || decision.ManualConfirmationRequired {
		t.Fatalf("preserve authority=%+v", decision)
	}
}

func TestEvaluateRestartRequiresManualReconstructionForSimulationCheckpoint(t *testing.T) {
	decision := EvaluateRestart(RestartContext{
		SessionType:          domain.SessionSimulation,
		LifecycleState:       domain.SessionLifecycleActive,
		HasTrustedCheckpoint: true,
	})
	if decision.Disposition != DispositionManualConfirmation || decision.ReasonCode != ReasonSimulationCheckpointManual {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.Automatic || decision.ReplayAllowed || !decision.ManualConfirmationRequired {
		t.Fatalf("checkpoint recovery authority=%+v", decision)
	}
}

func TestEvaluateRestartFailsClosedForUnsafeAuthorities(t *testing.T) {
	tests := []struct {
		name   string
		input  RestartContext
		reason string
	}{
		{
			name:   "show",
			input:  RestartContext{SessionType: domain.SessionShow, LifecycleState: domain.SessionLifecycleActive, TimecodeAuthority: TimecodeAuthorityInternal},
			reason: ReasonShowRestartFailClosed,
		},
		{
			name:   "simulation without checkpoint",
			input:  RestartContext{SessionType: domain.SessionSimulation, LifecycleState: domain.SessionLifecycleActive, TimecodeAuthority: TimecodeAuthorityInternal},
			reason: ReasonSimulationRestartFailClosed,
		},
		{
			name:   "external timecode",
			input:  RestartContext{SessionType: domain.SessionRehearsal, LifecycleState: domain.SessionLifecycleActive, TimecodeAuthority: TimecodeAuthorityExternal},
			reason: ReasonExternalTimecodeAuthority,
		},
		{
			name:   "in flight",
			input:  RestartContext{SessionType: domain.SessionRehearsal, LifecycleState: domain.SessionLifecycleActive, TimecodeAuthority: TimecodeAuthorityInternal, HasInFlightWork: true},
			reason: ReasonInFlightExecutionInterrupted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := EvaluateRestart(tt.input)
			if decision.Disposition != DispositionAbort || decision.ReasonCode != tt.reason {
				t.Fatalf("decision=%+v", decision)
			}
			if !decision.Automatic || decision.ReplayAllowed || decision.ManualConfirmationRequired {
				t.Fatalf("abort authority=%+v", decision)
			}
		})
	}
}

func TestClassifyTimecodeAuthorityIsConservative(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want TimecodeAuthority
	}{
		{name: "internal", raw: `{"targets":[{"logical_type":"TIMECODE_SOURCE","configuration":{"kind":"INTERNAL"}}]}`, want: TimecodeAuthorityInternal},
		{name: "external", raw: `{"targets":[{"logical_type":"TIMECODE_SOURCE","configuration":{"kind":"MTC"}}]}`, want: TimecodeAuthorityExternal},
		{name: "missing", raw: `{"targets":[]}`, want: TimecodeAuthorityMissing},
		{name: "ambiguous", raw: `{"targets":[{"logical_type":"TIMECODE_SOURCE","configuration":{"kind":"INTERNAL"}},{"logical_type":"TIMECODE_SOURCE","configuration":{"kind":"INTERNAL"}}]}`, want: TimecodeAuthorityAmbiguous},
		{name: "invalid manifest", raw: `{`, want: TimecodeAuthorityInvalid},
		{name: "invalid config", raw: `{"targets":[{"logical_type":"TIMECODE_SOURCE","configuration":{}}]}`, want: TimecodeAuthorityInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyTimecodeAuthority([]byte(tt.raw)); got != tt.want {
				t.Fatalf("authority=%s want %s", got, tt.want)
			}
	}
}
