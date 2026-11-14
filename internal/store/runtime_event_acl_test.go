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

func TestSimulationEventAppendRejectsRevocationInWriteTransaction(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	project, published, _ := createSessionFoundationFixture(t, st)
	session, err := st.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: published.ID, SessionType: domain.SessionSimulation,
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartBeginning},
	})
	if err != nil { t.Fatal(err) }
	initial, err := st.ListEvents(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	revoked := errors.New("rehearsal grant revoked during event insert")
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error {
		return revoked
	})
	_, err = st.AppendEvent(guarded, &session.ID, contracts.EventEnvelope{
		EventType: "simulation.twin.reset", SchemaVersion: contracts.SchemaVersion1,
		Source: "stagecore.simulator.digital_twin", ProjectID: project.ID,
		RuntimeSnapshotID: published.ID, Priority: "P2",
		TraceContext: json.RawMessage(`{}`), Payload: json.RawMessage(`{}`),
	})
	if !errors.Is(err, revoked) {
		t.Fatalf("revoked event append returned %v, want authorization error", err)
	}
	after, err := st.ListEvents(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if len(after) != len(initial) {
		t.Fatalf("revoked Digital Twin event committed: before=%d after=%d", len(initial), len(after))
	}
}
