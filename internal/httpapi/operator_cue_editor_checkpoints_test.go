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

func TestCueEditorCheckpointsArePrivateScopedAndDraftOnly(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	tech, err := h.auth.CreateUser(ctx, "checkpoint-tech", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, tech.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Checkpoint Project", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, otherRevision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Other Checkpoint Project", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	makeCue := func(revisionID, name string) domain.Cue {
		t.Helper()
		cue, err := st.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revisionID, DisplayLabel: "1A", Name: name, OrderIndex: 1,
			Enabled: true, ExecutionPolicy: json.RawMessage(`{"linked_cue_ids":["nested-child"]}`),
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return cue
	}
	cue := makeCue(revision.ID, "Mixed opening")
	foreignCue := makeCue(otherRevision.ID, "Other project cue")
	if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.CueEdit}); err != nil {
		t.Fatal(err)
	}
	makeHandler := func(beforeResponse func()) http.Handler {
		return New(withOperatorCuePublish(h.auth, st, publish.New(st, capability.NewRegistry()), cuePublishHooks{
			BeforeResponse: beforeResponse,
		}), WithProjectRouteQualification(access)).Handler()
	}
	path := "/api/v1/projects/" + project.ID + "/cues/" + cue.ID + "/editor-checkpoints"
	document := []byte(`{"version":1,"cue_id":"` + cue.ID + `","base":"mixed-base","draft":{"fields":["1A","Mixed opening"],"links":["nested-child"],"actions":[{"capability_key":"lighting.channels.fade"},{"capability_key":"osc.send"}]}}`)
	request := func(handler http.Handler, method, route, token, csrf string, body []byte, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := authenticatedMutationRequest(t, method, route, body, token, csrf)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s returned %d want %d: %s", method, route, res.Code, want, res.Body.String())
		}
		return res
	}
	handler := makeHandler(nil)
	created := request(handler, http.MethodPost, path, login.Token, login.CSRFToken,
		[]byte(`{"name":"Compound rehearsal","document":`+string(document)+`}`), http.StatusCreated)
	if created.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private checkpoint response is cacheable")
	}
	var checkpoint store.CueEditorCheckpoint
	if err := json.Unmarshal(created.Body.Bytes(), &checkpoint); err != nil {
		t.Fatal(err)
	}
	if checkpoint.ID == "" || checkpoint.CueID != cue.ID || checkpoint.Name != "Compound rehearsal" || string(checkpoint.Document) != string(document) {
		t.Fatalf("created checkpoint lost scoped mixed draft: %+v", checkpoint)
	}
	listed := request(handler, http.MethodGet, path, login.Token, "", nil, http.StatusOK)
	if listed.Header().Get("Cache-Control") != "no-store" || listed.Body.String() == "" {
		t.Fatalf("private list response missing no-store/content: headers=%v body=%s", listed.Header(), listed.Body)
	}
	var list struct {
		Checkpoints []store.CueEditorCheckpoint `json:"checkpoints"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Checkpoints) != 1 || list.Checkpoints[0].ID != checkpoint.ID {
		t.Fatalf("listed checkpoints=%+v", list.Checkpoints)
	}
	ownerList := request(handler, http.MethodGet, path, owner.Token, "", nil, http.StatusOK)
	if ownerList.Body.String() != "{\"checkpoints\":[]}\n" {
		t.Fatalf("checkpoint list leaked across users: %s", ownerList.Body)
	}
	request(handler, http.MethodPost, path, login.Token, login.CSRFToken,
		[]byte(`{"name":"invalid","document":{"version":1,"cue_id":"wrong","draft":{}}}`), http.StatusBadRequest)
	foreignPath := "/api/v1/projects/" + project.ID + "/cues/" + foreignCue.ID + "/editor-checkpoints"
	request(handler, http.MethodPost, foreignPath, login.Token, login.CSRFToken,
		[]byte(`{"name":"wrong project","document":{"version":1,"cue_id":"`+foreignCue.ID+`","draft":{}}}`), http.StatusNotFound)
	request(handler, http.MethodDelete, path+"/"+checkpoint.ID, owner.Token, owner.CSRFToken, nil, http.StatusNotFound)
	request(handler, http.MethodDelete, path+"/"+checkpoint.ID, login.Token, login.CSRFToken, nil, http.StatusNoContent)
	request(handler, http.MethodDelete, path+"/"+checkpoint.ID, login.Token, login.CSRFToken, nil, http.StatusNotFound)
	unchanged, err := st.GetCue(ctx, cue.ID)
	if err != nil || unchanged.Name != cue.Name || string(unchanged.ExecutionPolicy) != string(cue.ExecutionPolicy) {
		t.Fatalf("checkpoint handling mutated authored Cue: cue=%+v err=%v", unchanged, err)
	}
	_ = otherProject // Project isolation is exercised by the foreign Cue check above.
}

func TestCueEditorCheckpointSuppressesCommittedResponseAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	tech, err := h.auth.CreateUser(ctx, "checkpoint-revoke-tech", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, tech.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Revoked checkpoint", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := st.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Safe", OrderIndex: 1,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.CueEdit}); err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withOperatorCuePublish(h.auth, st, publish.New(st, capability.NewRegistry()), cuePublishHooks{
		BeforeResponse: func() {
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
				t.Fatal(err)
			}
		},
	}), WithProjectRouteQualification(access)).Handler()
	document := []byte(`{"version":1,"cue_id":"` + cue.ID + `","draft":{"fields":["1","Safe"],"links":[],"actions":[]}}`)
	body := []byte(`{"name":"Revoke boundary","document":` + string(document) + `}`)
	path := "/api/v1/projects/" + project.ID + "/cues/" + cue.ID + "/editor-checkpoints"
	req := authenticatedMutationRequest(t, http.MethodPost, path, body, login.Token, login.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if !revoked || response.Code != http.StatusForbidden {
		t.Fatalf("revoked checkpoint response boundary: revoked=%v status=%d body=%s", revoked, response.Code, response.Body.String())
	}
	checkpoints, err := st.ListCueEditorCheckpoints(ctx, project.ID, tech.ID, cue.ID)
	if err != nil || len(checkpoints) != 1 || checkpoints[0].Name != "Revoke boundary" {
		t.Fatalf("committed checkpoint was not retained after result suppression: checkpoints=%+v err=%v", checkpoints, err)
	}
}
