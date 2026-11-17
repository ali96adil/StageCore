package simulationcontrol_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestDigitalTwinFaultResetAndTargetMutationsRollbackAfterRevokedEventWrite(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil { t.Fatal(err) }
	defer h.Close()
	st := store.New(h.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Twin in-flight grant"})
	if err != nil { t.Fatal(err) }
	if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	published, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, "test")
	if err != nil { t.Fatal(err) }
	session, err := st.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: published.ID, SessionType: domain.SessionSimulation,
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartBeginning},
	})
	if err != nil { t.Fatal(err) }
	if session.ProjectID != project.ID { t.Fatal("test session has wrong project") }
	twin := simulator.NewDigitalTwinWithStateStore(st)
	if err := twin.BindSession(session); err != nil { t.Fatal(err) }
	if err := twin.ConfigureFaultContext(ctx, session.ID, simulator.FaultScenario{TargetRef: "video.1", Behavior: "FAIL"}); err != nil { t.Fatal(err) }
	if err := twin.SetTargetOnlineContext(ctx, session.ID, "video.1", false); err != nil { t.Fatal(err) }
	baseline := twin.Snapshot(session.ID)
	before, err := st.ListEvents(ctx, session.ID)
	if err != nil { t.Fatal(err) }
	revoked := errors.New("project permission revoked inside event transaction")
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error { return revoked })
	cases := []struct{
		name string
		mutate func() error
	}{
		{"configure-fault", func() error { return twin.ConfigureFaultContext(guarded, session.ID, simulator.FaultScenario{TargetRef: "video.2", Behavior: "DELAY", DelayMS: 10}) }},
		{"clear-fault", func() error { return twin.ClearFaultContext(guarded, session.ID, "video.1", "") }},
		{"target-online", func() error { return twin.SetTargetOnlineContext(guarded, session.ID, "video.1", true) }},
		{"reset", func() error { return twin.ResetSessionContext(guarded, session.ID) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.mutate(); !errors.Is(err, revoked) { t.Fatalf("mutation returned %v; want revoked grant", err) }
			if got := twin.Snapshot(session.ID); !reflect.DeepEqual(got, baseline) { t.Fatalf("mutated Twin after revocation: got=%+v baseline=%+v", got, baseline) }
			after, err := st.ListEvents(ctx, session.ID)
			if err != nil { t.Fatal(err) }
			if len(after) != len(before) { t.Fatalf("revoked event committed: before=%d after=%d", len(before), len(after)) }
		})
	}
}
