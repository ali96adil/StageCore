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
