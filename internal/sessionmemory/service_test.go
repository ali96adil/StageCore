package sessionmemory

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSessionDetailReturnsOperatorExecutionTrace(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Trace", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "Q1", Name: "Opening", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, []domain.Action{
		{
			OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "VIDEO-MAIN",
			CapabilityKey: "osc.send", PriorityClass: domain.PriorityP1, Enabled: true,
		},
		{
			OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetRef: "ABLETON-IAC",
			CapabilityKey: "midi.send", PriorityClass: domain.PriorityP1, Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Dress")
	if err != nil {
		t.Fatal(err)
	}
	cueExecution, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "corr-trace", "operator")
	if err != nil {
		t.Fatal(err)
	}
	actionExecution, err := s.CreateActionExecution(ctx, cueExecution.ID, cue.Actions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishActionExecution(ctx, actionExecution.ID, domain.ExecutionFailed, 17, "UDP send rejected", strptr("OSC_SEND_FAILED")); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"action_execution_id": actionExecution.ID,
		"action_id":           cue.Actions[0].ID,
		"result":              domain.ExecutionFailed,
		"ack_level":           "TRANSPORT_ONLY",
		"dispatch_attempts":   2,
		"latency_ms":          17,
		"response_summary":    "UDP send rejected",
		"error_code":          "OSC_SEND_FAILED",
	})
	sessionID := session.ID
	if _, err := s.AppendEvent(ctx, &sessionID, contracts.EventEnvelope{
		EventType: "action.failed", SchemaVersion: contracts.SchemaVersion1, Source: "test",
		ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID,
		CorrelationID: "corr-trace", Priority: "P1", Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}

	completedAction, err := s.CreateActionExecution(ctx, cueExecution.ID, cue.Actions[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishActionExecution(ctx, completedAction.ID, domain.ExecutionCompleted, 4, "CoreMIDI local transport accepted", nil); err != nil {
		t.Fatal(err)
	}
	completedPayload, _ := json.Marshal(map[string]any{
		"action_execution_id": completedAction.ID,
		"action_id":           cue.Actions[1].ID,
		"result":              domain.ExecutionCompleted,
		"ack_level":           "TRANSPORT_ONLY",
		"dispatch_attempts":   1,
		"latency_ms":          4,
		"response_summary":    "CoreMIDI local transport accepted",
	})
	if _, err := s.AppendEvent(ctx, &sessionID, contracts.EventEnvelope{
		EventType: "action.completed", SchemaVersion: contracts.SchemaVersion1, Source: "test",
		ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID,
		CorrelationID: "corr-trace", Priority: "P1", Payload: completedPayload,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, cueExecution.ID, domain.ExecutionFailed); err != nil {
		t.Fatal(err)
	}
	if err := s.EndSession(ctx, session.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}

	service := New(s)
	list, err := service.List(ctx, project.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].SessionID != session.ID || list[0].Status != domain.SessionCompleted {
		t.Fatalf("session list=%+v", list)
	}
	detail, err := service.Detail(ctx, project.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Cues) != 1 || detail.Cues[0].Name != "Opening" || detail.Cues[0].DisplayLabel != "Q1" {
		t.Fatalf("cue trace=%+v", detail.Cues)
	}
	if len(detail.Cues[0].Actions) != 2 {
		t.Fatalf("action trace=%+v", detail.Cues[0].Actions)
	}
	action := detail.Cues[0].Actions[0]
	if action.TargetRef != "VIDEO-MAIN" || action.CapabilityKey != "osc.send" || action.AckLevel != "TRANSPORT_ONLY" || action.DeliveryEvidence != "EXTERNAL_EXECUTION_UNKNOWN" || action.DispatchAttempts == nil || *action.DispatchAttempts != 2 || action.ErrorCode == nil || *action.ErrorCode != "OSC_SEND_FAILED" || action.ResponseSummary != "UDP send rejected" {
		t.Fatalf("action trace=%+v", action)
	}
	completed := detail.Cues[0].Actions[1]
	if completed.Result != domain.ExecutionCompleted || completed.AckLevel != "TRANSPORT_ONLY" || completed.DeliveryEvidence != "EXTERNAL_EXECUTION_UNKNOWN" || completed.DispatchAttempts == nil || *completed.DispatchAttempts != 1 {
		t.Fatalf("transport-only completion was overstated in the trace: %+v", completed)
	}
}

func TestDeliveryEvidenceDoesNotPromoteTransportReceiptToExecution(t *testing.T) {
	cases := []struct {
		ack  contracts.AckLevel
		want string
	}{
		{contracts.AckNone, "UNKNOWN"},
		{contracts.AckTransportOnly, "EXTERNAL_EXECUTION_UNKNOWN"},
		{contracts.AckAccepted, "OUTPUT_ACCEPTED"},
		{contracts.AckDevice, "DEVICE_ACKNOWLEDGED"},
		{contracts.AckApplication, "APPLICATION_ACKNOWLEDGED"},
		{contracts.AckVerifiedState, "STATE_VERIFIED"},
	}
	for _, tc := range cases {
		t.Run(string(tc.ack), func(t *testing.T) {
			if got := deliveryEvidence(string(tc.ack)); got != tc.want {
				t.Fatalf("delivery evidence=%q, want %q", got, tc.want)
			}
		})
	}
}

func strptr(value string) *string { return &value }
