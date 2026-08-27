package backup_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/backup"
	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestStateBackupVerifyRestorePreservesProjectSnapshotCuesAndSessionHistory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, runtimeSnapshot, session := createKnownRuntimeHistory(t, ctx, s)
	manager := managerForStore(s)
	service, err := backup.New(h, s, root, manager)
	if err != nil {
		t.Fatal(err)
	}

	record, err := service.CreateStateBackup(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !record.Verified || record.Manifest.BackupID != record.ID {
		t.Fatalf("backup record=%#v", record)
	}
	manifest, err := service.Verify(record.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(manifest.ProjectIDs, project.ID) || !contains(manifest.RuntimeSnapshotIDs, runtimeSnapshot.ID) || !contains(manifest.SessionIDs, session.ID) {
		t.Fatalf("backup manifest missing known identities: %#v", manifest)
	}

	target := filepath.Join(t.TempDir(), "restored-data")
	restored, err := service.Restore(ctx, record.Path, target)
	if err != nil {
		t.Fatal(err)
	}
	if restored.DataRoot != target {
		t.Fatalf("restored root=%q want %q", restored.DataRoot, target)
	}

	restoredHandle, err := db.Open(ctx, db.Config{DataRoot: target})
	if err != nil {
		t.Fatal(err)
	}
	defer restoredHandle.Close()
	restoredStore := store.New(restoredHandle.DB, clock.Real{})
	loadedProject, err := restoredStore.GetProject(ctx, project.ID)
	if err != nil || loadedProject.CurrentRevisionID != revision.ID {
		t.Fatalf("restored project=%#v err=%v", loadedProject, err)
	}
	loadedSnapshot, err := restoredStore.GetRuntimeSnapshot(ctx, runtimeSnapshot.ID)
	if err != nil || loadedSnapshot.ContentHash != runtimeSnapshot.ContentHash {
		t.Fatalf("restored snapshot=%#v err=%v", loadedSnapshot, err)
	}
	loadedSession, err := restoredStore.GetSession(ctx, session.ID)
	if err != nil || loadedSession.Status != domain.SessionCompleted {
		t.Fatalf("restored session=%#v err=%v", loadedSession, err)
	}
	cues, err := restoredStore.ListCues(ctx, revision.ID)
	if err != nil || len(cues) != 1 || cues[0].Name != "Backup Cue" {
		t.Fatalf("restored cues=%#v err=%v", cues, err)
	}
	executions, err := restoredStore.ListCueExecutions(ctx, session.ID)
	if err != nil || len(executions) != 1 || executions[0].Result != domain.ExecutionCompleted {
		t.Fatalf("restored cue history=%#v err=%v", executions, err)
	}
}

func TestRestoreAndBackupAreBlockedDuringActiveShow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, _, runtimeSnapshot, _ := createKnownRuntimeHistory(t, ctx, s)
	manager := managerForStore(s)
	service, err := backup.New(h, s, root, manager)
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.CreateStateBackup(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "Active SHOW")
	if err != nil {
		t.Fatal(err)
	}
	defer s.EndSession(ctx, show.ID, domain.SessionAborted)

	target := filepath.Join(t.TempDir(), "must-not-exist")
	if _, err := service.Restore(ctx, record.Path, target); !errors.Is(err, backup.ErrActiveShow) {
		t.Fatalf("restore during SHOW error=%v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore during SHOW created target: %v", err)
	}
	if _, err := service.CreateStateBackup(ctx, t.TempDir()); !errors.Is(err, bulk.ErrShowBlocked) {
		t.Fatalf("backup during SHOW error=%v", err)
	}
}

func TestVerifyRejectsTamperedBackup(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	createKnownRuntimeHistory(t, ctx, s)
	service, err := backup.New(h, s, root, managerForStore(s))
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.CreateStateBackup(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(record.Path, "db", db.FileName)
	file, err := os.OpenFile(databasePath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("tamper")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(record.Path); !errors.Is(err, backup.ErrIntegrity) {
		t.Fatalf("tampered backup verify error=%v", err)
	}
}

func createKnownRuntimeHistory(t *testing.T, ctx context.Context, s *store.Store) (domain.Project, domain.ProjectRevision, domain.RuntimeSnapshot, domain.Session) {
	t.Helper()
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Backup Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Backup Cue", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`), TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`), PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Recorded rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{})
	result := cueengine.New(s).ExecuteCueGo(ctx, session.ID, contracts.CommandEnvelope{
		CommandID: commandID, CommandType: cueengine.CueGoCommandType, SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(), ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer: "test.operator", Priority: "P1", Payload: payload,
	})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("seed Cue execution=%#v", result)
	}
	if err := s.EndSession(ctx, session.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	return project, revision, runtimeSnapshot, session
}

func managerForStore(s *store.Store) *bulk.Manager {
	return bulk.New(func(ctx context.Context) (bulk.Mode, error) {
		mode, err := s.ActiveOperationalSessionType(ctx)
		if err != nil {
			return "", err
		}
		switch mode {
		case domain.SessionShow:
			return bulk.ModeShow, nil
		case domain.SessionRehearsal:
			return bulk.ModeRehearsal, nil
		default:
			return bulk.ModeEdit, nil
		}
	})
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
