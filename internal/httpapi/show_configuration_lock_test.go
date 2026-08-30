package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
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
		ErrorCode string `json:"error_code"`
		Lock store.ShowConfigurationLockState `json:"show_configuration_lock"`
	}
	if err := json.Unmarshal(draftRes.Body.Bytes(), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.ErrorCode != "SHOW_CONFIGURATION_LOCKED" || !blocked.Lock.Locked {
		t.Fatalf("blocked response=%+v", blocked)
	}
}
