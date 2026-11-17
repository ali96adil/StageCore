package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestRuntimeJumpListUsesPublishedSnapshotNotNewDraft(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Published Jump List", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	publishedCue, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Published Cue", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Disabled Published Cue", OrderIndex: 2,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: false,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	publishedSnapshot, _, err := snapshot.NewBuilder(projectStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+project.ID+"/runtime", nil)
	view, err := buildRuntimeStatus(req, projectStore, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Cues) != 1 || view.Cues[0].ID != publishedCue.ID {
		t.Fatalf("published runtime cues=%+v, want only enabled published Cue %s", view.Cues, publishedCue.ID)
	}

	draft, err := projectStore.EnsureProjectDraft(ctx, project.ID, "owner", "edit after publish")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: draft.ID, DisplayLabel: "3", Name: "Draft Only Cue", OrderIndex: 3,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}

	viewAfterDraft, err := buildRuntimeStatus(req, projectStore, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(viewAfterDraft.Cues) != 1 || viewAfterDraft.Cues[0].ID != publishedCue.ID {
		t.Fatalf("runtime Jump list leaked Draft Cues: %+v", viewAfterDraft.Cues)
	}

	session, err := projectStore.CreateSessionWithPurpose(ctx, publishedSnapshot.ID, domain.SessionRehearsal, "Overlapping Cue executions", "")
	if err != nil {
		t.Fatal(err)
	}
	finished, err := projectStore.CreateCueExecution(ctx, session.ID, publishedCue.ID, "finished-correlation", "GO")
	if err != nil {
		t.Fatal(err)
	}
	if err := projectStore.FinishCueExecution(ctx, finished.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}
	active, err := projectStore.CreateCueExecution(ctx, session.ID, publishedCue.ID, "active-correlation", "GO")
	if err != nil {
		t.Fatal(err)
	}
	viewWithActive, err := buildRuntimeStatus(req, projectStore, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if viewWithActive.ActiveExecutionCount != 1 || len(viewWithActive.ActiveExecutions) != 1 ||
		viewWithActive.ActiveExecutions[0].ID != active.ID || viewWithActive.ActiveExecutions[0].Result != domain.ExecutionRunning {
		t.Fatalf("runtime did not expose only active Cue executions: %+v", viewWithActive)
	}
	if viewWithActive.LatestExecution == nil || viewWithActive.LatestExecution.ID != active.ID {
		t.Fatalf("runtime lost latest Cue result while exposing active executions: %+v", viewWithActive.LatestExecution)
	}
}
