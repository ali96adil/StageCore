package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

func TestRuntimeCueSchedulingPauseResumeAPIBlocksOnlyNewGOAndJUMP(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	cue, err := f.projects.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: project.CurrentRevisionID, DisplayLabel: "1", Name: "Temporary Acceptance Cue",
		OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(f.projects, capability.NewRegistry())
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access)).Handler()
	startBody, _ := json.Marshal(runtimeStartRequest{Mode: "REHEARSAL", Name: "Temporary Phase 2 acceptance", RequestID: "00000000-0000-7000-8000-000000009901"})
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/runtime/start", startBody, f.owner.Token, f.owner.CSRFToken))
	if start.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	var started struct {
		Session sessionView             `json:"session"`
		Result  contracts.CommandResult `json:"result"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil || started.Session.ID == "" || started.Result.Status != contracts.CommandCompleted {
		t.Fatalf("start response=%+v err=%v body=%s", started, err, start.Body.String())
	}

	post := func(path, requestID string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(runtimeCommandRequest{ExpectedSessionID: &started.Session.ID, RequestID: requestID})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, authenticatedMutationRequest(t, http.MethodPost,
			"/api/v1/projects/"+f.projectID+"/runtime/"+path, body, f.owner.Token, f.owner.CSRFToken))
		return response
	}
	paused := post("pause-cue-scheduling", "00000000-0000-7000-8000-000000009902")
	if paused.Code != http.StatusOK || !containsJSON(paused.Body.Bytes(), `"cue_scheduling_paused":true`) ||
		!containsJSON(paused.Body.Bytes(), `"scope":"NEW_GO_AND_JUMP_ADMISSION_ONLY"`) {
		t.Fatalf("pause response=%d %s", paused.Code, paused.Body.String())
	}
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+f.projectID+"/runtime", f.owner.Token))
	if status.Code != http.StatusOK || !containsJSON(status.Body.Bytes(), `"cue_scheduling_paused":true`) {
		t.Fatalf("paused Runtime status=%d %s", status.Code, status.Body.String())
	}
	goPaused := post("go", "00000000-0000-7000-8000-000000009903")
	jumpPayload, _ := json.Marshal(runtimeJumpRequest{
		ExpectedSessionID: &started.Session.ID, RequestID: "00000000-0000-7000-8000-000000009904",
		CueID: cue.ID, Confirm: true,
	})
	jumpPaused := httptest.NewRecorder()
	handler.ServeHTTP(jumpPaused, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/runtime/jump", jumpPayload, f.owner.Token, f.owner.CSRFToken))
	for _, response := range []*httptest.ResponseRecorder{goPaused, jumpPaused} {
		if response.Code != http.StatusConflict || !containsJSON(response.Body.Bytes(), `"error_code":"CUE_SCHEDULING_PAUSED"`) {
			t.Fatalf("scheduled Cue admitted during pause: HTTP %d %s", response.Code, response.Body.String())
		}
	}
	resumed := post("resume-cue-scheduling", "00000000-0000-7000-8000-000000009905")
	if resumed.Code != http.StatusOK || !containsJSON(resumed.Body.Bytes(), `"cue_scheduling_paused":false`) {
		t.Fatalf("resume response=%d %s", resumed.Code, resumed.Body.String())
	}
	goAfterResume := post("go", "00000000-0000-7000-8000-000000009906")
	if goAfterResume.Code != http.StatusOK || (!containsJSON(goAfterResume.Body.Bytes(), `"status":"COMPLETED"`) && !containsJSON(goAfterResume.Body.Bytes(), `"status":"ACCEPTED"`)) {
		t.Fatalf("GO after Resume=%d %s", goAfterResume.Code, goAfterResume.Body.String())
	}
}

func containsJSON(value []byte, fragment string) bool {
	return bytes.Contains(value, []byte(fragment))
}
