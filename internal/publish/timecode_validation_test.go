package publish

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestValidateTimecodeDraftUsesRuntimeDropFrameRules(t *testing.T) {
	tests := []struct {
		name       string
		rate       string
		start      string
		at         string
		wantErr    bool
	}{
		{name: "drop-frame delimiter mismatch", rate: "29.97 DF", start: "00:00:00;00", at: "00:00:05:00", wantErr: true},
		{name: "dropped frame number", rate: "29.97 DF", start: "00:00:00;00", at: "00:01:00;00", wantErr: true},
		{name: "first legal frame after drop", rate: "29.97 DF", start: "00:00:00;00", at: "00:01:00;02", wantErr: false},
		{name: "non-drop unchanged", rate: "25", start: "00:00:00:00", at: "00:01:00:00", wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aliases := []domain.ProjectDeviceAlias{{
				ID: "alias-1", LogicalName: "TIMECODE-MTC", LogicalType: "TIMECODE_SOURCE",
				ProjectConfig: json.RawMessage(`{"source_id":"phase3-mtc","kind":"MTC","rate":"` + tc.rate + `","offset_frames":0,"start_timecode":"` + tc.start + `"}`),
			}}
			cues := []domain.Cue{{
				ID: "cue-1", Enabled: true,
				ExecutionPolicy: json.RawMessage(`{"timecode":{"binding_id":"phase3-tc-cue-1","at":"` + tc.at + `","expiry_frames":25,"enabled":true}}`),
			}}
			err := validateTimecodeDraft(aliases, cues)
			if tc.wantErr && err == nil {
				t.Fatal("expected timecode validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected timecode validation error: %v", err)
			}
		})
	}
}

func TestValidateBlocksRuntimeInvalidDropFrameBinding(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Drop Frame Draft Validation"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID,
		LogicalName: "TIMECODE-MTC",
		LogicalType: "TIMECODE_SOURCE",
		TargetRef: "TIMECODE-MTC",
		ProjectConfig: json.RawMessage(`{"source_id":"phase3-mtc","kind":"MTC","rate":"29.97 DF","offset_frames":0,"start_timecode":"00:00:00;00"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID,
		DisplayLabel: "1",
		Name: "Invalid DF Cue",
		OrderIndex: 1,
		Criticality: "NORMAL",
		Enabled: true,
		ExecutionPolicy: json.RawMessage(`{"timecode":{"binding_id":"phase3-tc-cue-1","at":"00:01:00;00","expiry_frames":25,"enabled":true}}`),
	}, nil); err != nil {
		t.Fatal(err)
	}

	report, err := New(s, capability.NewRegistry()).Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasFinding(report, "TIMECODE_CONFIGURATION_INVALID") {
		t.Fatalf("runtime-invalid drop-frame binding must block publish: %#v", report)
	}
}
