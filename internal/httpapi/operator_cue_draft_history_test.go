package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestCueDraftHistoryHTTPIsEditorScopedAndNeverPublishes(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	firstEditor, err := h.auth.CreateUser(ctx, "cue-history-first", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	firstLogin, err := h.auth.Login(ctx, firstEditor.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	secondEditor, err := h.auth.CreateUser(ctx, "cue-history-second", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	secondLogin, err := h.auth.Login(ctx, secondEditor.Username, h.password, "127.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Cue history API", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := st.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Baseline", OrderIndex: 0,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, editor := range []string{firstEditor.ID, secondEditor.ID} {
		if err := access.SetPermissions(ctx, owner.Session.User.ID, editor, project.ID,
			[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.CueEdit}); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(
		WithOperatorCuePublish(h.auth, st, publish.New(st, capability.NewRegistry())),
		WithProjectRouteQualification(access),
	).Handler()
	path := "/api/v1/projects/" + project.ID + "/cue-draft-history"
	request := func(method, route, token, csrf string, body []byte, want int) *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, authenticatedMutationRequest(t, method, route, body, token, csrf))
		if res.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, route, res.Code, want, res.Body.String())
		}
		return res
	}
	readStatus := func(token string) (store.CueDraftHistoryStatus, *httptest.ResponseRecorder) {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, authenticatedReadRequest(http.MethodGet, path, token))
		if res.Code != http.StatusOK {
			t.Fatalf("GET history status=%d body=%s", res.Code, res.Body.String())
		}
		var status store.CueDraftHistoryStatus
		if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		return status, res
	}

	create := cueWriteRequest{
		DisplayLabel: "2", Name: "Mixed Link Candidate", OrderIndex: 1, CueType: "STANDARD",
		Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{"linked_cue_ids":["` + baseline.ID + `"]}`),
		Actions: []actionWriteRequest{{
			OrderIndex: 0, ExecutionMode: "PARALLEL", TargetRef: "MAC", CapabilityKey: "osc.send",
			Parameters: json.RawMessage(`{"address":"/scene/cue"}`), TimeoutPolicy: json.RawMessage(`{"start_delay_ms":90}`),
			ErrorPolicy: json.RawMessage(`{"mode":"FAIL_SOFT"}`), PriorityClass: "P1", Enabled: true,
		}},
	}
	createJSON, _ := json.Marshal(create)
	created := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/cues", firstLogin.Token, firstLogin.CSRFToken, createJSON, http.StatusCreated)
	var createdBody struct {
		Cue cueView `json:"cue"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatal(err)
	}
	if createdBody.Cue.ID == "" || len(createdBody.Cue.Actions) != 1 {
		t.Fatalf("created Cue response=%+v", createdBody.Cue)
	}
	firstStatus, firstStatusResponse := readStatus(firstLogin.Token)
	if firstStatusResponse.Header().Get("Cache-Control") != "no-store" || !firstStatus.CanUndo || firstStatus.CanRedo {
		t.Fatalf("first editor history status=%+v headers=%v", firstStatus, firstStatusResponse.Header())
	}
	secondStatus, _ := readStatus(secondLogin.Token)
	if secondStatus.CanUndo || secondStatus.CanRedo {
		t.Fatalf("history leaked across editors: %+v", secondStatus)
	}
	request(http.MethodPost, path, secondLogin.Token, secondLogin.CSRFToken, []byte(`{"direction":"UNDO"}`), http.StatusNotFound)

	undone, _ := json.Marshal(cueDraftHistoryRequest{Direction: "UNDO"})
	undoResult := request(http.MethodPost, path, firstLogin.Token, firstLogin.CSRFToken, undone, http.StatusOK)
	var afterUndo store.CueDraftHistoryStatus
	if err := json.Unmarshal(undoResult.Body.Bytes(), &afterUndo); err != nil {
		t.Fatal(err)
	}
	if !afterUndo.CanRedo || afterUndo.CanUndo {
		t.Fatalf("history state after Undo=%+v", afterUndo)
	}
	projectAfterUndo, err := st.GetProject(ctx, project.ID)
	if err != nil || projectAfterUndo.CurrentRevisionID != revision.ID {
		t.Fatalf("Undo changed current revision: %+v err=%v", projectAfterUndo, err)
	}
	cuesAfterUndo, err := st.ListCues(ctx, revision.ID)
	if err != nil || len(cuesAfterUndo) != 1 || cuesAfterUndo[0].ID != baseline.ID {
		t.Fatalf("Undo Cue state=%+v err=%v", cuesAfterUndo, err)
	}
	if snapshot, err := st.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID); err != nil || snapshot != nil {
		t.Fatalf("Draft history unexpectedly created a Published Snapshot: %+v err=%v", snapshot, err)
	}

	redoJSON, _ := json.Marshal(cueDraftHistoryRequest{Direction: "REDO"})
	request(http.MethodPost, path, firstLogin.Token, firstLogin.CSRFToken, redoJSON, http.StatusOK)
	cuesAfterRedo, err := st.ListCues(ctx, revision.ID)
	if err != nil || len(cuesAfterRedo) != 2 || cuesAfterRedo[1].ID != createdBody.Cue.ID ||
		len(cuesAfterRedo[1].Actions) != 1 || cuesAfterRedo[1].Actions[0].ID != createdBody.Cue.Actions[0].ID {
		t.Fatalf("Redo did not preserve created Mixed Cue/action IDs: cues=%+v err=%v", cuesAfterRedo, err)
	}
}
