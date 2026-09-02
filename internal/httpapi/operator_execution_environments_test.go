package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorExecutionEnvironmentLifecycleAndRevisionAuthority(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorExecutionEnvironments(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Execution Environment API", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-MAIN", DisplayName: "Main Video", Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Other Project", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRole, err := stageStore.CreateMachineRole(ctx, otherProject.ID, store.CreateMachineRoleParams{
		RoleKey: "OTHER-VIDEO", DisplayName: "Other Video", Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	collection := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments"
	createBody, err := json.Marshal(map[string]any{"manifest": testVDMXExecutionEnvironmentManifest("video-main")})
	if err != nil {
		t.Fatal(err)
	}

	unauthenticated := httptest.NewRequest(http.MethodPost, collection, bytes.NewReader(createBody))
	unauthenticated.RemoteAddr = "127.0.0.1:14000"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create status=%d body=%s", unauthenticatedRes.Code, unauthenticatedRes.Body.String())
	}

	createReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, collection, createBody)
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var created executionEnvironmentView
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.RevisionID != revision.ID || created.EnvironmentKey != "video-main" || created.AdapterKey != "stagecore.adapter.vdmx" {
		t.Fatalf("unexpected created environment: %+v", created)
	}
	if len(created.ContentSHA256) != 64 || created.MachineRoleID != nil {
		t.Fatalf("created environment identity/binding=%+v", created)
	}

	listReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodGet, collection, nil)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		Revision              revisionView               `json:"revision"`
		ExecutionEnvironments []executionEnvironmentView `json:"execution_environments"`
		MachineRoles          []machineRoleView          `json:"machine_roles"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Revision.ID != revision.ID || len(listed.ExecutionEnvironments) != 1 || len(listed.MachineRoles) != 1 || listed.MachineRoles[0].ID != role.ID {
		t.Fatalf("unexpected list: revision=%+v environments=%+v roles=%+v", listed.Revision, listed.ExecutionEnvironments, listed.MachineRoles)
	}

	bindingPath := collection + "/" + created.ID + "/machine-role"
	crossBody, _ := json.Marshal(map[string]any{"machine_role_id": otherRole.ID})
	crossReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPut, bindingPath, crossBody)
	crossRes := httptest.NewRecorder()
	handler.ServeHTTP(crossRes, crossReq)
	if crossRes.Code != http.StatusConflict {
		t.Fatalf("cross-project bind status=%d body=%s", crossRes.Code, crossRes.Body.String())
	}

	bindBody, _ := json.Marshal(map[string]any{"machine_role_id": role.ID})
	bindReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPut, bindingPath, bindBody)
	bindRes := httptest.NewRecorder()
	handler.ServeHTTP(bindRes, bindReq)
	if bindRes.Code != http.StatusOK {
		t.Fatalf("bind status=%d body=%s", bindRes.Code, bindRes.Body.String())
	}
	var bound executionEnvironmentView
	if err := json.Unmarshal(bindRes.Body.Bytes(), &bound); err != nil {
		t.Fatal(err)
	}
	if bound.MachineRoleID == nil || *bound.MachineRoleID != role.ID {
		t.Fatalf("bound role=%v want %s", bound.MachineRoleID, role.ID)
	}

	unbindReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPut, bindingPath, []byte(`{"machine_role_id":null}`))
	unbindRes := httptest.NewRecorder()
	handler.ServeHTTP(unbindRes, unbindReq)
	if unbindRes.Code != http.StatusOK {
		t.Fatalf("unbind status=%d body=%s", unbindRes.Code, unbindRes.Body.String())
	}
	var unbound executionEnvironmentView
	if err := json.Unmarshal(unbindRes.Body.Bytes(), &unbound); err != nil {
		t.Fatal(err)
	}
	if unbound.MachineRoleID != nil {
		t.Fatalf("unbind retained role=%v", unbound.MachineRoleID)
	}

	deleteManifest := testVDMXExecutionEnvironmentManifest("video-secondary")
	deleteBody, _ := json.Marshal(map[string]any{"manifest": deleteManifest})
	deleteCreateReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, collection, deleteBody)
	deleteCreateRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteCreateRes, deleteCreateReq)
	if deleteCreateRes.Code != http.StatusCreated {
		t.Fatalf("create deletion fixture status=%d body=%s", deleteCreateRes.Code, deleteCreateRes.Body.String())
	}
	var deletionFixture executionEnvironmentView
	if err := json.Unmarshal(deleteCreateRes.Body.Bytes(), &deletionFixture); err != nil {
		t.Fatal(err)
	}
	deletePath := collection + "/" + deletionFixture.ID
	missingConfirmReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodDelete, deletePath, nil)
	missingConfirmRes := httptest.NewRecorder()
	handler.ServeHTTP(missingConfirmRes, missingConfirmReq)
	if missingConfirmRes.Code != http.StatusBadRequest {
		t.Fatalf("delete without confirmation status=%d body=%s", missingConfirmRes.Code, missingConfirmRes.Body.String())
	}
	confirmedDeleteReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodDelete, deletePath+"?confirm=true", nil)
	confirmedDeleteRes := httptest.NewRecorder()
	handler.ServeHTTP(confirmedDeleteRes, confirmedDeleteReq)
	if confirmedDeleteRes.Code != http.StatusNoContent {
		t.Fatalf("confirmed delete status=%d body=%s", confirmedDeleteRes.Code, confirmedDeleteRes.Body.String())
	}

	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	validatedReadReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodGet, collection, nil)
	validatedReadRes := httptest.NewRecorder()
	handler.ServeHTTP(validatedReadRes, validatedReadReq)
	if validatedReadRes.Code != http.StatusOK {
		t.Fatalf("validated read status=%d body=%s", validatedReadRes.Code, validatedReadRes.Body.String())
	}

	validatedDeleteReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodDelete, collection+"/"+created.ID+"?confirm=true", nil)
	validatedDeleteRes := httptest.NewRecorder()
	handler.ServeHTTP(validatedDeleteRes, validatedDeleteReq)
	if validatedDeleteRes.Code != http.StatusConflict {
		t.Fatalf("validated delete status=%d body=%s", validatedDeleteRes.Code, validatedDeleteRes.Body.String())
	}
	var validatedError map[string]any
	if err := json.Unmarshal(validatedDeleteRes.Body.Bytes(), &validatedError); err != nil {
		t.Fatal(err)
	}
	if validatedError["error_code"] != "REVISION_NOT_DRAFT" {
		t.Fatalf("validated delete error=%v", validatedError)
	}
}

func authenticatedExecutionEnvironmentRequest(t *testing.T, token, csrf, method, path string, body []byte) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "127.0.0.1:14001"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set(csrfHeader, csrf)
	}
	return req
}

func testVDMXExecutionEnvironmentManifest(environmentKey string) executionenv.Manifest {
	return executionenv.Manifest{
		SchemaVersion:  executionenv.ManifestSchemaVersion,
		EnvironmentKey: environmentKey,
		Name:           "Main video workstation",
		AdapterKey:     "stagecore.adapter.vdmx",
		Application: executionenv.ApplicationRequirement{
			Key: "vdmx", Name: "VDMX", Vendor: "VIDVOX", VersionConstraint: "8.x-tested",
			Hosts: []executionenv.HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
		Assets: []executionenv.AssetRequirement{{
			Key: "workspace", Kind: executionenv.AssetProjectFile, Name: "VDMX workspace",
			CapturePolicy: executionenv.CaptureReferenceOnly, Locator: "/Users/show/Stage.vdmx5",
		}},
		Launch: &executionenv.LaunchTarget{Kind: executionenv.LaunchAsset, AssetKey: "workspace"},
	}
}
