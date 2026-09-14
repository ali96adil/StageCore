package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/recovery"
)

func TestHubRestartRecordsPreserveDecisionWithoutReplayAuthority(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "preserve-evidence")
	if err != nil {
		t.Fatal(err)
	}

	count, err := s.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("reconciled Sessions=%d want 0", count)
	}
	assertRecoveryDecisionEvent(t, s, session.ID, recovery.DispositionPreserve, recovery.ReasonCleanInternalTimecodeRehearsal, false)
}

func TestHubRestartRecordsFailClosedShowDecisionWithoutReplayAuthority(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "show-evidence")
	if err != nil {
		t.Fatal(err)
	}

	count, err := s.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled Sessions=%d want 1", count)
	}
	assertRecoveryDecisionEvent(t, s, session.ID, recovery.DispositionAbort, recovery.ReasonShowRestartFailClosed, false)
}

func TestHubRestartRecordsInFlightDecisionAndCancelsWork(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, cue := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "in-flight-evidence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "recovery-in-flight", "test"); err != nil {
		t.Fatal(err)
	}

	count, err := s.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled Sessions=%d want 1", count)
	}
	assertRecoveryDecisionEvent(t, s, session.ID, recovery.DispositionAbort, recovery.ReasonInFlightExecutionInterrupted, true)
}

func assertRecoveryDecisionEvent(t *testing.T, s interface {
	ListEvents(context.Context, string) ([]contracts.EventEnvelope, error)
}, sessionID string, disposition recovery.Disposition, reason string, inFlight bool) {
	t.Helper()
	events, err := s.ListEvents(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, event := range events {
		if event.EventType != "runtime.recovery.decision" {
			continue
		}
		found = true
		var payload struct {
			DecisionVersion            int                        `json:"decision_version"`
			Scope                      string                     `json:"scope"`
			Disposition                recovery.Disposition       `json:"disposition"`
			ReasonCode                 string                     `json:"reason_code"`
			Automatic                  bool                       `json:"automatic"`
			ReplayAllowed              bool                       `json:"replay_allowed"`
			ManualConfirmationRequired bool                       `json:"manual_confirmation_required"`
			TimecodeAuthority          recovery.TimecodeAuthority `json:"timecode_authority"`
			InFlightWork               bool                       `json:"in_flight_work"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.DecisionVersion != 1 || payload.Scope != "SESSION_RESTART" || payload.Disposition != disposition || payload.ReasonCode != reason {
			t.Fatalf("recovery payload=%+v", payload)
		}
		if !payload.Automatic || payload.ReplayAllowed || payload.ManualConfirmationRequired || payload.InFlightWork != inFlight {
			t.Fatalf("recovery authority=%+v", payload)
		}
	}
	if !found {
		t.Fatalf("no runtime.recovery.decision event: %+v", events)
	}
}
