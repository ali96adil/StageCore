package httpapi

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/projectaccess"
)

type tabletCameraDevices interface {
	GetDevice(context.Context, string) (deviceexperience.Device, error)
}

type tabletCameraRuntime interface {
	CurrentV2ScopeForSession(string, string) (devicechannel.V2RuntimeScope, bool)
}

// WithTabletCameraViewer installs the opt-in viewer on the device TLS gateway.
func WithTabletCameraViewer(auth stageDeviceSessionValidator, devices tabletCameraDevices, runtime tabletCameraRuntime, viewer *ProjectCameraViewer) Option {
	return func(s *Server) {
		if auth == nil || devices == nil || runtime == nil || viewer == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/stage-device-camera/projects/{project_id}/stream", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if r.TLS == nil {
				writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
				return
			}
			session, token, ok := requireDeviceRuntimeSession(w, r, auth)
			if !ok {
				return
			}
			if r.PathValue("project_id") != viewer.projectID {
				writeProjectQualificationError(w, projectaccess.ErrNotFound)
				return
			}
			resolve := func(ctx context.Context) (devicechannel.V2RuntimeScope, error) {
				ctx, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()
				current, err := auth.ValidateRuntimeSession(ctx, token)
				if err != nil || current.ID != session.ID || current.CompanionID != session.CompanionID {
					return devicechannel.V2RuntimeScope{}, projectaccess.ErrDenied
				}
				device, err := devices.GetDevice(ctx, session.CompanionID)
				assignment := device.Assignment
				if err != nil || device.ID != session.CompanionID || !device.Enabled ||
					device.Kind != deviceexperience.DeviceTabletPlayer || device.ProfileID != deviceexperience.TabletPlayerProfileID ||
					device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
					assignment == nil || assignment.DeviceID != device.ID || assignment.State != "ACTIVE" ||
					assignment.ProjectID != viewer.projectID || assignment.RuntimeSnapshotID == "" || assignment.Epoch < 1 {
					return devicechannel.V2RuntimeScope{}, projectaccess.ErrDenied
				}
				scope, ok := runtime.CurrentV2ScopeForSession(device.ID, session.ID)
				if !ok || !scope.CommandsEnabled || scope.ProjectID != assignment.ProjectID ||
					scope.AssignmentEpoch != assignment.Epoch || scope.RuntimeSnapshotID != assignment.RuntimeSnapshotID ||
					!slices.Contains(scope.Capabilities, deviceexperience.CapabilityTabletLiveShow) {
					return devicechannel.V2RuntimeScope{}, projectaccess.ErrDenied
				}
				return scope, nil
			}
			bound, err := resolve(r.Context())
			if !writeProjectQualificationError(w, err) {
				return
			}
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			viewer.stream(w, r, func(ctx context.Context) error {
				current, err := resolve(ctx)
				if err != nil || current.Generation != bound.Generation ||
					current.AssignmentEpoch != bound.AssignmentEpoch || current.RuntimeSnapshotID != bound.RuntimeSnapshotID {
					return projectaccess.ErrDenied
				}
				return nil
			})
		})
	}
}
