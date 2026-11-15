package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestRouteDraftHistoryHTTPIsEditorScopedAndNeverPublishes(t *testing.T) {
	ctx := context.Background()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	firstEditor, err := h.auth.CreateUser(ctx, "route-history-first", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	firstLogin, err := h.auth.Login(ctx, firstEditor.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	secondEditor, err := h.auth.CreateUser(ctx, "route-history-second", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	secondLogin, err := h.auth.Login(ctx, secondEditor.Username, h.password, "127.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.db.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Route Draft history API", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	input, err := st.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
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
	path := "/api/v1/projects/" + project.ID + "/route-draft-history"
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

	routeResponse := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/routes", firstLogin.Token, firstLogin.CSRFToken, map[string]any{
		"name": "GO Route", "input_id": input.ID, "enabled": true, "actions": []any{},
	}, http.StatusCreated)
	var created routeConfigurationView
	if err := json.Unmarshal(routeResponse.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("created Route=%+v err=%v", created, err)
	}
	firstStatusResponse := request(http.MethodGet, path, firstLogin.Token, "", nil, http.StatusOK)
	var firstStatus store.RouteDraftHistoryStatus
	if err := json.Unmarshal(firstStatusResponse.Body.Bytes(), &firstStatus); err != nil ||
		firstStatusResponse.Header().Get("Cache-Control") != "no-store" || !firstStatus.CanUndo || firstStatus.CanRedo {
		t.Fatalf("first editor Route history=%+v headers=%v err=%v", firstStatus, firstStatusResponse.Header(), err)
	}
	secondStatusResponse := request(http.MethodGet, path, secondLogin.Token, "", nil, http.StatusOK)
	var secondStatus store.RouteDraftHistoryStatus
	if err := json.Unmarshal(secondStatusResponse.Body.Bytes(), &secondStatus); err != nil || secondStatus.CanUndo || secondStatus.CanRedo {
		t.Fatalf("Route history leaked across editors: %+v err=%v", secondStatus, err)
	}
	request(http.MethodPost, path, secondLogin.Token, secondLogin.CSRFToken,
		routeDraftHistoryRequest{Direction: "UNDO"}, http.StatusNotFound)
	request(http.MethodPost, path, firstLogin.Token, firstLogin.CSRFToken,
		routeDraftHistoryRequest{Direction: "UNDO"}, http.StatusOK)
	routes, err := st.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 0 {
		t.Fatalf("HTTP Undo did not remove only its editor's Route: routes=%+v err=%v", routes, err)
	}
	if snapshot, err := st.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID); err != nil || snapshot != nil {
		t.Fatalf("Route Draft history unexpectedly published: snapshot=%+v err=%v", snapshot, err)
	}
}
