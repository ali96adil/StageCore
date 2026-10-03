package cuegroup

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
)

func linkedPolicy(ids ...string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"linked_cue_ids": ids,
		"linked_cue_mode": "TOGETHER",
	})
	return raw
}

func TestValidateAllowsNestedSingleParentCueTree(t *testing.T) {
	cues := []domain.Cue{
		{ID: "cue-1", Name: "Cue 1", Enabled: true, ExecutionPolicy: linkedPolicy("cue-2", "cue-3")},
		{ID: "cue-2", Name: "Cue 2", Enabled: true, ExecutionPolicy: linkedPolicy("cue-4")},
		{ID: "cue-3", Name: "Cue 3", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`)},
		{ID: "cue-4", Name: "Cue 4", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`)},
	}
	if err := Validate(cues); err != nil {
		t.Fatalf("valid Cue tree rejected: %v", err)
	}
	parents, err := ParentMap(cues)
	if err != nil {
		t.Fatal(err)
	}
	if parents["cue-2"] != "cue-1" || parents["cue-3"] != "cue-1" || parents["cue-4"] != "cue-2" {
		t.Fatalf("parents=%v", parents)
	}
}

func TestValidateRejectsCueCyclesAndMultipleParents(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		cues := []domain.Cue{
			{ID: "cue-1", Name: "Cue 1", Enabled: true, ExecutionPolicy: linkedPolicy("cue-2")},
			{ID: "cue-2", Name: "Cue 2", Enabled: true, ExecutionPolicy: linkedPolicy("cue-1")},
		}
		err := Validate(cues)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cycle") {
			t.Fatalf("cycle not rejected: %v", err)
		}
	})
	t.Run("multiple parents", func(t *testing.T) {
		cues := []domain.Cue{
			{ID: "cue-1", Name: "Cue 1", Enabled: true, ExecutionPolicy: linkedPolicy("cue-3")},
			{ID: "cue-2", Name: "Cue 2", Enabled: true, ExecutionPolicy: linkedPolicy("cue-3")},
			{ID: "cue-3", Name: "Cue 3", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`)},
		}
		err := Validate(cues)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "more than one parent") {
			t.Fatalf("multiple parents not rejected: %v", err)
		}
	})
}
