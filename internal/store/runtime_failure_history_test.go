package store_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestRecentActionFailuresRemainAfterSuccessfulCueAndHubReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil { t.Fatal(err) }
	s := store.New(h.DB, clock.Fixed{Time: fixedTime})

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Failure history"})
	if err != nil { t.Fatal(err) }
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil { t.Fatal(err) }
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, Name: "Visible failures", OrderIndex: 1, Enabled: true}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	published, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }
	session, err := s.CreateSession(ctx, published.ID, domain.SessionRehearsal, "failure history")
	if err != nil { t.Fatal(err) }

	first, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "failure-one", "OPERATOR")
	if err != nil { t.Fatal(err) }
	failedAction, err := s.CreateActionExecution(ctx, first.ID, cue.Actions[0].ID)
	if err != nil { t.Fatal(err) }
	code := "OUTPUT_OFFLINE"
	if err := s.FinishActionExecution(ctx, failedAction.ID, domain.ExecutionFailed, 42, "output unavailable", &code); err != nil { t.Fatal(err) }
	// CONTINUE means the Cue may be completed even though this output failed.
	if err := s.FinishCueExecution(ctx, first.ID, domain.ExecutionCompleted); err != nil { t.Fatal(err) }

	second, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "success-two", "OPERATOR")
	if err != nil { t.Fatal(err) }
	success, err := s.CreateActionExecution(ctx, second.ID, cue.Actions[0].ID)
	if err != nil { t.Fatal(err) }
	if err := s.FinishActionExecution(ctx, success.ID, domain.ExecutionCompleted, 3, "OK", nil); err != nil { t.Fatal(err) }
	if err := s.FinishCueExecution(ctx, second.ID, domain.ExecutionCompleted); err != nil { t.Fatal(err) }

	got, err := s.ListRecentActionFailures(ctx, session.ID, 8)
	if err != nil { t.Fatal(err) }
	if len(got) != 1 || got[0].ID != failedAction.ID || got[0].ErrorCode == nil || *got[0].ErrorCode != code {
		t.Fatalf("failure disappeared after successful Cue: %+v", got)
	}
	if _, err := s.ListRecentActionFailures(ctx, session.ID, 0); err == nil {
		t.Fatal("unbounded failure query accepted")
	}
	if err := h.Close(); err != nil { t.Fatal(err) }
	h, err = db.Open(ctx, db.Config{DataRoot: root})
	if err != nil { t.Fatal(err) }
	defer h.Close()
	s = store.New(h.DB, clock.Fixed{Time: fixedTime})
	reopened, err := s.ListRecentActionFailures(ctx, session.ID, 8)
	if err != nil { t.Fatal(err) }
	if len(reopened) != 1 || reopened[0].ID != failedAction.ID {
		t.Fatalf("persisted failure history missing after Hub reopen: %+v", reopened)
	}
}
