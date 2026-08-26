package snapshot_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBuilderRequiresValidatedRevisionAndCreatesStableContentHash(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Fixed{Time: time.Date(2026, 8, 26, 14, 0, 0, 0, time.UTC)})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Snapshot Show"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, Name: "Cue 1", OrderIndex: 0, Enabled: true,
		ExecutionPolicy: json.RawMessage(`{"z":1,"a":2}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		Parameters: json.RawMessage(`{"simulation":{"delay_ms":0,"behavior":"COMPLETE"},"b":2,"a":1}`),
		TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	builder := snapshot.NewBuilder(s)
	if _, _, err := builder.Create(ctx, revision.ID, "test"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("draft snapshot err=%v want conflict", err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	first, firstManifest, err := builder.Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	second, secondManifest, err := builder.Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentHash != second.ContentHash {
		t.Fatalf("content hash drift: %s != %s", first.ContentHash, second.ContentHash)
	}
	if first.SnapshotVersion != 1 || second.SnapshotVersion != 2 {
		t.Fatalf("snapshot versions %d %d", first.SnapshotVersion, second.SnapshotVersion)
	}
	if len(firstManifest.Cues) != 1 || len(firstManifest.Cues[0].Actions) != 1 {
		t.Fatalf("unexpected manifest: %#v", firstManifest)
	}
	if firstManifest.RevisionID != secondManifest.RevisionID {
		t.Fatalf("manifest revision mismatch")
	}
	if _, err := h.DB.ExecContext(ctx, `UPDATE runtime_snapshots SET manifest_json = '{}' WHERE runtime_snapshot_id = ?`, first.ID); err == nil {
		t.Fatal("expected immutable runtime snapshot content update to fail")
	}
}
