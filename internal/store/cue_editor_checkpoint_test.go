package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueEditorCheckpointsAreBoundedScopedAndDraftOnly(t *testing.T) {
	ctx := context.Background()
	s, h := newStore(t)
	if _, err := h.DB.ExecContext(ctx, `INSERT INTO local_users
		(user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('owner', 'checkpoint-owner', 'test-hash', 'OWNER', 1, ?, ?)`, fixedTime.UnixMicro(), fixedTime.UnixMicro()); err != nil {
		t.Fatal(err)
	}
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Cue checkpoint show", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "7A", Name: "Compound intro", OrderIndex: 1,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{"linked_cue_ids":["child"]}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	document, err := json.Marshal(map[string]any{
		"version": 1, "cue_id": cue.ID, "base": `{}`, "draft": map[string]any{
			"fields": []string{"7A", "Compound intro"}, "links": []string{"child"}, "actions": []any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		checkpoint, err := s.CreateCueEditorCheckpoint(ctx, project.ID, "owner", cue.ID,
			fmt.Sprintf("Rehearsal take %02d", i), document)
		if err != nil {
			t.Fatalf("create checkpoint %d: %v", i, err)
		}
		if checkpoint.ID == "" || checkpoint.CueID != cue.ID || checkpoint.CreatedAt == "" {
			t.Fatalf("incomplete checkpoint metadata: %+v", checkpoint)
		}
	}
	checkpoints, err := s.ListCueEditorCheckpoints(ctx, project.ID, "owner", cue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 20 || checkpoints[0].Name != "Rehearsal take 24" || checkpoints[19].Name != "Rehearsal take 05" {
		t.Fatalf("retained checkpoints=%d first=%q last=%q; want newest 24 through 05",
			len(checkpoints), checkpoints[0].Name, checkpoints[len(checkpoints)-1].Name)
	}
	if len(checkpoints[0].Document) == 0 || string(checkpoints[0].Document) != string(document) {
		t.Fatalf("checkpoint document was not preserved: %s", checkpoints[0].Document)
	}
	if other, err := s.ListCueEditorCheckpoints(ctx, project.ID, "other-user", cue.ID); err != nil || len(other) != 0 {
		t.Fatalf("checkpoint list leaked across users: count=%d err=%v", len(other), err)
	}
	if other, err := s.ListCueEditorCheckpoints(ctx, "other-project", "owner", cue.ID); err != nil || len(other) != 0 {
		t.Fatalf("checkpoint list leaked across Projects: count=%d err=%v", len(other), err)
	}
	if err := s.DeleteCueEditorCheckpoint(ctx, project.ID, "other-user", cue.ID, checkpoints[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-user delete err=%v, want not found", err)
	}
	if err := s.DeleteCueEditorCheckpoint(ctx, project.ID, "owner", cue.ID, checkpoints[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCueEditorCheckpoint(ctx, project.ID, "owner", cue.ID, checkpoints[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete err=%v, want not found", err)
	}
	remaining, err := s.ListCueEditorCheckpoints(ctx, project.ID, "owner", cue.ID)
	if err != nil || len(remaining) != 19 {
		t.Fatalf("remaining checkpoints=%d err=%v, want 19", len(remaining), err)
	}
	unchanged, err := s.GetCue(ctx, cue.ID)
	if err != nil || unchanged.Name != cue.Name || string(unchanged.ExecutionPolicy) != string(cue.ExecutionPolicy) {
		t.Fatalf("checkpoint operation changed authored Cue: cue=%+v err=%v", unchanged, err)
	}
	if _, err := h.DB.ExecContext(ctx, `DELETE FROM cues WHERE cue_id = ?`, cue.ID); err != nil {
		t.Fatal(err)
	}
	deletedCueCheckpoints, err := s.ListCueEditorCheckpoints(ctx, project.ID, "owner", cue.ID)
	if err != nil || len(deletedCueCheckpoints) != 0 {
		t.Fatalf("deleted Cue left stale checkpoints: checkpoints=%+v err=%v", deletedCueCheckpoints, err)
	}
}

func TestCreateCueEditorCheckpointRejectsInvalidScopeAndContent(t *testing.T) {
	ctx := context.Background()
	s, h := newStore(t)
	if _, err := h.DB.ExecContext(ctx, `INSERT INTO local_users
		(user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('owner', 'checkpoint-validation-owner', 'test-hash', 'OWNER', 1, ?, ?)`, fixedTime.UnixMicro(), fixedTime.UnixMicro()); err != nil {
		t.Fatal(err)
	}
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Cue checkpoint validation", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := json.Marshal(map[string]any{"version": 1, "cue_id": cue.ID, "draft": map[string]any{}})
	cases := []struct {
		label string
		name  string
		cue   string
		doc   []byte
	}{
		{label: "blank name", name: " ", cue: cue.ID, doc: document},
		{label: "overlong name", name: strings.Repeat("界", 65), cue: cue.ID, doc: document},
		{label: "control in name", name: "bad\nname", cue: cue.ID, doc: document},
		{label: "different Cue document", name: "Checkpoint", cue: cue.ID, doc: []byte(`{"version":1,"cue_id":"other-cue","draft":{}}`)},
		{label: "invalid JSON", name: "Checkpoint", cue: cue.ID, doc: []byte(`{"draft":`)},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if _, err := s.CreateCueEditorCheckpoint(ctx, project.ID, "owner", tc.cue, tc.name, tc.doc); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("create invalid checkpoint err=%v, want invalid input", err)
			}
		})
	}
	missingCueDocument := []byte(`{"version":1,"cue_id":"missing-cue","draft":{}}`)
	if _, err := s.CreateCueEditorCheckpoint(ctx, project.ID, "owner", "missing-cue", "Checkpoint", missingCueDocument); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("checkpoint for missing Cue err=%v, want not found", err)
	}
}
