package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	mixedRequest := []byte(`{"name":"Warm scene","actions":[{"action_id":"source-light","order_index":8,"target_ref":"LIGHTING","capability_key":"lighting.channels.fade","parameters":{"levels":{"warm":55},"fade_ms":900}},{"action_id":"source-osc","order_index":9,"target_ref":"VDMX","capability_key":"osc.send","parameters":{"address":"/scene/4","arguments":[4]}}]}`)
	mixedCreated := presetRequest(http.MethodPost, project, "", owner.Token, owner.CSRFToken, mixedRequest, http.StatusCreated)
	var mixedPreset store.CueActionPreset
	if err := json.Unmarshal(mixedCreated.Body.Bytes(), &mixedPreset); err != nil {
		t.Fatal(err)
	}
	if mixedPreset.Action != nil || len(mixedPreset.Actions) != 2 {
		t.Fatalf("mixed preset response did not preserve its actions field: %+v", mixedPreset)
	}
	var mixedActions []actionWriteRequest
	for _, raw := range mixedPreset.Actions {
		var action actionWriteRequest
		if err := json.Unmarshal(raw, &action); err != nil {
			t.Fatal(err)
		}
		mixedActions = append(mixedActions, action)
	}
	if mixedActions[0].ActionID != "" || mixedActions[0].OrderIndex != 0 || mixedActions[0].CapabilityKey != "lighting.channels.fade" ||
		mixedActions[1].ActionID != "" || mixedActions[1].OrderIndex != 1 || mixedActions[1].CapabilityKey != "osc.send" {
		t.Fatalf("mixed preset retained Cue identities or changed order: %+v", mixedActions)
	}
	var listed struct {
		Presets []store.CueActionPreset `json:"presets"`
	}
	listedResponse := presetRequest(http.MethodGet, project, "", owner.Token, "", nil, http.StatusOK)
	if err := json.Unmarshal(listedResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Presets) != 2 || listed.Presets[0].Action == nil || len(listed.Presets[0].Actions) != 0 ||
		len(listed.Presets[1].Actions) != 2 || listed.Presets[1].Action != nil {
		t.Fatalf("listing did not preserve the old single-action shape and new multi-action shape: %+v", listed.Presets)
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

func TestCueEditorPrivateWritesRecheckProjectGrantAtCommit(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	tech, err := h.auth.CreateUser(ctx, "private-cue-tech", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, tech.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Private cue data", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorCuePublish(h.auth, st, publish.New(st, capability.NewRegistry())), WithProjectRouteQualification(access)).Handler()
	for _, request := range []struct {
		method string
		route  string
		body   string
	}{
		{http.MethodPut, "cue-editor-recovery", `{"document":{"draft":true}}`},
		{http.MethodPost, "cue-action-presets", `{"name":"Scene","action":{"capability_key":"osc.send"}}`},
	} {
		t.Run(request.route, func(t *testing.T) {
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView, projectaccess.CueEdit}); err != nil {
				t.Fatal(err)
			}
			req := authenticatedMutationRequest(t, request.method, "/api/v1/projects/"+project.ID+"/"+request.route, []byte(request.body), login.Token, login.CSRFToken)
			req.Body = &firmwareUploadRevokeReader{ReadCloser: req.Body, revoke: func() {
				if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
					t.Fatal(err)
				}
			}}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusNotFound {
				t.Fatalf("write after revocation returned %d: %s", res.Code, res.Body)
			}
			if recovery, err := st.CueEditorRecovery(ctx, project.ID, tech.ID); err != nil || recovery.Generation != "" {
				t.Fatalf("recovery=%+v err=%v", recovery, err)
			}
			if presets, err := st.ListCueActionPresets(ctx, project.ID, tech.ID); err != nil || len(presets) != 0 {
				t.Fatalf("presets=%+v err=%v", presets, err)
			}
		})
	}
}

func TestCuePrivateReadRejectsSessionRevokedBeforeDelivery(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	req := authenticatedMutationRequest(t, http.MethodGet, "/api/v1/projects/example/cue-editor-recovery", nil, owner.Token, "")
	if err := h.auth.Logout(ctx, owner.Token); err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	if allowLiveProjectRead(res, req, h.auth, owner.Session, userauth.PermissionProjectEdit) || res.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session returned status %d", res.Code)
	}
}

func TestCueEditorRecoverySuppressesCommittedResultAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name:      "cue recovery response boundary",
		CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withOperatorCuePublish(
		h.auth,
		stageStore,
		publish.New(stageStore, capability.NewRegistry()),
		cuePublishHooks{BeforeResponse: func() {
			revoked = true
			if err := h.auth.Logout(ctx, owner.Token); err != nil {
				t.Fatal(err)
			}
		}},
	)).Handler()
	body := []byte(`{"document":{"cue_id":"","draft":{"raw":"{unfinished}"}}}`)
	request := authenticatedMutationRequest(t, http.MethodPut, "/api/v1/projects/"+project.ID+"/cue-editor-recovery", body, owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatal("response boundary hook did not revoke the Owner session")
	}
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"generation"`) {
		t.Fatalf("revoked recovery write exposed result: status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked recovery response lost commit uncertainty: %s", response.Body.String())
	}
	recovery, err := stageStore.CueEditorRecovery(ctx, project.ID, owner.Session.User.ID)
	if err != nil || recovery.Generation == "" || string(recovery.Document) != `{"cue_id":"","draft":{"raw":"{unfinished}"}}` {
		t.Fatalf("committed recovery was not retained truthfully: recovery=%+v err=%v", recovery, err)
	}
}
