package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSimulationStartConfirmationRejectsRevocationInsideTransaction(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	_, snapshot, cues := createSessionFoundationFixture(t, st)
	session, err := st.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: snapshot.ID, SessionType: domain.SessionSimulation, Name: "range approval",
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartCue, CueID: &cues[1].ID},
	})
	if err != nil { t.Fatal(err) }
	if !session.StateTruth.ManualConfirmationRequired { t.Fatalf("test requires unconfirmed state: %+v", session) }
	revoked := errors.New("simulation approval grant revoked")
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error { return revoked })
	if err := st.ConfirmSimulationStartState(guarded, session.ID); !errors.Is(err, revoked) {
		t.Fatalf("revoked start-state confirmation returned %v", err)
	}
	current, err := st.GetSessionFoundation(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if !current.StateTruth.ManualConfirmationRequired ||
		current.StateTruth.RestorationStatus != domain.SessionRestorationManualConfirmationRequired {
		t.Fatalf("revoked start-state confirmation persisted: %+v", current.StateTruth)
	}
}
