package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const maxExecutionEnvironmentOperationIDBytes = 128

type executionEnvironmentOperationRuntime interface {
	OperateExecutionEnvironment(context.Context, companionchannel.EnvironmentOperationRequest) companionchannel.EnvironmentOperationResult
}

type executionEnvironmentOperationRequest struct {
	OperationID string                                    `json:"operation_id"`
	Kind        companionchannel.EnvironmentOperationKind `json:"kind"`
	TimeoutMS   int64                                     `json:"timeout_ms"`
}

type executionEnvironmentOperationView struct {
	OperationID     string                                               `json:"operation_id"`
	Kind            companionchannel.EnvironmentOperationKind            `json:"kind"`
	Status          companionchannel.EnvironmentOperationStatus          `json:"status"`
	ErrorCode       string                                               `json:"error_code,omitempty"`
	ResponseSummary string                                               `json:"response_summary,omitempty"`
	Snapshot        *executionenv.Snapshot                               `json:"snapshot,omitempty"`
	CaptureObject   *companionchannel.EnvironmentCaptureObjectDescriptor `json:"capture_object,omitempty"`
	CaptureUpload   *companionchannel.EnvironmentCaptureUploadReceipt    `json:"capture_upload,omitempty"`
}

// WithOperatorExecutionEnvironmentOperations exposes only the bounded F-025
// operation kinds. The browser cannot supply a Companion capability, manifest,
// Machine Role, Runtime Snapshot, adapter parameters, or command payload.
func WithOperatorExecutionEnvironmentOperations(auth *userauth.Service, stageStore *store.Store, runtime executionEnvironmentOperationRuntime) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil || runtime == nil {
			return
		}
		registerOperatorExecutionEnvironmentOperationRoutes(s.mux, auth, stageStore, runtime)
		if discovery, ok := runtime.(oscQueryDiscoveryRuntime); ok {
			registerOperatorOSCQueryDiscoveryRoute(s.mux, auth, stageStore, discovery)
		}
	}
}

func registerOperatorExecutionEnvironmentOperationRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, runtime executionEnvironmentOperationRuntime) {
	path := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/operations"
	mux.HandleFunc("POST "+path, withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
		environment, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_OPERATION_FAILED")
			return
		}
		if environment.RevisionID != revision.ID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND"})
			return
		}

		var body executionEnvironmentOperationRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		body.OperationID = strings.TrimSpace(body.OperationID)
		if body.OperationID == "" || len(body.OperationID) > maxExecutionEnvironmentOperationIDBytes {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_OPERATION_ID_INVALID"})
			return
		}
		switch body.Kind {
		case companionchannel.EnvironmentOperationOpen,
			companionchannel.EnvironmentOperationReconnect,
			companionchannel.EnvironmentOperationCaptureSnapshot,
			companionchannel.EnvironmentOperationRestoreObservableState:
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_OPERATION_KIND_INVALID"})
			return
		}
		if body.TimeoutMS < 0 || body.TimeoutMS > 30_000 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_OPERATION_TIMEOUT_INVALID"})
			return
		}

		if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionRuntimeControl) {
			return
		}
		result := runtime.OperateExecutionEnvironment(r.Context(), companionchannel.EnvironmentOperationRequest{
			IssuerBrowserSessionID:      session.ID,
			IssuerRequiresProjectGrants: qualificationAccess(r, nil) != nil,
			OperationID:                 body.OperationID,
			EnvironmentManifestID:       environment.ID,
			Kind:                        body.Kind,
			TimeoutMS:                   body.TimeoutMS,
		})
		if !recheckCommittedExecutionEnvironmentOperation(w, r, auth, session) {
			return
		}
		writeJSON(w, http.StatusOK, executionEnvironmentOperationView{
			OperationID:     result.OperationID,
			Kind:            result.Kind,
			Status:          result.Status,
			ErrorCode:       result.ErrorCode,
			ResponseSummary: result.ResponseSummary,
			Snapshot:        result.Snapshot,
			CaptureObject:   result.CaptureObject,
			CaptureUpload:   result.CaptureUpload,
		})
	}))
}

func recheckCommittedExecutionEnvironmentOperation(w http.ResponseWriter, r *http.Request, auth *userauth.Service, session userauth.Session) bool {
	access := qualificationAccess(r, nil)
	err := projectMutationAuthorityError(r, auth, session, userauth.PermissionRuntimeControl, access,
		projectaccess.RehearsalControl, projectaccess.MediaRead)
	if err == nil {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	status, code := http.StatusForbidden, "FORBIDDEN"
	if !errors.Is(err, userauth.ErrForbidden) && !errors.Is(err, projectaccess.ErrDenied) && !errors.Is(err, projectaccess.ErrNotFound) {
		status, code = http.StatusServiceUnavailable, "PROJECT_ACCESS_UNAVAILABLE"
	}
	writeJSON(w, status, map[string]any{
		"error_code":                     code,
		"effect_may_have_been_committed": true,
		"retryable":                      false,
	})
	return false
}
