package runtimecontrol

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func waitForSelectedCue(t *testing.T, h *runtimeHarness, sessionID, cueID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session, err := h.store.GetSession(context.Background(), sessionID)
		if err != nil { t.Fatal(err) }
		if session.CurrentCueID != nil && *session.CurrentCueID == cueID {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Cue %s not selected", cueID)
}

func overlapFixture(t *testing.T) *runtimeHarness {
	t.Helper()
	slow := json.RawMessage(`{"simulation":{"behavior":"COMPLETE","delay_ms":5000}}`)
	return newRuntimeHarness(t, slow, slow, slow)
}

func startOverlapSession(t *testing.T, h *runtimeHarness) domain.Session {
	t.Helper()
	session, result := h.service.StartSession(context.Background(), StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000008001",
	})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("start Session: %+v", result)
	}
	return session
}

func runningGo(h *runtimeHarness, sessionID, requestID string) <-chan contracts.CommandResult {
	results := make(chan contracts.CommandResult, 1)
	go func() {
		results <- h.service.Go(context.Background(), CueRequest{
			SessionID: sessionID, Issuer: "owner", RequestID: requestID,
		})
	}()
	return results
}

func TestOverlappingGoStopLatestAndStopSessionStopsOthers(t *testing.T) {
	h := overlapFixture(t)
	session := startOverlapSession(t, h)
	first := runningGo(h, session.ID, "00000000-0000-7000-8000-000000008002")
	waitForSelectedCue(t, h, session.ID, h.cues[0].ID)
	second := runningGo(h, session.ID, "00000000-0000-7000-8000-000000008003")
	waitForSelectedCue(t, h, session.ID, h.cues[1].ID)
	if n := len(h.service.activeRunsForSession(session.ID)); n != 2 {
		t.Fatalf("should have two concurrent Cue executions, got %d", n)
	}
	executions, err := h.store.ListCueExecutions(context.Background(), session.ID)
	if err != nil || len(executions) != 2 ||
		executions[0].CueID == executions[1].CueID {
		t.Fatalf("GO selection not unique: %+v error=%v", executions, err)
	}
	stop := h.service.StopCue(context.Background(), StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000008004",
	})
	if stop.Status != contracts.CommandCompleted { t.Fatalf("stop latest: %+v", stop) }
	select {
	case result := <-second:
		if result.Status != contracts.CommandCancelled { t.Fatalf("GO2 must cancel: %+v", result) }
	case <-time.After(2*time.Second):
		t.Fatal("GO2 did not cancel")
	}
	select {
	case result := <-first:
		t.Fatalf("STOP latest unexpectedly stopped older GO1: %+v", result)
	default:
	}
	stopSession := h.service.StopSession(context.Background(), StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000008005",
	})
	if stopSession.Status != contracts.CommandCompleted { t.Fatalf("session stop: %+v", stopSession) }
	select {
	case result := <-first:
		if result.Status != contracts.CommandCancelled { t.Fatalf("GO1 must cancel on session stop: %+v", result) }
	case <-time.After(2*time.Second):
		t.Fatal("GO1 did not cancel on session stop")
	}
	if n := len(h.service.activeRunsForSession(session.ID)); n != 0 {
		t.Fatalf("active Cues leaked after session stop: %d", n)
	}
}

func TestOverlappingGoEmergencyBlackoutCancelsAllAndBlocksNewGo(t *testing.T) {
	h := overlapFixture(t)
	session := startOverlapSession(t, h)
	h.service.emergencySafety = func(
		_ context.Context, _ domain.Session, _ contracts.CommandEnvelope, _ bool,
	) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }
	first := runningGo(h, session.ID, "00000000-0000-7000-8000-000000008102")
	waitForSelectedCue(t, h, session.ID, h.cues[0].ID)
	second := runningGo(h, session.ID, "00000000-0000-7000-8000-000000008103")
	waitForSelectedCue(t, h, session.ID, h.cues[1].ID)
	blackout := h.service.EmergencyBlackout(context.Background(), EmergencyRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000008104",
		Enabled: true,
	})
	if blackout.Status != contracts.CommandCompleted { t.Fatalf("blackout: %+v", blackout) }
	for i, result := range []<-chan contracts.CommandResult{first, second} {
		select {
		case r := <-result:
			if r.Status != contracts.CommandCancelled {
				t.Fatalf("GO%d status=%s want CANCELLED", i+1, r.Status)
			}
		case <-time.After(2*time.Second):
			t.Fatalf("GO%d did not terminate after blackout", i+1)
		}
	}
	next := h.service.Go(context.Background(), CueRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000008105",
	})
	if next.Status != contracts.CommandRejected || next.Error == nil ||
		next.Error.ErrorCode != "EMERGENCY_BLACKOUT_ACTIVE" {
		t.Fatalf("GO accepted during blackout: %+v", next)
	}
}

func TestRapidConcurrentGoCannotSelectSameCueTwice(t *testing.T) {
	h := overlapFixture(t)
	session := startOverlapSession(t, h)
	first := runningGo(h, session.ID, "00000000-0000-7000-8000-000000008202")
	second := runningGo(h, session.ID, "00000000-0000-7000-8000-000000008203")
	deadline := time.Now().Add(3*time.Second)
	for time.Now().Before(deadline) {
		executions, err := h.store.ListCueExecutions(context.Background(), session.ID)
		if err != nil { t.Fatal(err) }
		if len(executions) == 2 {
			if executions[0].CueID == executions[1].CueID {
				t.Fatalf("racing GO selected same Cue: %+v", executions)
			}
			break
		}
		time.Sleep(10*time.Millisecond)
	}
	executions, err := h.store.ListCueExecutions(context.Background(), session.ID)
	if err != nil || len(executions) != 2 {
		t.Fatalf("rapid GO did not create two distinct executions: %+v err=%v", executions, err)
	}
	stop := h.service.StopSession(context.Background(), StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000008204",
	})
	if stop.Status != contracts.CommandCompleted { t.Fatalf("stop session: %+v", stop) }
	for _, ch := range []<-chan contracts.CommandResult{first, second} {
		select { case <-ch: case <-time.After(2*time.Second): t.Fatal("GO leaked") }
	}
}

func TestOverlappingTargetRejectsSharedPhysicalTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		a []string
		b []string
		want string
	}{
		{"different output aliases", []string{"LIGHTING_CH1"}, []string{"VDMX"}, ""},
		{"same output alias", []string{"LIGHTING_CH1"}, []string{"LIGHTING_CH1"}, "LIGHTING_CH1"},
		{"missing target conflicts with everything", []string{"*"}, []string{"TABLET"}, "UNRESOLVED_TARGET"},
		{"no external output", nil, []string{"VDMX"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := overlappingTarget(tc.a, tc.b); got != tc.want {
				t.Fatalf("conflict=%q want %q", got, tc.want)
			}
		})
	}
}

