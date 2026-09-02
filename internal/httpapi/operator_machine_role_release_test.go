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

func TestOperatorMachineRoleAssignmentReleaseAllowsReassignment(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorMachineRoles(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Role Release Test", CreatedBy: owner.Session.User.ID,
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

	first, err := stageStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		CompanionID: "11111111-1111-4111-8111-111111111111",
		DisplayName: "Old Mac", Platform: "macos", Architecture: "arm64",
		Version: "0.1.0", Capabilities: []string{"local.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := stageStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		CompanionID: "22222222-2222-4222-8222-222222222222",
		DisplayName: "New Mac", Platform: "macos", Architecture: "arm64",
		Version: "0.1.0", Capabilities: []string{"local.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, companionID := range []string{first.ID, second.ID} {
		if err := stageStore.SetCompanionTrustState(ctx, companionID, domain.CompanionTrusted); err != nil {
			t.Fatal(err)
		}
	}

	firstAssignment, err := stageStore.AssignMachineRole(ctx, role.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}

	assignmentPath := "/api/v1/projects/" + project.ID + "/machine-roles/" + role.ID + "/assignment"
	releaseReq := httptest.NewRequest(http.MethodDelete, assignmentPath, nil)
	releaseReq.RemoteAddr = "127.0.0.1:13300"
	releaseReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	releaseReq.Header.Set(csrfHeader, owner.CSRFToken)
	releaseRes := httptest.NewRecorder()
	handler.ServeHTTP(releaseRes, releaseReq)
	if releaseRes.Code != http.StatusNoContent {
		t.Fatalf("release assignment status=%d body=%s", releaseRes.Code, releaseRes.Body.String())
	}

	released, err := stageStore.GetRoleAssignment(ctx, firstAssignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if released.State != domain.RoleReleased {
		t.Fatalf("released assignment state=%s, want %s", released.State, domain.RoleReleased)
	}

	assignmentBody, _ := json.Marshal(map[string]string{"companion_id": second.ID})
	assignReq := httptest.NewRequest(http.MethodPost, assignmentPath, bytes.NewReader(assignmentBody))
	assignReq.RemoteAddr = "127.0.0.1:13301"
	assignReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	assignReq.Header.Set(csrfHeader, owner.CSRFToken)
	assignRes := httptest.NewRecorder()
	handler.ServeHTTP(assignRes, assignReq)
	if assignRes.Code != http.StatusCreated {
		t.Fatalf("replacement assignment status=%d body=%s", assignRes.Code, assignRes.Body.String())
	}
	var replacement roleAssignmentView
	if err := json.Unmarshal(assignRes.Body.Bytes(), &replacement); err != nil {
		t.Fatal(err)
	}
	if replacement.CompanionID != second.ID || replacement.MachineRoleID != role.ID || replacement.State != domain.RoleAssigned {
		t.Fatalf("unexpected replacement assignment: %+v", replacement)
	}
}

func TestOperatorMachineRoleAssignmentReleaseRejectsCrossProjectRole(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorMachineRoles(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	firstProject, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "First", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	secondProject, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Second", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, firstProject.ID, store.CreateMachineRoleParams{RoleKey: "VIDEO-MAIN"})
	if err != nil {
		t.Fatal(err)
	}
	companion, err := stageStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		CompanionID: "33333333-3333-4333-8333-333333333333",
		DisplayName: "Video Mac", Platform: "macos", Architecture: "arm64",
		Version: "0.1.0", Capabilities: []string{"local.echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetCompanionTrustState(ctx, companion.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	assignment, err := stageStore.AssignMachineRole(ctx, role.ID, companion.ID)
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/projects/" + secondProject.ID + "/machine-roles/" + role.ID + "/assignment"
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.RemoteAddr = "127.0.0.1:13400"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	req.Header.Set(csrfHeader, owner.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("cross-project release status=%d body=%s", res.Code, res.Body.String())
	}

	stillActive, err := stageStore.GetRoleAssignment(ctx, assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillActive.State == domain.RoleReleased {
		t.Fatal("cross-project release mutated the assignment")
	}
}
