package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/livereconcile"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// lightingLiveDiagnosticReader deliberately exposes only an informational
// read. No device dispatcher or output command is reachable from this route.
type lightingLiveDiagnosticReader interface {
	Read(context.Context, string, string) livereconcile.BlockedSoftwareDiagnostic
}

var _ lightingLiveDiagnosticReader = (*livereconcile.BlockedDiagnosticReader)(nil)

// WithOperatorLightingLiveDiagnostics exposes a strictly read-only,
// authenticated operator diagnostic for the experimental BLOCKED v2 node.
// UNKNOWN is an ordinary diagnostic result, never an optimistic 200/READY.
func WithOperatorLightingLiveDiagnostics(
	auth *userauth.Service, stageStore *store.Store,
	reader lightingLiveDiagnosticReader,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || stageStore == nil || reader == nil {
			return
		}
		s.mux.HandleFunc(
			"GET /api/v1/projects/{project_id}/lighting-controller/nodes/{device_id}/live-diagnostic",
			withPermission(auth, userauth.PermissionProjectRead,
				func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
					w.Header().Set("Cache-Control", "no-store")
					projectID := strings.TrimSpace(r.PathValue("project_id"))
					deviceID := strings.TrimSpace(r.PathValue("device_id"))
					if projectID == "" || deviceID == "" {
						writeJSON(w, http.StatusBadRequest, map[string]any{
							"error": "LIGHTING_DIAGNOSTIC_SCOPE_REQUIRED",
						})
						return
					}
					if _, err := stageStore.GetProject(r.Context(), projectID); err != nil {
						writeProjectStoreError(w, err)
						return
					}
					diagnostic := reader.Read(r.Context(), projectID, deviceID)
					advice := livereconcile.AdviseBlockedRecovery(diagnostic)
					// Keep UNKNOWN free of stale channel data and identity
					// fields even if a future reader accidentally populates
					// them before a failing final-scope recheck.
					if diagnostic.Status == livereconcile.SoftwareDiagnosticUnknown {
						diagnostic = livereconcile.BlockedSoftwareDiagnostic{
							Status: livereconcile.SoftwareDiagnosticUnknown,
							Reason: diagnostic.Reason,
							PhysicalVerified: false, CommandsEnabled: false,
						}
					}
					writeJSON(w, http.StatusOK, map[string]any{
						"schema_version": 1,
						"source": "SOFTWARE_ONLY",
						"device_id": deviceID,
						"diagnostic": map[string]any{
							"status": diagnostic.Status,
							"reason": diagnostic.Reason,
							"project_id": diagnostic.ProjectID,
							"session_id": diagnostic.SessionID,
							"snapshot_id": diagnostic.SnapshotID,
							"cue_id": diagnostic.CueID,
							"cue_execution_id": diagnostic.CueExecutionID,
							"assignment_epoch": diagnostic.AssignmentEpoch,
							"connection_generation": diagnostic.ConnectionGeneration,
							"desired_slots": diagnostic.DesiredSlots,
							"reported_slots": diagnostic.ReportedSlots,
							"differing_slots": diagnostic.DifferingSlots,
							"physical_output_verified": false,
							"commands_enabled": false,
						},
						"recovery_advice": advice,
					})
				},
			),
		)
	}
}
