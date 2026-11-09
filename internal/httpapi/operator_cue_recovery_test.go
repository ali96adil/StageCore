package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestCueEditorRecoveryIsPrivatePersistentAndFencesStaleTabs(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	tech, err := h.auth.CreateUser(ctx, "cue-tech", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.auth.Login(ctx, tech.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	makeProject := func(name string) string {
		t.Helper()
		project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: name, CreatedBy: owner.Session.User.ID})
		if err != nil {
			t.Fatal(err)
		}
		return project.ID
	}
	project, otherProject := makeProject("Recovery"), makeProject("Other recovery")
	if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project, []projectaccess.Permission{projectaccess.ProjectView, projectaccess.CueEdit}); err != nil {
		t.Fatal(err)
	}
	makeHandler := func() http.Handler {
		st := store.New(h.db.DB, clock.Real{})
		return New(WithOperatorCuePublish(h.auth, st, publish.New(st, capability.NewRegistry())), WithProjectRouteQualification(access)).Handler()
	}
	handler := makeHandler()
	request := func(method, id, token, csrf, generation string, body []byte, status int) store.CueEditorRecovery {
		t.Helper()
		req := authenticatedMutationRequest(t, method, "/api/v1/projects/"+id+"/cue-editor-recovery", body, token, csrf)
		req.Header.Set("If-Match", generation)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s recovery returned %d want %d: %s", method, res.Code, status, res.Body)
		}
		var payload store.CueEditorRecovery
		if status == http.StatusOK {
			if res.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private recovery response is cacheable")
			}
			if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
		}
		return payload
	}
	document := []byte(`{"document":{"cue_id":"","draft":{"raw":"{unfinished"}}}`)
	first := request(http.MethodPut, project, owner.Token, owner.CSRFToken, "", document, http.StatusOK)
	if first.Generation == "" {
		t.Fatal("missing generation")
	}
	handler = makeHandler()
	recovered := request(http.MethodGet, project, owner.Token, "", "", nil, http.StatusOK)
	if recovered.Generation != first.Generation || string(recovered.Document) != `{"cue_id":"","draft":{"raw":"{unfinished"}}` {
		t.Fatalf("recovered=%+v", recovered)
	}
	if request(http.MethodGet, project, second.Token, "", "", nil, http.StatusOK).Generation != "" ||
		request(http.MethodGet, otherProject, owner.Token, "", "", nil, http.StatusOK).Generation != "" {
		t.Fatal("private recovery leaked across users or projects")
	}
	request(http.MethodDelete, project, second.Token, second.CSRFToken, first.Generation, nil, http.StatusConflict)
	request(http.MethodGet, otherProject, second.Token, "", "", nil, http.StatusNotFound)
	presetRequest := func(method, id, suffix, token, csrf string, body []byte, expected int) *httptest.ResponseRecorder {
		t.Helper()
		req := authenticatedMutationRequest(t, method, "/api/v1/projects/"+id+"/cue-action-presets"+suffix, body, token, csrf)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != expected {
			t.Fatalf("%s preset status=%d want=%d: %s", method, res.Code, expected, res.Body)
		}
		return res
	}
	action := []byte(`{"name":"Scene","action":{"action_id":"original","order_index":10,"capability_key":"osc.send","parameters":{"address":"/scene"},"enabled":true}}`)
	created := presetRequest(http.MethodPost, project, "", owner.Token, owner.CSRFToken, action, http.StatusCreated)
	var preset store.CueActionPreset
	if err := json.Unmarshal(created.Body.Bytes(), &preset); err != nil {
		t.Fatal(err)
	}
	var authored actionWriteRequest
	if err := json.Unmarshal(preset.Action, &authored); err != nil {
		t.Fatal(err)
	}
	if preset.ID == "" || authored.ActionID != "" || authored.OrderIndex != 0 || authored.CapabilityKey != "osc.send" {
		t.Fatalf("preset persisted cue identity or lost capability: %+v", authored)
	}
	presetRequest(http.MethodPost, project, "", owner.Token, owner.CSRFToken, action, http.StatusConflict)
	presetRequest(http.MethodDelete, project, "/"+preset.ID, second.Token, second.CSRFToken, nil, http.StatusNotFound)
	presetRequest(http.MethodDelete, otherProject, "/"+preset.ID, owner.Token, owner.CSRFToken, nil, http.StatusNotFound)
	visible := presetRequest(http.MethodGet, project, "", second.Token, "", nil, http.StatusOK)
	if visible.Body.String() != "{\"presets\":[]}\n" {
		t.Fatalf("presets leaked across users: %s", visible.Body)
	}
	presetRequest(http.MethodDelete, project, "/"+preset.ID, owner.Token, owner.CSRFToken, nil, http.StatusNoContent)
	if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, project, second.Token, "", "", nil, http.StatusNotFound)
	presetRequest(http.MethodGet, project, "", second.Token, "", nil, http.StatusNotFound)
	request(http.MethodPut, project, second.Token, second.CSRFToken, "", document, http.StatusNotFound)
	request(http.MethodPut, project, owner.Token, owner.CSRFToken, "", document, http.StatusConflict)
	request(http.MethodDelete, project, owner.Token, "", first.Generation, nil, http.StatusForbidden)
	request(http.MethodPut, project, owner.Token, owner.CSRFToken, "", []byte(`{"document":[]}`), http.StatusBadRequest)
	request(http.MethodDelete, project, owner.Token, owner.CSRFToken, first.Generation, nil, http.StatusOK)
	newDraft := request(http.MethodPut, project, owner.Token, owner.CSRFToken, "", document, http.StatusOK)
	if newDraft.Generation == first.Generation {
		t.Fatal("replacement reused stale generation")
	}
	request(http.MethodDelete, project, owner.Token, owner.CSRFToken, first.Generation, nil, http.StatusConflict)
	if err := h.auth.Logout(ctx, owner.Token); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, project, owner.Token, "", "", nil, http.StatusUnauthorized)
}
