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

func TestOperatorVisualEngineWorkspaceRequiresAuthAndReturnsCanonicalProjectState(t *testing.T) {
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
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Workspace", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VISUAL-MAIN", DisplayName: "Main Visual Renderer",
		RequiredCapabilities: []string{visualengine.CapabilityPlay, deviceexperience.CapabilityVideoSourceRoute}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "AUDIO-MAIN", DisplayName: "Audio", RequiredCapabilities: []string{"audio.play"}, Required: true,
	}); err != nil {
		t.Fatal(err)
	}
	visualOutput, err := stageStore.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Main Visual", TargetRef: role.ID,
		CapabilityKey: visualengine.CapabilityOutputConfigure, ValueSchema: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Audio Out", TargetRef: "audio-main", CapabilityKey: "audio.play", ValueSchema: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, Name: "Visual Cue", OrderIndex: 1, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 1, TargetRef: role.ID, CapabilityKey: visualengine.CapabilityBlackout,
		Parameters: json.RawMessage(`{"contract_version":1,"enabled":true}`), Enabled: true,
	}, {
		OrderIndex: 2, TargetRef: "audio-main", CapabilityKey: "audio.play", Parameters: json.RawMessage(`{}`), Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	live, err := devices.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: project.ID, Name: "Camera A", Class: deviceexperience.SourceLocalCamera,
		ExecutionMachineRoleID: role.ID, Capabilities: []string{deviceexperience.CapabilityVideoSourceRoute},
		Required: true, DesiredEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + project.ID + "/visual-engine"
	unauth := httptest.NewRequest(http.MethodGet, path, nil)
	unauth.RemoteAddr = "127.0.0.1:15000"
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthRes.Code, unauthRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:15001"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("workspace status=%d body=%s", res.Code, res.Body.String())
	}
	var view visualEngineWorkspaceView
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ProjectID != project.ID || view.RevisionID != revision.ID || view.RevisionStatus != domain.RevisionDraft {
		t.Fatalf("unexpected project/revision: %+v", view)
	}
	if view.EngineMode != visualengine.EngineModeExternal {
		t.Fatalf("default engine mode=%q want=%q", view.EngineMode, visualengine.EngineModeExternal)
	}
	if view.ContractVersion != visualengine.ContractVersion1 || !view.NativeConfigured || !view.ExternalEngineSupported {
		t.Fatalf("unexpected workspace flags: %+v", view)
	}
	if len(view.MachineRoles) != 1 || view.MachineRoles[0].ID != role.ID {
		t.Fatalf("visual roles=%+v", view.MachineRoles)
	}
	if len(view.LiveSources) != 1 || view.LiveSources[0].ID != live.ID {
		t.Fatalf("live sources=%+v", view.LiveSources)
	}
	if len(view.Outputs) != 1 || view.Outputs[0].ID != visualOutput.ID {
		t.Fatalf("visual outputs=%+v", view.Outputs)
	}
	if len(view.Actions) != 1 || view.Actions[0].CueID != cue.ID || view.Actions[0].Action.CapabilityKey != visualengine.CapabilityBlackout {
		t.Fatalf("visual actions=%+v", view.Actions)
	}
	if len(view.Capabilities) == 0 {
		t.Fatal("visual capability catalog is empty")
	}
}

func TestOperatorVisualEngineModeMutatesDraftAndRejectsInvalidMode(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore, handler := newVisualEngineAcceptanceHandler(t, h)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode Draft", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/projects/" + project.ID + "/visual-engine/mode"

	nativeRes := visualEngineAuthenticatedRequest(handler, owner.Token, owner.CSRFToken, http.MethodPut, path, `{"engine_mode":"NATIVE"}`)
	if nativeRes.Code != http.StatusOK {
		t.Fatalf("NATIVE status=%d body=%s", nativeRes.Code, nativeRes.Body.String())
	}
	var native visualEngineModeView
	if err := json.Unmarshal(nativeRes.Body.Bytes(), &native); err != nil {
		t.Fatal(err)
	}
	if native.RevisionID != revision.ID || native.RevisionStatus != domain.RevisionDraft || native.EngineMode != visualengine.EngineModeNative {
		t.Fatalf("NATIVE response=%+v", native)
	}
	mode, err := stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeNative {
		t.Fatalf("stored NATIVE mode=%q", mode)
	}

	externalRes := visualEngineAuthenticatedRequest(handler, owner.Token, owner.CSRFToken, http.MethodPut, path, `{"engine_mode":"EXTERNAL"}`)
	if externalRes.Code != http.StatusOK {
		t.Fatalf("EXTERNAL status=%d body=%s", externalRes.Code, externalRes.Body.String())
	}
	mode, err = stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeExternal {
		t.Fatalf("stored EXTERNAL mode=%q", mode)
	}

	invalidRes := visualEngineAuthenticatedRequest(handler, owner.Token, owner.CSRFToken, http.MethodPut, path, `{"engine_mode":"UNSUPPORTED"}`)
	if invalidRes.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode status=%d body=%s", invalidRes.Code, invalidRes.Body.String())
	}
	var invalid struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(invalidRes.Body.Bytes(), &invalid); err != nil {
		t.Fatal(err)
	}
	if invalid.ErrorCode != "VISUAL_ENGINE_INVALID_MODE" {
		t.Fatalf("invalid mode response=%+v", invalid)
	}
	mode, err = stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeExternal {
		t.Fatalf("invalid request mutated mode=%q", mode)
	}
}

