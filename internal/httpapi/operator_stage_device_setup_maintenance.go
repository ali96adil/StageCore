package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const setupAPMaintenanceTimeout = 7 * time.Second

func WithOperatorStageDeviceSetupMaintenance(
	auth *userauth.Service,
	devices *deviceexperience.Repository,
	runtime *devicechannel.Runtime,
	stageStore *store.Store,
	audit *securityaudit.Service,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil ||
			runtime == nil || stageStore == nil {
			return
		}

		s.mux.HandleFunc(
			"POST /api/v1/stage-devices/{device_id}/setup-ap-password",
			withPermission(auth, userauth.PermissionDeviceMaintenanceManage, func(
				w http.ResponseWriter,
				r *http.Request,
				session userauth.Session,
			) {
				deviceID := strings.TrimSpace(r.PathValue("device_id"))
				var input struct {
					Password       string `json:"password"`
					ResetToDefault bool   `json:"reset_to_default"`
				}
				if !decodeBoundedJSON(w, r, &input) {
					return
				}

				operation := devicechannel.SetupAPPasswordSet
				password := input.Password
				if input.ResetToDefault {
					if password != "" {
						writeJSON(w, http.StatusBadRequest, map[string]any{
							"error_code": "STAGE_DEVICE_SETUP_AP_INPUT_INVALID",
							"detail": "reset_to_default cannot include a password",
						})
						return
					}
					operation = devicechannel.SetupAPPasswordResetDefault
				} else if len(password) < 8 || len(password) > 63 {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error_code": "STAGE_DEVICE_SETUP_AP_PASSWORD_INVALID",
						"detail": "Setup AP password must be 8-63 bytes",
					})
					return
				}

				active, err := stageStore.ActiveOperationalSessionType(r.Context())
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_SETUP_AP_SESSION_CHECK_FAILED",
					})
					return
				}
				if active != "" {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_SETUP_AP_SESSION_ACTIVE",
						"session_type": active,
						"detail": "Device maintenance is unavailable while an operational Session is active.",
					})
					return
				}

				device, err := devices.GetDevice(r.Context(), deviceID)
				if err != nil || !device.Enabled ||
					device.ProtocolVersion != deviceexperience.ProtocolVersion2 {
					writeJSON(w, http.StatusNotFound, map[string]any{
						"error_code": "STAGE_DEVICE_SETUP_AP_DEVICE_NOT_FOUND",
					})
					return
				}
				supported := false
				for _, capability := range device.Capabilities {
					if capability == devicechannel.SetupAPPasswordCapability {
						supported = true
						break
					}
				}
				if !supported {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_SETUP_AP_CAPABILITY_MISSING",
					})
					return
				}

				requestID, err := stageid.New()
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_SETUP_AP_REQUEST_ID_FAILED",
					})
					return
				}

				ctx, cancel := context.WithTimeout(r.Context(), setupAPMaintenanceTimeout)
				defer cancel()
				result, err := runtime.ConfigureSetupAPPassword(
					ctx, device.ID, requestID, operation, password,
				)
				if err != nil {
					status := http.StatusConflict
					code := "STAGE_DEVICE_SETUP_AP_REJECTED"
					switch {
					case errors.Is(err, devicechannel.ErrSetupAPMaintenanceOffline):
						code = "STAGE_DEVICE_SETUP_AP_DEVICE_OFFLINE"
					case errors.Is(err, devicechannel.ErrSetupAPMaintenanceProtocol):
						code = "STAGE_DEVICE_SETUP_AP_PROTOCOL_REQUIRED"
					case errors.Is(err, devicechannel.ErrSetupAPMaintenanceCapabilityMissing):
						code = "STAGE_DEVICE_SETUP_AP_CAPABILITY_MISSING"
					case errors.Is(err, devicechannel.ErrSetupAPMaintenanceInput):
						status = http.StatusBadRequest
						code = "STAGE_DEVICE_SETUP_AP_INPUT_INVALID"
					case errors.Is(err, context.DeadlineExceeded):
						status = http.StatusGatewayTimeout
						code = "STAGE_DEVICE_SETUP_AP_ACK_TIMEOUT"
					}
					writeJSON(w, status, map[string]any{
						"error_code": code,
						"detail": err.Error(),
					})
					return
				}

				appendAudit(r, audit, securityaudit.Event{
					EventType: "stage_device.setup_ap_password.updated",
					ActorUserID: session.User.ID,
					ActorUsername: session.User.Username,
					ResourceType: "stage_device",
					ResourceID: device.ID,
					Result: securityaudit.ResultSuccess,
					Metadata: map[string]any{
						"device_id": device.ID,
						"operation": operation,
						"request_id": requestID,
						"connection_generation": result.ConnectionGeneration,
					},
				})

				writeJSON(w, http.StatusOK, map[string]any{
					"device_id": device.ID,
					"request_id": requestID,
					"operation": operation,
					"maintenance_state": result.MaintenanceState,
					"detail": result.Detail,
				})
			}),
		)
	}
}
