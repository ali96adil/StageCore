package simulationcontrol_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulationcontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSimulationStartRevokedAfterSessionCreationAbortsFreshSession(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil { t.Fatal(err) }
	defer handle.Close()
	st := store.New(handle.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Abort after ACL revocation"})
	if err != nil { t.Fatal(err) }
	if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	if _, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, "test"); err != nil { t.Fatal(err) }
	twin := simulator.NewDigitalTwin()
	engine := cueengine.NewWithExecutor(st, simulator.NewSessionExecutorWithDigitalTwin(st, simulator.Adapter{}, twin))
	control := simulationcontrol.New(st, engine, twin)
	revoked := errors.New("rehearsal grant revoked during simulation.started event commit")
	var checked int
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error {
		checked++
		// 1: command reservation, 2: newly created Session, 3: the
		// simulation.started event; revoke after Session creation.
		if checked >= 3 { return revoked }
		return nil
	})
	created, result := control.Start(guarded, simulationcontrol.StartRequest{
		ProjectID: project.ID, Issuer: "operator",
		RequestID: "00000000-0000-7000-8000-000000000945",
		StartKind: domain.SessionStartBeginning,
	})
	if checked < 3 || result.Status != contracts.CommandFailed || created.ID != "" {
		t.Fatalf("revocation not exercised: checks=%d created=%+v result=%+v", checked, created, result)
	}
	active, err := st.ActiveSessionForProject(ctx, project.ID)
	if err != nil || active != nil {
		t.Fatalf("revoked failed Start left active Session: active=%+v err=%v", active, err)
	}
	var lifecycle, status string
	if err := handle.DB.QueryRowContext(ctx, "SELECT lifecycle_state, status FROM sessions WHERE project_id = ?", project.ID).Scan(&lifecycle, &status); err != nil {
		t.Fatal(err)
	}
	if lifecycle != string(domain.SessionLifecycleAborted) || status != string(domain.SessionAborted) {
		t.Fatalf("failed Start did not compensate: lifecycle=%s status=%s", lifecycle, status)
	}
	// No event may be committed under the revoked grant. The aborted Session
	// must be visible only as audit/cleanup, not as running Simulation.
	var eventCount int
	if err := handle.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_records WHERE project_id = ?", project.ID).Scan(&eventCount); err != nil { t.Fatal(err) }
	if eventCount != 0 { t.Fatalf("revoked Simulation event committed: count=%d", eventCount) }
}