func TestOperatorVisualEngineModeForksValidatedRevisionAndPreservesSource(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore, handler := newVisualEngineAcceptanceHandler(t, h)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, source, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode Fork", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, source.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + project.ID + "/visual-engine/mode"
	res := visualEngineAuthenticatedRequest(handler, owner.Token, owner.CSRFToken, http.MethodPut, path, `{"engine_mode":"NATIVE"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("validated mutation status=%d body=%s", res.Code, res.Body.String())
	}
	var view visualEngineModeView
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.RevisionID == source.ID || view.RevisionStatus != domain.RevisionDraft || view.EngineMode != visualengine.EngineModeNative {
		t.Fatalf("validated mutation response=%+v source=%s", view, source.ID)
	}
	draft, err := stageStore.GetRevision(ctx, view.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if draft.ParentRevisionID == nil || *draft.ParentRevisionID != source.ID {
		t.Fatalf("draft parent=%v want=%s", draft.ParentRevisionID, source.ID)
	}
	current, err := stageStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevisionID != draft.ID {
		t.Fatalf("current revision=%s want=%s", current.CurrentRevisionID, draft.ID)
	}
	sourceMode, err := stageStore.GetVisualEngineMode(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sourceMode != visualengine.EngineModeExternal {
		t.Fatalf("validated source mutated mode=%q", sourceMode)
	}
	draftMode, err := stageStore.GetVisualEngineMode(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if draftMode != visualengine.EngineModeNative {
		t.Fatalf("draft mode=%q want=%q", draftMode, visualengine.EngineModeNative)
	}
}

func TestOperatorVisualEngineModeBlocksMutationDuringActiveShow(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore, handler := newVisualEngineAcceptanceHandler(t, h)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode SHOW Lock", CreatedBy: owner.Session.User.ID})
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
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "visual engine lock"); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + project.ID + "/visual-engine/mode"
	res := visualEngineAuthenticatedRequest(handler, owner.Token, owner.CSRFToken, http.MethodPut, path, `{"engine_mode":"NATIVE"}`)
	if res.Code != http.StatusLocked {
		t.Fatalf("SHOW mutation status=%d body=%s", res.Code, res.Body.String())
	}
	var blocked struct {
		ErrorCode string                           `json:"error_code"`
		Lock      store.ShowConfigurationLockState `json:"show_configuration_lock"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.ErrorCode != "SHOW_CONFIGURATION_LOCKED" || !blocked.Lock.Locked {
		t.Fatalf("SHOW lock response=%+v", blocked)
	}
	current, err := stageStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevisionID != revision.ID {
		t.Fatalf("SHOW lock created revision=%s source=%s", current.CurrentRevisionID, revision.ID)
	}
	mode, err := stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeExternal {
		t.Fatalf("SHOW lock mutated mode=%q", mode)
	}
}

func TestOperatorVisualEngineRBACAllowsReadButDeniesEditForOperatorAndViewer(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore, handler := newVisualEngineAcceptanceHandler(t, h)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Visual Mode RBAC", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	operator := addVisualEngineTestUser(t, h, "00000000-0000-7000-8000-000000000010", "operator", userauth.RoleOperator, "127.0.0.2")
	viewer := addVisualEngineTestUser(t, h, "00000000-0000-7000-8000-000000000011", "viewer", userauth.RoleViewer, "127.0.0.3")

	workspacePath := "/api/v1/projects/" + project.ID + "/visual-engine"
	modePath := workspacePath + "/mode"
	for _, credential := range []userauth.Credential{operator, viewer} {
		readRes := visualEngineAuthenticatedRequest(handler, credential.Token, credential.CSRFToken, http.MethodGet, workspacePath, "")
		if readRes.Code != http.StatusOK {
			t.Fatalf("%s read status=%d body=%s", credential.Session.User.Role, readRes.Code, readRes.Body.String())
		}
		editRes := visualEngineAuthenticatedRequest(handler, credential.Token, credential.CSRFToken, http.MethodPut, modePath, `{"engine_mode":"NATIVE"}`)
		if editRes.Code != http.StatusForbidden {
			t.Fatalf("%s edit status=%d body=%s", credential.Session.User.Role, editRes.Code, editRes.Body.String())
		}
	}
	mode, err := stageStore.GetVisualEngineMode(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeExternal {
		t.Fatalf("RBAC-denied edits mutated mode=%q", mode)
	}
	current, err := stageStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentRevisionID != revision.ID {
		t.Fatalf("RBAC-denied edits changed revision=%s want=%s", current.CurrentRevisionID, revision.ID)
	}
}

func newVisualEngineAcceptanceHandler(t *testing.T, h *authHarness) (*store.Store, http.Handler) {
	t.Helper()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	return stageStore, New(WithOperatorVisualEngine(h.auth, stageStore, devices)).Handler()
}

func visualEngineAuthenticatedRequest(handler http.Handler, token, csrfToken, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:15002"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, csrfToken)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func addVisualEngineTestUser(t *testing.T, h *authHarness, userID, username, role, remoteKey string) userauth.Credential {
	t.Helper()
	ctx := context.Background()
	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES (?, ?, ?, ?, 1, 1, 1)
	`, userID, username, passwordHash, role); err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, username, h.password, remoteKey)
	if err != nil {
		t.Fatal(err)
	}
	return credential
}
