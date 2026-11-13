package cueengine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/snapshot"
)

func TestNestedCueIDsValidatesManifestGraph(t *testing.T) {
	tests := []struct {
		name       string
		cues       []snapshot.Cue
		wantNested map[string]bool
		wantError  string
	}{
		{
			name: "nested chain",
			cues: []snapshot.Cue{
				groupTestCue("root", true, "middle"),
				groupTestCue("middle", true, "leaf"),
				groupTestCue("leaf", true),
			},
			wantNested: map[string]bool{"middle": true, "leaf": true},
		},
		{
			name:      "missing child",
			cues:      []snapshot.Cue{groupTestCue("root", true, "missing")},
			wantError: "links to missing Cue missing",
		},
		{
			name: "disabled child",
			cues: []snapshot.Cue{
				groupTestCue("root", true, "child"),
				groupTestCue("child", false),
			},
			wantError: "links to disabled Cue child",
		},
		{
			name: "multiple parents",
			cues: []snapshot.Cue{
				groupTestCue("first", true, "child"),
				groupTestCue("second", true, "child"),
				groupTestCue("child", true),
			},
			wantError: "Cue child is linked by more than one parent Cue",
		},
		{
			name: "cycle",
			cues: []snapshot.Cue{
				groupTestCue("root", true, "child"),
				groupTestCue("child", true, "root"),
			},
			wantError: "linked Cue cycle detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nestedCueIDs(snapshot.Manifest{Cues: tt.cues})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("nestedCueIDs error=%v, want substring %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.wantNested) {
				t.Fatalf("nested Cue IDs=%v, want %v", got, tt.wantNested)
			}
			for id := range tt.wantNested {
				if !got[id] {
					t.Errorf("nested Cue %q missing from %v", id, got)
				}
			}
		})
	}
}

func groupTestCue(id string, enabled bool, linked ...string) snapshot.Cue {
	policy := map[string]any{"linked_cue_ids": linked}
	if len(linked) != 0 {
		policy["linked_cue_mode"] = "TOGETHER"
	}
	raw, _ := json.Marshal(policy)
	return snapshot.Cue{ID: id, Name: id, Enabled: enabled, ExecutionPolicy: raw}
}
