package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSimulationCheckpointCaptureAndRestoreRejectRevokedGrantAtCommit(t *testing.T) {
	ctx := context.Background()
	st, _ := newStore(t)
	project, published, _ := createSessionFoundationFixture(t, st)
	session, err := st.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: published.ID, SessionType: domain.SessionSimulation, Name: "checkpoint ACL",
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartBeginning},
	})
	if err != nil { t.Fatal(err) }
	revoked := errors.New("rehearsal control revoked at checkpoint commit")
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error { return revoked })
	if _, err := st.CreateSimulationCheckpoint(guarded, session.ID, 1, json.RawMessage(`{}`)); !errors.Is(err, revoked) {
		t.Fatalf("revoked capture returned %v, want denied commit", err)
	}
	list, err := st.ListSimulationCheckpoints(ctx, project.ID, published.ID, 50)
	if err != nil || len(list) != 0 {
		t.Fatalf("checkpoint committed despite revocation: %d, err=%v", len(list), err)
	}
	checkpoint, err := st.CreateSimulationCheckpoint(ctx, session.ID, 1, json.RawMessage(`{}`))
	if err != nil { t.Fatal(err) }
	if err := st.MarkSimulationCheckpointRestored(guarded, session.ID, checkpoint.ID); !errors.Is(err, revoked) {
		t.Fatalf("revoked restore returned %v, want denied commit", err)
	}
	current, err := st.GetSessionFoundation(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	if current.StateTruth.VerifiedStateRef != nil || current.StateTruth.DesiredStateRef != nil {
		t.Fatalf("revoked restore persisted session truth: %+v", current.StateTruth)
	}
}
