package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestInputDraftHistoryHTTPIsEditorScopedAndNeverPublishes(t *testing.T) {
	ctx := context.Background()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	firstEditor, err := h.auth.CreateUser(ctx, "input-history-first", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	firstLogin, err := h.auth.Login(ctx, firstEditor.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	secondEditor, err := h.auth.CreateUser(ctx, "input-history-second", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	secondLogin, err := h.auth.Login(ctx, secondEditor.Username, h.password, "127.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.db.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Input Draft history API", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, editor := range []string{firstEditor.ID, secondEditor.ID} {
		if err := access.SetPermissions(ctx, owner.Session.User.ID, editor, project.ID,
			[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(WithOperatorConfiguration(h.auth, st), WithProjectRouteQualification(access)).Handler()
	path := "/api/v1/projects/" + project.ID + "/input-draft-history"
	request := func(method, route, token, csrf string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if method == http.MethodGet {
			req = authenticatedReadRequest(method, route, token)
		} else {
			var payload []byte
			if body != nil {
				payload, err = json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
			}
			req = authenticatedMutationRequest(t, method, route, payload, token, csrf)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, route, res.Code, want, res.Body.String())
		}
		return res
	}

	createdResponse := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/inputs", firstLogin.Token, firstLogin.CSRFToken, map[string]any{
		"name": "GO", "source_ref": "osc:/go", "event_type": "osc.message", "value_schema": map[string]any{}, "enabled": true,
	}, http.StatusCreated)
	var created struct {
		ID string `json:"input_id"`
	}
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("created Input=%+v err=%v", created, err)
	}
	firstStatusResponse := request(http.MethodGet, path, firstLogin.Token, "", nil, http.StatusOK)
	var firstStatus store.InputDraftHistoryStatus
	if err := json.Unmarshal(firstStatusResponse.Body.Bytes(), &firstStatus); err != nil ||
		firstStatusResponse.Header().Get("Cache-Control") != "no-store" || !firstStatus.CanUndo || firstStatus.CanRedo {
		t.Fatalf("first editor Input history=%+v headers=%v err=%v", firstStatus, firstStatusResponse.Header(), err)
	}
	secondStatusResponse := request(http.MethodGet, path, secondLogin.Token, "", nil, http.StatusOK)
	var secondStatus store.InputDraftHistoryStatus
	if err := json.Unmarshal(secondStatusResponse.Body.Bytes(), &secondStatus); err != nil || secondStatus.CanUndo || secondStatus.CanRedo {
		t.Fatalf("Input history leaked across editors: %+v err=%v", secondStatus, err)
	}
	request(http.MethodPost, path, secondLogin.Token, secondLogin.CSRFToken, routeDraftHistoryRequest{Direction: "UNDO"}, http.StatusNotFound)
	request(http.MethodPost, path, firstLogin.Token, firstLogin.CSRFToken, routeDraftHistoryRequest{Direction: "UNDO"}, http.StatusOK)
	inputs, err := st.ListInputs(ctx, revision.ID)
	if err != nil || len(inputs) != 0 {
		t.Fatalf("HTTP Undo did not remove only its editor's Input: inputs=%+v err=%v", inputs, err)
	}
	request(http.MethodPost, path, firstLogin.Token, firstLogin.CSRFToken, routeDraftHistoryRequest{Direction: "REDO"}, http.StatusOK)
	inputs, err = st.ListInputs(ctx, revision.ID)
	if err != nil || len(inputs) != 1 || inputs[0].ID != created.ID {
		t.Fatalf("HTTP Redo did not restore Input identity: inputs=%+v err=%v", inputs, err)
	}
	if snapshot, err := st.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID); err != nil || snapshot != nil {
		t.Fatalf("Input Draft history unexpectedly published: snapshot=%+v err=%v", snapshot, err)
	}
}
