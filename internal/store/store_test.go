package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

var fixedTime = time.Date(2026, 8, 26, 12, 0, 0, 123456000, time.UTC)

func newStore(t *testing.T) (*store.Store, *db.Handle) {
	t.Helper()
	h, err := db.Open(context.Background(), db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return store.New(h.DB, clock.Fixed{Time: fixedTime}), h
}

func TestProjectAndRevisionPersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Fixed{Time: fixedTime})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Demo Show", Description: "M0", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	h, err = db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s = store.New(h.DB, clock.Real{})
	loadedProject, err := s.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedRevision, err := s.GetRevision(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedProject.CurrentRevisionID != revision.ID || loadedRevision.ProjectID != project.ID {
		t.Fatalf("reopen mismatch: %#v %#v", loadedProject, loadedRevision)
	}
	if !loadedProject.CreatedAt.Equal(fixedTime) {
		t.Fatalf("created_at=%s want %s", loadedProject.CreatedAt, fixedTime)
	}
}

func TestCueActionPersistenceAndRollback(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Show"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:      revision.ID,
		DisplayLabel:    "1",
		Name:            "Intro",
		OrderIndex:      0,
		Enabled:         true,
		ExecutionPolicy: json.RawMessage(`{"mode":"normal"}`),
	}, []domain.Action{{
		OrderIndex:    0,
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: "osc.send",
		Parameters:    json.RawMessage(`{"address":"/go"}`),
		Enabled:       true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.ListCues(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || len(loaded[0].Actions) != 1 || loaded[0].ID != cue.ID {
		t.Fatalf("unexpected loaded cues: %#v", loaded)
	}

	_, err = s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID,
		Name:       "Rollback",
		OrderIndex: 1,
		Enabled:    true,
	}, []domain.Action{
		{OrderIndex: 0, TargetRef: "X", CapabilityKey: "osc.send", Enabled: true},
		{OrderIndex: 0, TargetRef: "X", CapabilityKey: "osc.send", Enabled: true},
	})
	if err == nil {
		t.Fatal("expected duplicate action order failure")
	}
	loaded, err = s.ListCues(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("partial cue survived rollback: %#v", loaded)
	}
}

func TestAliasInputOutputRoutePersistence(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Show"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "VIDEO-MAIN", LogicalType: "MACHINE_ROLE", TargetRef: "VIDEO-MAIN", ProjectConfig: json.RawMessage(`{"role":"VIDEO-MAIN"}`),
	}); err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID, Name: "TEST-GO", SourceRef: "test", EventType: "input.test", ValueSchema: json.RawMessage(`{"type":"object"}`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := s.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Video Go", TargetRef: "VIDEO-MAIN", CapabilityKey: "osc.send", ValueSchema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	if _, err := s.CreateRouteWithActions(ctx, domain.Route{
		RevisionID: revision.ID, Name: "Test Route", InputID: input.ID, PriorityClass: domain.PriorityP2, Enabled: true,
	}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputID, Parameters: json.RawMessage(`{"value":1}`)}}); err != nil {
		t.Fatal(err)
	}

	aliases, err := s.ListAliases(ctx, project.ID)
	if err != nil || len(aliases) != 1 {
		t.Fatalf("aliases=%#v err=%v", aliases, err)
	}
	inputs, err := s.ListInputs(ctx, revision.ID)
	if err != nil || len(inputs) != 1 {
		t.Fatalf("inputs=%#v err=%v", inputs, err)
	}
	outputs, err := s.ListOutputs(ctx, revision.ID)
	if err != nil || len(outputs) != 1 {
		t.Fatalf("outputs=%#v err=%v", outputs, err)
	}
	routes, err := s.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 1 || len(routes[0].Actions) != 1 {
		t.Fatalf("routes=%#v err=%v", routes, err)
	}
}

func TestFrozenRevisionRejectsDefinitionMutation(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Show"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "Blocked", EventType: "test", Enabled: true})
	if !errors.Is(err, domain.ErrRevisionFrozen) {
		t.Fatalf("err=%v want ErrRevisionFrozen", err)
	}
}

func TestForeignKeyRejectsUnknownProjectAlias(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:   "00000000-0000-7000-8000-000000000099",
		LogicalName: "ghost",
	})
	if err == nil {
		t.Fatal("expected foreign-key failure")
	}
}

func TestBackupPreservesRepresentativeDomainRows(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Fixed{Time: fixedTime})
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Backup Show"})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "verified.sqlite3")
	if err := db.Backup(ctx, h.DB, backup); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	copyRoot := t.TempDir()
	copyPath := filepath.Join(copyRoot, "db", db.FileName)
	if err := copyFile(backup, copyPath); err != nil {
		t.Fatal(err)
	}
	copyHandle, err := db.Open(ctx, db.Config{DataRoot: copyRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer copyHandle.Close()
	copyStore := store.New(copyHandle.DB, clock.Real{})
	loaded, err := copyStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Backup Show" {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o640)
}

func TestCreateProjectIsAtomicWhenInitialRevisionFails(t *testing.T) {
	ctx := context.Background()
	s, h := newStore(t)
	if _, err := h.DB.ExecContext(ctx, `
		CREATE TRIGGER fail_initial_revision
		BEFORE INSERT ON project_revisions
		BEGIN
			SELECT RAISE(ABORT, 'forced revision failure');
		END`); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Must Roll Back"})
	if err == nil {
		t.Fatal("expected forced create failure")
	}
	var count int
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE name = 'Must Roll Back'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial project survived failed aggregate: count=%d", count)
	}
}
