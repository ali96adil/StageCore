package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/cuegroup"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestEnsureProjectDraftRemapsLinkedCueIDs(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Cue Group Fork", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Child", OrderIndex: 2,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := json.Marshal(map[string]any{
		"linked_cue_ids": []string{child.ID},
		"linked_cue_mode": "TOGETHER",
		"timecode": map[string]any{"enabled": false},
	})
	parent, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Parent", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: policy,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	draft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "edit grouped cue")
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID == revision.ID || draft.Status != domain.RevisionDraft {
		t.Fatalf("draft=%+v", draft)
	}
	cues, err := s.ListCues(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("forked cues=%+v", cues)
	}
	var forkedParent, forkedChild domain.Cue
	for _, cue := range cues {
		switch cue.Name {
		case "Parent":
			forkedParent = cue
		case "Child":
			forkedChild = cue
		}
	}
	if forkedParent.ID == "" || forkedChild.ID == "" {
		t.Fatalf("forked parent/child missing: %+v", cues)
	}
	if forkedParent.ID == parent.ID || forkedChild.ID == child.ID {
		t.Fatalf("fork reused source Cue IDs: parent=%s/%s child=%s/%s", parent.ID, forkedParent.ID, child.ID, forkedChild.ID)
	}
	var forkedPolicy struct {
		LinkedCueIDs []string       `json:"linked_cue_ids"`
		LinkedCueMode string         `json:"linked_cue_mode"`
		Timecode      map[string]any `json:"timecode"`
	}
	if err := json.Unmarshal(forkedParent.ExecutionPolicy, &forkedPolicy); err != nil {
		t.Fatal(err)
	}
	if len(forkedPolicy.LinkedCueIDs) != 1 || forkedPolicy.LinkedCueIDs[0] != forkedChild.ID {
		t.Fatalf("forked linked Cue IDs=%v want child=%s", forkedPolicy.LinkedCueIDs, forkedChild.ID)
	}
	if forkedPolicy.LinkedCueMode != "TOGETHER" || forkedPolicy.Timecode == nil {
		t.Fatalf("fork lost Cue policy fields: %+v", forkedPolicy)
	}
	if err := cuegroup.Validate(cues); err != nil {
		t.Fatalf("forked Cue Group is invalid: %v", err)
	}
}
