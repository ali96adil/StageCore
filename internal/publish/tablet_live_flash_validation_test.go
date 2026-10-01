package publish

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestValidateAllowsTabletLiveURLWithSelectiveFlash(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	st := store.New(h.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet Live Flash Publish"})
	if err != nil {
		t.Fatal(err)
	}

	alias, err := st.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:   project.ID,
		LogicalName: "tablet.tablet-01.live-show",
		LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef:   "tablet-01",
		ProjectConfig: json.RawMessage(
			`{"device_id":"tablet-01","capability_key":"tablet.media.live.show"}`,
		),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:      revision.ID,
		DisplayLabel:    "1",
		Name:            "Camera Live + Flash",
		OrderIndex:      1,
		CueType:         "TABLET_SCENE",
		Criticality:     "NORMAL",
		Enabled:         true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex:     0,
		ExecutionMode:  "PARALLEL_BARRIER",
		TargetRef:      alias.LogicalName,
		CapabilityKey:  deviceexperience.CapabilityTabletLiveShow,
		Parameters:     json.RawMessage(`{"url":"http://192.168.3.135:9081/api/v0/stream?flash=1"}`),
		TimeoutPolicy:  json.RawMessage(`{}`),
		ErrorPolicy:    json.RawMessage(`{}`),
		PriorityClass:  domain.PriorityP1,
		Enabled:        true,
	}}); err != nil {
		t.Fatal(err)
	}

	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(devicechannel.StageDeviceLogicalType, capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{}
	})); err != nil {
		t.Fatal(err)
	}

	report, err := New(st, registry).Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Fatalf("tablet Live URL with flash=1 must remain publishable: %#v", report)
	}
}
