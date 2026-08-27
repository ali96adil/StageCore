package bulk_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestActiveShowPausesBulkWhileP1CueStillCompletes(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "SHOW bulk isolation"})
	if err != nil {
		t.Fatal(err)
	}
	parameters := json.RawMessage(`{"simulation":{"behavior":"COMPLETE","delay_ms":0}}`)
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "P1 GO", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		Parameters: parameters, TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`), PriorityClass: domain.PriorityP1, Enabled: true,
	}})
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

	manager := bulk.New(func(ctx context.Context) (bulk.Mode, error) {
		mode, err := s.ActiveOperationalSessionType(ctx)
		if err != nil {
			return "", err
		}
		if mode == domain.SessionShow {
			return bulk.ModeShow, nil
		}
		if mode == domain.SessionRehearsal {
			return bulk.ModeRehearsal, nil
		}
		return bulk.ModeEdit, nil
	})
	jobID, err := manager.Begin(ctx, bulk.KindMediaSync, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "Reference SHOW")
	if err != nil {
		t.Fatal(err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- manager.WaitAllowed(context.Background(), jobID) }()
	deadline := time.Now().Add(time.Second)
	for {
		job, _ := manager.Job(jobID)
		if job.State == bulk.Paused {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bulk job did not pause in SHOW: %#v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}

	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{CueID: cue.ID})
	result := cueengine.New(s).ExecuteCueGo(ctx, show.ID, contracts.CommandEnvelope{
		CommandID: commandID, CommandType: cueengine.CueGoCommandType, SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(), ProjectID: runtimeSnapshot.ProjectID, RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer: "test.operator", Priority: "P1", Payload: payload,
	})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("P1 Cue did not complete while bulk was paused: %#v", result)
	}
	job, _ := manager.Job(jobID)
	if job.State != bulk.Paused {
		t.Fatalf("bulk job escaped SHOW while Cue ran: %#v", job)
	}

	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("bulk job did not resume after SHOW ended")
	}
	job, _ = manager.Job(jobID)
	if job.State != bulk.Running {
		t.Fatalf("bulk job after SHOW=%#v", job)
	}
}
