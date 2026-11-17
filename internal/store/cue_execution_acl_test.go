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

func TestCueExecutionInsertRejectsRevokedSimulationGrant(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	_, snapshot, cues := createSessionFoundationFixture(t, st)
	session, err := st.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: snapshot.ID, SessionType: domain.SessionSimulation,
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartBeginning},
	})
	if err != nil { t.Fatal(err) }
	revoked := errors.New("simulation cue grant revoked")
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error { return revoked })
	_, err = st.CreateCueExecution(guarded, session.ID, cues[0].ID, "correlation", "operator")
	if !errors.Is(err, revoked) {
		t.Fatalf("Cue execution was admitted after grant loss: %v", err)
	}
	executions, err := st.ListCueExecutions(ctx, session.ID)
	if err != nil || len(executions) != 0 {
		t.Fatalf("revoked Cue persisted: %+v, err=%v", executions, err)
	}
}
