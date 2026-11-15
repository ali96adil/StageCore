package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOperatorConfigurationLockIsVisibleAndBlocksDraftDuringShow(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "F-012 API", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Cue One", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	show, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "active show")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorConfigurationDraft(h.auth, stageStore)).Handler()

	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "127.0.0.1:18120"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		if method != http.MethodGet {
			req.Header.Set(csrfHeader, credential.CSRFToken)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	lockRes := request(http.MethodGet, "/api/v1/projects/"+project.ID+"/configuration/lock")
	if lockRes.Code != http.StatusOK {
		t.Fatalf("lock status=%d body=%s", lockRes.Code, lockRes.Body.String())
	}
	var lockPayload struct {
		Lock store.ShowConfigurationLockState `json:"show_configuration_lock"`
	}
	if err := json.Unmarshal(lockRes.Body.Bytes(), &lockPayload); err != nil {
		t.Fatal(err)
	}
	if !lockPayload.Lock.Locked || lockPayload.Lock.ActiveShowSessionID == nil || *lockPayload.Lock.ActiveShowSessionID != show.ID || lockPayload.Lock.RuntimeSnapshotID == nil || *lockPayload.Lock.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("lock payload=%+v", lockPayload.Lock)
	}

	draftRes := request(http.MethodPost, "/api/v1/projects/"+project.ID+"/configuration/draft")
	if draftRes.Code != http.StatusLocked {
		t.Fatalf("draft status=%d body=%s", draftRes.Code, draftRes.Body.String())
	}
	var blocked struct {
		ErrorCode string                           `json:"error_code"`
		Lock      store.ShowConfigurationLockState `json:"show_configuration_lock"`
	}
	if err := json.Unmarshal(draftRes.Body.Bytes(), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.ErrorCode != "SHOW_CONFIGURATION_LOCKED" || !blocked.Lock.Locked {
		t.Fatalf("blocked response=%+v", blocked)
	}

	technician, err := h.auth.CreateUser(ctx, "lock-response-operator", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	technicianLogin, err := h.auth.Login(ctx, technician.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, credential.Session.User.ID, technician.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}); err != nil {
		t.Fatal(err)
	}
	revokedHandler := New(
		withOperatorConfigurationDraft(h.auth, stageStore, operatorConfigurationDraftHooks{BeforeResponse: func() {
			if err := access.SetPermissions(ctx, credential.Session.User.ID, technician.ID, project.ID, nil); err != nil {
				t.Errorf("revoke ProjectEdit before locked response: %v", err)
			}
		}}),
		WithProjectRouteQualification(access),
	).Handler()
	revokedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/configuration/draft", nil)
	revokedRequest.RemoteAddr = "127.0.0.2:18121"
	revokedRequest.Header.Set(csrfHeader, technicianLogin.CSRFToken)
	revokedRequest.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: technicianLogin.Token})
	revokedResponse := httptest.NewRecorder()
	revokedHandler.ServeHTTP(revokedResponse, revokedRequest)
	if revokedResponse.Code != http.StatusNotFound || strings.Contains(revokedResponse.Body.String(), show.ID) || strings.Contains(revokedResponse.Body.String(), runtimeSnapshot.ID) {
		t.Fatalf("locked state escaped after ProjectEdit revocation: status=%d body=%s", revokedResponse.Code, revokedResponse.Body.String())
	}
}
