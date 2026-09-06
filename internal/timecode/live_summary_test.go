package timecode

import (
	"context"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestLiveSummaryUsesActiveSessionStateForSharedSnapshot(t *testing.T) {
	ctx := context.Background()
	stageStore, _ := newIntegrationStore(t)
	project, runtimeSnapshot := createTimecodeSnapshot(t, ctx, stageStore, SourceMTC, "30", "mtc-main")
	runtime := NewRuntimeService(stageStore, nil)

	oldSession, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "old rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	oldObservedAt := time.Now().UTC()
	if _, err := runtime.IngestFrame(ctx, oldSession.ID, "mtc-main", SourceMTC, Rate30, 10, oldObservedAt, 0, false); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.EndSessionLifecycle(ctx, oldSession.ID, domain.SessionLifecycleCompleted, "test completed"); err != nil {
		t.Fatal(err)
	}

	showSession, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "active show")
	if err != nil {
		t.Fatal(err)
	}

	beforeSample, err := runtime.LiveSummary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !beforeSample.ShowLocked {
		t.Fatal("active SHOW must report the immutable timecode source as locked before its first sample")
	}
	if beforeSample.Health.State != HealthMissing {
		t.Fatalf("active SHOW without a sample health=%s want %s", beforeSample.Health.State, HealthMissing)
	}
	if beforeSample.LastSample != nil {
		t.Fatalf("active SHOW leaked a completed session sample: %#v", beforeSample.LastSample)
	}

	showObservedAt := time.Now().UTC()
	if _, err := runtime.IngestFrame(ctx, showSession.ID, "mtc-main", SourceMTC, Rate30, 20, showObservedAt, 0, false); err != nil {
		t.Fatal(err)
	}
	live, err := runtime.LiveSummary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !live.ShowLocked {
		t.Fatal("active SHOW lost timecode source lock after observation")
	}
	if live.LastSample == nil {
		t.Fatal("active SHOW sample is missing from live summary")
	}
	if live.Health.LastFrame != 20 || live.Health.SourceID != "mtc-main" {
		t.Fatalf("live summary selected the wrong session state: %#v", live.Health)
	}

	if err := stageStore.EndSessionLifecycle(ctx, showSession.ID, domain.SessionLifecycleCompleted, "test completed"); err != nil {
		t.Fatal(err)
	}
	afterShow, err := runtime.LiveSummary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterShow.ShowLocked || afterShow.LastSample != nil || afterShow.Health.State != HealthMissing {
		t.Fatalf("completed sessions must not leak into live summary: %#v", afterShow)
	}
}
