package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorMachineRoleProvisioningRequiresAuthAndTrustedCompanion(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorMachineRoles(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Companion Role Test", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	companion, err := stageStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		CompanionID: "11111111-1111-4111-8111-111111111111",
		DisplayName: "Video Mac", Platform: "macos", Architecture: "arm64",
		Version: "0.1.0", Capabilities: []string{"local.echo", "midi.send"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.UpdateCompanionReport(ctx, companion.ID, store.CompanionReportParams{
		DisplayName: "Video Mac", Platform: "macos", Architecture: "arm64", Version: "0.1.0",
		Capabilities: []string{"local.echo", "midi.send"},
		MIDIDestinations: []string{"IAC Driver Bus 1"},
		Readiness: domain.CompanionReadinessUnknown,
	}); err != nil {
		t.Fatal(err)
	}

	roleBody, _ := json.Marshal(map[string]any{
		"role_key": "VIDEO-MAIN", "display_name": "Main Video",
		"required_capabilities": []string{"local.echo"}, "required": true,
	})
	rolePath := "/api/v1/projects/" + project.ID + "/machine-roles"

	unauthenticated := httptest.NewRequest(http.MethodPost, rolePath, bytes.NewReader(roleBody))
	unauthenticated.RemoteAddr = "127.0.0.1:13000"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create status=%d body=%s", unauthenticatedRes.Code, unauthenticatedRes.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, rolePath, bytes.NewReader(roleBody))
	createReq.RemoteAddr = "127.0.0.1:13001"
	createReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	createReq.Header.Set(csrfHeader, owner.CSRFToken)
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create role status=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var role machineRoleView
	if err := json.Unmarshal(createRes.Body.Bytes(), &role); err != nil {
		t.Fatal(err)
	}
	if role.ProjectID != project.ID || role.RoleKey != "VIDEO-MAIN" || len(role.RequiredCapabilities) != 1 || role.RequiredCapabilities[0] != "local.echo" {
		t.Fatalf("unexpected role: %+v", role)
	}

	assignmentBody, _ := json.Marshal(map[string]string{"companion_id": companion.ID})
	assignmentPath := rolePath + "/" + role.ID + "/assignment"

	untrustedReq := httptest.NewRequest(http.MethodPost, assignmentPath, bytes.NewReader(assignmentBody))
	untrustedReq.RemoteAddr = "127.0.0.1:13002"
	untrustedReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	untrustedReq.Header.Set(csrfHeader, owner.CSRFToken)
	untrustedRes := httptest.NewRecorder()
	handler.ServeHTTP(untrustedRes, untrustedReq)
	if untrustedRes.Code != http.StatusConflict {
		t.Fatalf("untrusted assignment status=%d body=%s", untrustedRes.Code, untrustedRes.Body.String())
	}

	if err := stageStore.SetCompanionTrustState(ctx, companion.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	assignReq := httptest.NewRequest(http.MethodPost, assignmentPath, bytes.NewReader(assignmentBody))
	assignReq.RemoteAddr = "127.0.0.1:13003"
	assignReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	assignReq.Header.Set(csrfHeader, owner.CSRFToken)
	assignRes := httptest.NewRecorder()
	handler.ServeHTTP(assignRes, assignReq)
	if assignRes.Code != http.StatusCreated {
		t.Fatalf("trusted assignment status=%d body=%s", assignRes.Code, assignRes.Body.String())
	}
	var assignment roleAssignmentView
	if err := json.Unmarshal(assignRes.Body.Bytes(), &assignment); err != nil {
		t.Fatal(err)
	}
	if assignment.MachineRoleID != role.ID || assignment.CompanionID != companion.ID || assignment.State != domain.RoleAssigned {
		t.Fatalf("unexpected assignment: %+v", assignment)
	}

	repeatReq := httptest.NewRequest(http.MethodPost, assignmentPath, bytes.NewReader(assignmentBody))
	repeatReq.RemoteAddr = "127.0.0.1:13004"
	repeatReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	repeatReq.Header.Set(csrfHeader, owner.CSRFToken)
	repeatRes := httptest.NewRecorder()
	handler.ServeHTTP(repeatRes, repeatReq)
	if repeatRes.Code != http.StatusCreated {
		t.Fatalf("idempotent assignment status=%d body=%s", repeatRes.Code, repeatRes.Body.String())
	}
	var repeated roleAssignmentView
	if err := json.Unmarshal(repeatRes.Body.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.ID != assignment.ID {
		t.Fatalf("repeated assignment id=%s, want %s", repeated.ID, assignment.ID)
	}

	listReq := httptest.NewRequest(http.MethodGet, rolePath, nil)
	listReq.RemoteAddr = "127.0.0.1:13005"
	listReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list roles status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	var listing struct {
		Roles      []machineRoleView       `json:"roles"`
		Companions []companionRoleOptionView `json:"companions"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Roles) != 1 || listing.Roles[0].ID != role.ID ||
		listing.Roles[0].Assignment == nil ||
		listing.Roles[0].Assignment.ID != assignment.ID {
		t.Fatalf("unexpected role listing: %+v", listing.Roles)
	}
	if len(listing.Companions) != 1 || listing.Companions[0].ID != companion.ID ||
		listing.Companions[0].TrustState != domain.CompanionTrusted ||
		!machineRoleTestContains(listing.Companions[0].Capabilities, "midi.send") ||
		len(listing.Companions[0].MIDIDestinations) != 1 ||
		listing.Companions[0].MIDIDestinations[0] != "IAC Driver Bus 1" {
		t.Fatalf("unexpected companion options: %+v", listing.Companions)
	}
}

func TestOperatorMachineRoleAssignmentRejectsCrossProjectRole(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorMachineRoles(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "First", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Second", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, first.ID, store.CreateMachineRoleParams{RoleKey: "VIDEO-MAIN"})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"companion_id": "11111111-1111-4111-8111-111111111111"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+second.ID+"/machine-roles/"+role.ID+"/assignment", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:13100"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	req.Header.Set(csrfHeader, owner.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("cross-project assignment status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestOperatorMachineRoleRuntimeRequirementUsesPublishedProjectSnapshot(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorMachineRoles(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Runtime Requirement Test", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-MAIN", RequiredCapabilities: []string{"local.echo"}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshot, err := stageStore.CreateRuntimeSnapshot(
		ctx, revision.ID, owner.Session.User.ID, strings.Repeat("a", 64), json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{
		"runtime_snapshot_id": snapshot.ID,
		"config_hash": "qualification-config",
	})
	path := "/api/v1/projects/" + project.ID + "/machine-roles/" + role.ID + "/runtime-requirement"
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:13200"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	req.Header.Set(csrfHeader, owner.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("runtime requirement status=%d body=%s", res.Code, res.Body.String())
	}
	var updated machineRoleView
	if err := json.Unmarshal(res.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.RequiredRuntimeSnapshotID == nil || *updated.RequiredRuntimeSnapshotID != snapshot.ID {
		t.Fatalf("runtime snapshot=%v, want %s", updated.RequiredRuntimeSnapshotID, snapshot.ID)
	}
	if updated.RequiredConfigHash != "qualification-config" {
		t.Fatalf("config hash=%q", updated.RequiredConfigHash)
	}

	emptyReq := httptest.NewRequest(http.MethodPut, path, bytes.NewReader([]byte(`{"runtime_snapshot_id":""}`)))
	emptyReq.RemoteAddr = "127.0.0.1:13201"
	emptyReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	emptyReq.Header.Set(csrfHeader, owner.CSRFToken)
	emptyRes := httptest.NewRecorder()
	handler.ServeHTTP(emptyRes, emptyReq)
	if emptyRes.Code != http.StatusBadRequest {
		t.Fatalf("empty runtime requirement status=%d body=%s", emptyRes.Code, emptyRes.Body.String())
	}
}


func machineRoleTestContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}


func TestOperatorMachineRoleLifecyclePreservesIdentityAndHistory(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorMachineRoles(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Machine Role Lifecycle", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-MAIN", DisplayName: "Old Video",
		RequiredCapabilities: []string{"local.echo"}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, payload any) *httptest.ResponseRecorder {
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:13300"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		if method != http.MethodGet {
			req.Header.Set(csrfHeader, owner.CSRFToken)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	base := "/api/v1/projects/" + project.ID + "/machine-roles/" + role.ID
	update := request(http.MethodPut, base, map[string]any{
		"display_name": "Main Video",
		"required_capabilities": []string{"midi.send"},
		"required": false,
	})
	if update.Code != http.StatusOK {
		t.Fatalf("role update status=%d body=%s", update.Code, update.Body.String())
	}
	var updated machineRoleView
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != role.ID || updated.RoleKey != "VIDEO-MAIN" || updated.DisplayName != "Main Video" ||
		updated.Required || len(updated.RequiredCapabilities) != 1 || updated.RequiredCapabilities[0] != "midi.send" {
		t.Fatalf("updated role=%+v", updated)
	}

	companion, err := stageStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		CompanionID: "22222222-2222-4222-8222-222222222222",
		DisplayName: "Lifecycle Mac", Platform: "macos", Architecture: "arm64", Version: "1",
		Capabilities: []string{"midi.send"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetCompanionTrustState(ctx, companion.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.AssignMachineRole(ctx, role.ID, companion.ID); err != nil {
		t.Fatal(err)
	}

	retireAssigned := request(http.MethodPost, base+"/retire", map[string]any{"confirm": "RETIRE"})
	if retireAssigned.Code != http.StatusConflict {
		t.Fatalf("retire assigned role status=%d body=%s", retireAssigned.Code, retireAssigned.Body.String())
	}
	release := request(http.MethodDelete, base+"/assignment", nil)
	if release.Code != http.StatusNoContent {
		t.Fatalf("release status=%d body=%s", release.Code, release.Body.String())
	}
	retire := request(http.MethodPost, base+"/retire", map[string]any{"confirm": "RETIRE"})
	if retire.Code != http.StatusOK {
		t.Fatalf("retire status=%d body=%s", retire.Code, retire.Body.String())
	}
	var retired machineRoleView
	if err := json.Unmarshal(retire.Body.Bytes(), &retired); err != nil {
		t.Fatal(err)
	}
	if !retired.Retired || retired.RetiredAt == nil || retired.RoleKey != role.RoleKey {
		t.Fatalf("retired role=%+v", retired)
	}

	assignRetired := request(http.MethodPost, base+"/assignment", map[string]any{"companion_id": companion.ID})
	if assignRetired.Code != http.StatusConflict {
		t.Fatalf("assign retired role status=%d body=%s", assignRetired.Code, assignRetired.Body.String())
	}
	deleteHistorical := request(http.MethodDelete, base+"?confirm=true", nil)
	if deleteHistorical.Code != http.StatusConflict {
		t.Fatalf("delete role with assignment history status=%d body=%s", deleteHistorical.Code, deleteHistorical.Body.String())
	}

	restore := request(http.MethodPost, base+"/restore", nil)
	if restore.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restore.Code, restore.Body.String())
	}
	var restored machineRoleView
	if err := json.Unmarshal(restore.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Retired || restored.RetiredAt != nil || restored.RoleKey != role.RoleKey {
		t.Fatalf("restored role=%+v", restored)
	}

	disposable, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "DISPOSABLE", DisplayName: "Disposable",
		RequiredCapabilities: []string{"local.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	disposableBase := "/api/v1/projects/" + project.ID + "/machine-roles/" + disposable.ID
	if res := request(http.MethodPost, disposableBase+"/retire", map[string]any{"confirm": "RETIRE"}); res.Code != http.StatusOK {
		t.Fatalf("retire disposable status=%d body=%s", res.Code, res.Body.String())
	}
	if res := request(http.MethodDelete, disposableBase+"?confirm=true", nil); res.Code != http.StatusNoContent {
		t.Fatalf("delete disposable status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := stageStore.GetMachineRole(ctx, disposable.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted role lookup err=%v", err)
	}
}
