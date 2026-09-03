package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func createInternalRestartFixture(t *testing.T, s *store.Store, kind string) (domain.RuntimeSnapshot, domain.Cue) {
	t.Helper()
	ctx := context.Background()
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Internal restart continuity"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:     project.ID,
		LogicalName:   "TIMECODE-INTERNAL",
		LogicalType:   "TIMECODE_SOURCE",
		TargetRef:     "TIMECODE-INTERNAL",
		ProjectConfig: json.RawMessage(`{"source_id":"restart-clock","kind":"` + kind + `","rate":"25","offset_frames":0,"start_timecode":"00:00:00:00"}`),
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Restart Cue",
		OrderIndex: 0, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{"timecode":{"binding_id":"restart-cue-1","at":"00:00:30:00","expiry_frames":25,"enabled":true}}`),
	}, nil)
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
	return runtimeSnapshot, cue
}

func TestHubRestartPreservesCleanInternalTimecodeRehearsal(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "restart-continuous")
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
	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.SessionActive || loaded.LifecycleState != domain.SessionLifecycleActive || loaded.EndedAt != nil {
		t.Fatalf("preserved INTERNAL REHEARSAL=%+v", loaded)
	}
}

func TestHubRestartKeepsExternalRehearsalFailClosed(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "MTC")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "external")
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
	loaded, err := s.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.SessionAborted || loaded.EndedAt == nil {
		t.Fatalf("external REHEARSAL after restart=%+v", loaded)
	}
}

func TestHubRestartNeverAutoPreservesShow(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "show")
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
	loaded, err := s.GetSession(ctx, show.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.SessionAborted || loaded.EndedAt == nil {
		t.Fatalf("SHOW after restart=%+v", loaded)
	}
}

func TestHubRestartDoesNotPreserveInternalRehearsalWithInFlightCue(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, cue := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "in-flight")
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "restart-running", "test")
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
	loaded, err := s.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.SessionAborted {
		t.Fatalf("in-flight INTERNAL REHEARSAL status=%s want ABORTED", loaded.Status)
	}
	executions, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 || executions[0].ID != execution.ID || executions[0].Result != domain.ExecutionCancelled {
		t.Fatalf("in-flight cue reconciliation=%+v", executions)
	}
}
