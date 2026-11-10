package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSimulationStopEventAndLifecycleCommitAtomicallyAfterAuthorization(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	project, snapshot, _ := createSessionFoundationFixture(t, st)
	session, err := st.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: snapshot.ID, SessionType: domain.SessionSimulation,
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartBeginning},
	})
	if err != nil { t.Fatal(err) }
	event := contracts.EventEnvelope{
		EventType: "simulation.stopped", SchemaVersion: contracts.SchemaVersion1,
		Source: "hub.simulation_control", ProjectID: project.ID,
		RuntimeSnapshotID: snapshot.ID, CorrelationID: "stop-request",
		CausationID: "stop-request", Priority: "P1",
		TraceContext: json.RawMessage(`{}`), Payload: json.RawMessage(`{"session_id":"test"}`),
	}
	denied := errors.New("project grant revoked before STOP commit")
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error { return denied })
	if err := st.StopSimulationWithEvent(guarded, session, event); !errors.Is(err, denied) {
		t.Fatalf("revoked simulation STOP returned %v, want exact revocation", err)
	}
	unchanged, err := st.GetSessionFoundation(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if unchanged.Status != domain.SessionActive || unchanged.LifecycleState != domain.SessionLifecycleActive {
		t.Fatalf("revoked STOP changed Session lifecycle: %+v", unchanged)
	}
	before, err := st.ListEvents(ctx, session.ID)
	if err != nil || len(before) != 0 {
		t.Fatalf("revoked STOP saved event: count=%d err=%v", len(before), err)
	}
	if err := st.StopSimulationWithEvent(ctx, session, event); err != nil { t.Fatal(err) }
	stopped, err := st.GetSessionFoundation(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if stopped.Status != domain.SessionCompleted || stopped.LifecycleState != domain.SessionLifecycleStopped {
		t.Fatalf("authorized STOP not completed: %+v", stopped)
	}
	events, err := st.ListEvents(ctx, session.ID)
	if err != nil || len(events) != 1 || events[0].EventType != "simulation.stopped" ||
		events[0].CausationID != "stop-request" {
		t.Fatalf("authorized STOP missing single matching event: %+v err=%v", events, err)
	}
	if err := st.StopSimulationWithEvent(ctx, session, event); err == nil {
		t.Fatal("replayed STOP mutated a terminal Session")
	}
}
