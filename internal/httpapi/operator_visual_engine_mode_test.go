package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

func visualEngineModeRequest(handler http.Handler, credential userauth.Credential, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:15260"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	if method != http.MethodGet {
		req.Header.Set(csrfHeader, credential.CSRFToken)
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func TestOperatorVisualEngineModeMutationUsesDraftAuthority(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorVisualEngine(h.auth, stageStore, devices)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode Draft", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + project.ID + "/visual-engine/mode"
	res := visualEngineModeRequest(handler, owner, http.MethodPut, path, `{"engine_mode":"NATIVE"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("mode mutation status=%d body=%s", res.Code, res.Body.String())
	}
	var view visualEngineModeView
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.RevisionID != revision.ID || view.RevisionStatus != domain.RevisionDraft || view.EngineMode != visualengine.EngineModeNative {
		t.Fatalf("mode view=%+v", view)
	}
	mode, err := stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeNative {
		t.Fatalf("stored mode=%q", mode)
	}

	invalid := visualEngineModeRequest(handler, owner, http.MethodPut, path, `{"engine_mode":"AUTO"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	mode, err = stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeNative {
		t.Fatalf("invalid mutation changed mode=%q", mode)
	}
}

func TestOperatorVisualEngineModeForksValidatedRevisionWithoutMutatingSource(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorVisualEngine(h.auth, stageStore, devices)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, source, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode Fork", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetVisualEngineMode(ctx, source.ID, visualengine.EngineModeNative, owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, source.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + project.ID + "/visual-engine/mode"
	res := visualEngineModeRequest(handler, owner, http.MethodPut, path, `{"engine_mode":"EXTERNAL"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("validated mutation status=%d body=%s", res.Code, res.Body.String())
	}
	var view visualEngineModeView
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.RevisionID == source.ID || view.RevisionStatus != domain.RevisionDraft || view.EngineMode != visualengine.EngineModeExternal {
		t.Fatalf("fork response=%+v source=%s", view, source.ID)
	}
	current, err := stageStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevisionID != view.RevisionID {
		t.Fatalf("current revision=%s want=%s", current.CurrentRevisionID, view.RevisionID)
	}
	draft, err := stageStore.GetRevision(ctx, view.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if draft.ParentRevisionID == nil || *draft.ParentRevisionID != source.ID {
		t.Fatalf("draft parent=%v want=%s", draft.ParentRevisionID, source.ID)
	}
	sourceAfter, err := stageStore.GetRevision(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sourceAfter.Status != domain.RevisionValidated {
		t.Fatalf("source status=%s", sourceAfter.Status)
	}
	sourceMode, err := stageStore.GetVisualEngineMode(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	draftMode, err := stageStore.GetVisualEngineMode(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sourceMode != visualengine.EngineModeNative || draftMode != visualengine.EngineModeExternal {
		t.Fatalf("source mode=%q draft mode=%q", sourceMode, draftMode)
	}
}

func TestOperatorVisualEngineModeMutationIsLockedDuringActiveShow(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorVisualEngine(h.auth, stageStore, devices)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode Show Lock", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetVisualEngineMode(ctx, revision.ID, visualengine.EngineModeNative, owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Visual Cue", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "Visual mode lock"); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + project.ID + "/visual-engine/mode"
	res := visualEngineModeRequest(handler, owner, http.MethodPut, path, `{"engine_mode":"EXTERNAL"}`)
	if res.Code != http.StatusLocked {
		t.Fatalf("SHOW lock status=%d body=%s", res.Code, res.Body.String())
	}
	var blocked struct {
		ErrorCode string                           `json:"error_code"`
		Lock      store.ShowConfigurationLockState `json:"show_configuration_lock"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.ErrorCode != "SHOW_CONFIGURATION_LOCKED" || !blocked.Lock.Locked {
		t.Fatalf("blocked payload=%+v", blocked)
	}
	current, err := stageStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevisionID != revision.ID {
		t.Fatalf("SHOW mutation forked revision=%s want=%s", current.CurrentRevisionID, revision.ID)
	}
	mode, err := stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeNative {
		t.Fatalf("SHOW mutation changed mode=%q", mode)
	}
}

func TestOperatorVisualEngineModeMutationRequiresProjectEdit(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorVisualEngine(h.auth, stageStore, devices)).Handler()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode RBAC", CreatedBy: credential.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	putPath := "/api/v1/projects/" + project.ID + "/visual-engine/mode"
	getPath := "/api/v1/projects/" + project.ID + "/visual-engine"

	for _, role := range []string{userauth.RoleOperator, userauth.RoleViewer} {
		if _, err := h.db.DB.ExecContext(ctx, `UPDATE local_users SET role = ? WHERE user_id = ?`, role, credential.Session.User.ID); err != nil {
			t.Fatal(err)
		}
		res := visualEngineModeRequest(handler, credential, http.MethodPut, putPath, `{"engine_mode":"NATIVE"}`)
		if res.Code != http.StatusForbidden {
			t.Fatalf("role=%s PUT status=%d body=%s", role, res.Code, res.Body.String())
		}
		read := visualEngineModeRequest(handler, credential, http.MethodGet, getPath, "")
		if read.Code != http.StatusOK {
			t.Fatalf("role=%s GET status=%d body=%s", role, read.Code, read.Body.String())
		}
	}
}
