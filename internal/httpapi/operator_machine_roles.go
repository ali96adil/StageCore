package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type machineRoleCreateRequest struct {
	RoleKey                   string   `json:"role_key"`
	DisplayName               string   `json:"display_name"`
	RequiredCapabilities      []string `json:"required_capabilities"`
	RequiredRuntimeSnapshotID *string  `json:"required_runtime_snapshot_id,omitempty"`
	RequiredConfigHash        string   `json:"required_config_hash,omitempty"`
	Required                  bool     `json:"required"`
}

type roleAssignmentCreateRequest struct {
	CompanionID string `json:"companion_id"`
}

type machineRoleRuntimeRequirementRequest struct {
	RuntimeSnapshotID string `json:"runtime_snapshot_id"`
	ConfigHash        string `json:"config_hash,omitempty"`
}

type machineRoleView struct {
	ID                        string    `json:"machine_role_id"`
	ProjectID                 string    `json:"project_id"`
	RoleKey                   string    `json:"role_key"`
	DisplayName               string    `json:"display_name"`
	RequiredCapabilities      []string  `json:"required_capabilities"`
	RequiredRuntimeSnapshotID *string   `json:"required_runtime_snapshot_id,omitempty"`
	RequiredConfigHash        string    `json:"required_config_hash"`
	Required                  bool      `json:"required"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type roleAssignmentView struct {
	ID              string                     `json:"role_assignment_id"`
	MachineRoleID   string                     `json:"machine_role_id"`
	CompanionID     string                     `json:"companion_id"`
	State           domain.RoleAssignmentState `json:"state"`
	AssignedAt      time.Time                  `json:"assigned_at"`
	LastEvaluatedAt time.Time                  `json:"last_evaluated_at"`
}

func WithOperatorMachineRoles(auth *userauth.Service, stageStore *store.Store) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil {
			return
		}
		registerOperatorMachineRoleRoutes(s.mux, auth, stageStore)
	}
}

func registerOperatorMachineRoleRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store) {
	mux.HandleFunc("POST /api/v1/projects/{project_id}/machine-roles", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		var body machineRoleCreateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		role, err := stageStore.CreateMachineRole(r.Context(), projectID, store.CreateMachineRoleParams{
			RoleKey: strings.TrimSpace(body.RoleKey), DisplayName: strings.TrimSpace(body.DisplayName),
			RequiredCapabilities: body.RequiredCapabilities,
			RequiredRuntimeSnapshotID: body.RequiredRuntimeSnapshotID,
			RequiredConfigHash: strings.TrimSpace(body.RequiredConfigHash),
			Required: body.Required,
		})
		if err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_CREATE_FAILED")
			return
		}
		writeJSON(w, http.StatusCreated, makeMachineRoleView(role))
	}))

	mux.HandleFunc("PUT /api/v1/projects/{project_id}/machine-roles/{machine_role_id}/runtime-requirement", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		roleID := strings.TrimSpace(r.PathValue("machine_role_id"))
		role, err := stageStore.GetMachineRole(r.Context(), roleID)
		if err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_REQUIREMENT_FAILED")
			return
		}
		if role.ProjectID != projectID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MACHINE_ROLE_NOT_FOUND"})
			return
		}
		var body machineRoleRuntimeRequirementRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		if err := stageStore.SetMachineRoleRuntimeRequirement(
			r.Context(), roleID, strings.TrimSpace(body.RuntimeSnapshotID), strings.TrimSpace(body.ConfigHash),
		); err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_REQUIREMENT_FAILED")
			return
		}
		updated, err := stageStore.GetMachineRole(r.Context(), roleID)
		if err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_REQUIREMENT_FAILED")
			return
		}
		writeJSON(w, http.StatusOK, makeMachineRoleView(updated))
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/machine-roles/{machine_role_id}/assignment", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		roleID := strings.TrimSpace(r.PathValue("machine_role_id"))
		role, err := stageStore.GetMachineRole(r.Context(), roleID)
		if err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_ASSIGN_FAILED")
			return
		}
		if role.ProjectID != projectID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MACHINE_ROLE_NOT_FOUND"})
			return
		}
		var body roleAssignmentCreateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		assignment, err := stageStore.AssignMachineRole(r.Context(), roleID, strings.TrimSpace(body.CompanionID))
		if err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_ASSIGN_FAILED")
			return
		}
		writeJSON(w, http.StatusCreated, makeRoleAssignmentView(assignment))
	}))

	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/machine-roles/{machine_role_id}/assignment", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		roleID := strings.TrimSpace(r.PathValue("machine_role_id"))
		role, err := stageStore.GetMachineRole(r.Context(), roleID)
		if err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_RELEASE_FAILED")
			return
		}
		if role.ProjectID != projectID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MACHINE_ROLE_NOT_FOUND"})
			return
		}
		assignment, err := stageStore.GetActiveRoleAssignment(r.Context(), roleID)
		if err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_RELEASE_FAILED")
			return
		}
		if err := stageStore.ReleaseRoleAssignment(r.Context(), assignment.ID); err != nil {
			writeMachineRoleStoreError(w, err, "MACHINE_ROLE_RELEASE_FAILED")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func writeMachineRoleStoreError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "MACHINE_ROLE_INVALID"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MACHINE_ROLE_NOT_FOUND"})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "MACHINE_ROLE_CONFLICT"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": fallback})
	}
}

func makeMachineRoleView(role domain.MachineRole) machineRoleView {
	return machineRoleView{
		ID: role.ID, ProjectID: role.ProjectID, RoleKey: role.RoleKey, DisplayName: role.DisplayName,
		RequiredCapabilities: role.RequiredCapabilities,
		RequiredRuntimeSnapshotID: role.RequiredRuntimeSnapshotID,
		RequiredConfigHash: role.RequiredConfigHash, Required: role.Required,
		CreatedAt: role.CreatedAt, UpdatedAt: role.UpdatedAt,
	}
}

func makeRoleAssignmentView(assignment domain.RoleAssignment) roleAssignmentView {
	return roleAssignmentView{
		ID: assignment.ID, MachineRoleID: assignment.MachineRoleID,
		CompanionID: assignment.CompanionID, State: assignment.State,
		AssignedAt: assignment.AssignedAt, LastEvaluatedAt: assignment.LastEvaluatedAt,
	}
}
