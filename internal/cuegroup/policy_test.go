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
	t.Run("disabled parent", func(t *testing.T) {
		cues := []domain.Cue{
			{ID: "cue-1", Name: "Cue 1", Enabled: false, ExecutionPolicy: linkedPolicy("cue-2")},
			{ID: "cue-2", Name: "Cue 2", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`)},
		}
		err := Validate(cues)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "disabled") {
			t.Fatalf("disabled parent group not rejected: %v", err)
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

func TestCueStartDelayPolicyRange(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw string
		want int64
		ok bool
	}{
		{"unset", `{}`, 0, true},
		{"immediate", `{"start_delay_ms":0}`, 0, true},
		{"one and a half seconds", `{"start_delay_ms":1500}`, 1500, true},
		{"maximum", `{"start_delay_ms":600000}`, 600000, true},
		{"negative", `{"start_delay_ms":-1}`, 0, false},
		{"too long", `{"start_delay_ms":600001}`, 0, false},
		{"fraction", `{"start_delay_ms":1.5}`, 0, false},
		{"string", `{"start_delay_ms":"100"}`, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy, err := Parse(json.RawMessage(tc.raw))
			if (err == nil) != tc.ok {
				t.Fatalf("Parse(%s) err=%v, want ok=%v", tc.raw, err, tc.ok)
			}
			if tc.ok && policy.StartDelayMS != tc.want {
				t.Fatalf("delay=%d, want %d", policy.StartDelayMS, tc.want)
			}
		})
	}
}
