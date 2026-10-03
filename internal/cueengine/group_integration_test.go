package cueengine_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestLinkedCueGroupRunsTogetherAndChildIsSkippedByNextGO(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Cue Group", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}

	childAction := action("SEQUENTIAL", "COMPLETE", 80, "")
	childAction.OrderIndex = 0
	child, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Child Cue", OrderIndex: 2,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{childAction})
	if err != nil {
		t.Fatal(err)
	}

	policy, _ := json.Marshal(map[string]any{
		"linked_cue_ids": []string{child.ID},
		"linked_cue_mode": "TOGETHER",
	})
	parentAction := action("SEQUENTIAL", "COMPLETE", 80, "")
	parentAction.OrderIndex = 0
	parent, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Parent Cue", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: policy,
	}, []domain.Action{parentAction})
	if err != nil {
		t.Fatal(err)
	}

	nextAction := action("SEQUENTIAL", "COMPLETE", 0, "")
	nextAction.OrderIndex = 0
	next, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "3", Name: "Next Top Level", OrderIndex: 3,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{nextAction})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "group test")
	if err != nil {
		t.Fatal(err)
	}
	engine := cueengine.New(s)
	command := func() contracts.CommandEnvelope {
		id, err := stageid.New()
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(cueengine.CueGoPayload{})
		return contracts.CommandEnvelope{
			CommandID: id, CommandType: cueengine.CueGoCommandType,
			SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(),
			ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID,
			Issuer: "test.operator", Priority: "P1", Payload: payload,
		}
	}

	first := engine.ExecuteCueGo(ctx, session.ID, command())
	if first.Status != contracts.CommandCompleted {
		t.Fatalf("group GO=%+v", first)
	}
	var firstPayload struct {
		CueID        string   `json:"cue_id"`
		LinkedCueIDs []string `json:"linked_cue_ids"`
	}
	if err := json.Unmarshal(first.Payload, &firstPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload.CueID != parent.ID || len(firstPayload.LinkedCueIDs) != 1 || firstPayload.LinkedCueIDs[0] != child.ID {
		t.Fatalf("group payload=%+v", firstPayload)
	}

	executions, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil || len(executions) != 1 {
		t.Fatalf("cue executions=%+v err=%v", executions, err)
	}
	actionExecutions, err := s.ListActionExecutions(ctx, executions[0].ID)
	if err != nil || len(actionExecutions) != 2 {
		t.Fatalf("group action executions=%+v err=%v", actionExecutions, err)
	}
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	startedBeforeCompletion := 0
	for _, event := range events {
		if event.EventType == "action.completed" {
			break
		}
		if event.EventType == "action.started" {
			startedBeforeCompletion++
		}
	}
	if startedBeforeCompletion != 2 {
		t.Fatalf("linked Cue branches did not start together; action.started before first completion=%d events=%+v", startedBeforeCompletion, events)
	}

	second := engine.ExecuteCueGo(ctx, session.ID, command())
	if second.Status != contracts.CommandCompleted {
		t.Fatalf("second GO=%+v", second)
	}
	var secondPayload struct {
		CueID string `json:"cue_id"`
	}
	if err := json.Unmarshal(second.Payload, &secondPayload); err != nil {
		t.Fatal(err)
	}
	if secondPayload.CueID != next.ID {
		t.Fatalf("second GO selected %s, want top-level Cue %s; child Cue must be skipped", secondPayload.CueID, next.ID)
	}
	state, err := s.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentCueID == nil || *state.CurrentCueID != next.ID {
		t.Fatalf("session current Cue=%v want %s", state.CurrentCueID, next.ID)
	}
}
