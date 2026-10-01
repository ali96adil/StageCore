package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestReplaceDraftCueRegeneratesStaleActionIDAfterRevisionFork(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Fork stale action", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:      revision.ID,
		DisplayLabel:    "1",
		Name:            "OSC Bench",
		OrderIndex:      0,
		Enabled:         true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex:     0,
		ExecutionMode:  "SEQUENTIAL",
		TargetRef:      "OSC-BENCH-V2",
		CapabilityKey:  "osc.send",
		Parameters:     json.RawMessage(`{"address":"/stagecore/qualification/test","arguments":[]}`),
		TimeoutPolicy:  json.RawMessage(`{}`),
		ErrorPolicy:    json.RawMessage(`{}`),
		PriorityClass:  domain.PriorityP1,
		Enabled:        true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Actions) != 1 {
		t.Fatalf("created actions=%d, want 1", len(created.Actions))
	}
	staleActionID := created.Actions[0].ID

	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	successor, err := s.EnsureProjectDraft(ctx, project.ID, "test", "edit after publish")
	if err != nil {
		t.Fatal(err)
	}
	cues, err := s.ListCues(ctx, successor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || len(cues[0].Actions) != 1 {
		t.Fatalf("successor cues=%#v", cues)
	}
	mapped := cues[0]
	mapped.Name = "OSC Bench Revised"
	incoming := mapped.Actions[0]
	incoming.ID = staleActionID // browser can still hold the published revision Action ID.
	incoming.TargetRef = "OSC-BENCH-OK"

	updated, err := s.ReplaceDraftCue(ctx, mapped, []domain.Action{incoming})
	if err != nil {
		t.Fatalf("replace successor Draft with stale published Action ID: %v", err)
	}
	if len(updated.Actions) != 1 {
		t.Fatalf("updated actions=%d, want 1", len(updated.Actions))
	}
	if updated.Actions[0].ID == staleActionID {
		t.Fatal("stale published Action ID was reused in successor Draft")
	}
	if updated.Actions[0].TargetRef != "OSC-BENCH-OK" {
		t.Fatalf("updated target=%q", updated.Actions[0].TargetRef)
	}

	source, err := s.GetCue(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Actions) != 1 || source.Actions[0].ID != staleActionID || source.Actions[0].TargetRef != "OSC-BENCH-V2" {
		t.Fatalf("published source mutated: %#v", source.Actions)
	}
}


func TestDuplicateDraftCueRegeneratesExplicitTimecodeBindingID(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Duplicate timecode cue", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:   revision.ID,
		DisplayLabel: "1",
		Name:         "Timed Cue",
		OrderIndex:   1,
		Enabled:      true,
		ExecutionPolicy: json.RawMessage(`{
			"hold":{"mode":"manual"},
			"timecode":{
				"binding_id":"explicit-binding",
				"at":"00:00:10:00",
				"expiry_frames":3,
				"enabled":true
			}
		}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	duplicated, err := s.DuplicateDraftCue(ctx, revision.ID, created.ID, "2", "Timed Cue Copy", 2)
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err := json.Unmarshal(duplicated.ExecutionPolicy, &policy); err != nil {
		t.Fatal(err)
	}
	if _, ok := policy["hold"]; !ok {
		t.Fatalf("non-timecode execution policy was not preserved: %s", duplicated.ExecutionPolicy)
	}
	timecodePolicy, ok := policy["timecode"].(map[string]any)
	if !ok {
		t.Fatalf("timecode policy missing after duplicate: %s", duplicated.ExecutionPolicy)
	}
	if _, cloned := timecodePolicy["binding_id"]; cloned {
		t.Fatalf("duplicate retained explicit timecode binding_id: %s", duplicated.ExecutionPolicy)
	}
	if timecodePolicy["at"] != "00:00:10:00" || timecodePolicy["expiry_frames"] != float64(3) || timecodePolicy["enabled"] != true {
		t.Fatalf("duplicate changed timecode schedule: %#v", timecodePolicy)
	}
}
