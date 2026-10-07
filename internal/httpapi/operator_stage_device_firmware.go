package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/deviceupdate"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const operatorFirmwareManifestTTL = 5 * time.Minute

func WithOperatorStageDeviceFirmware(
	auth *userauth.Service,
	devices *deviceexperience.Repository,
	artifacts *deviceupdate.ArtifactRegistry,
	updates *deviceupdate.LifecycleStore,
	runtime *devicechannel.Runtime,
	stageStore *store.Store,
	audit *securityaudit.Service,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil ||
			artifacts == nil || updates == nil || runtime == nil || stageStore == nil {
			return
		}

		s.mux.HandleFunc(
			"GET /api/v1/stage-devices/{device_id}/firmware-artifacts",
			withPermission(auth, userauth.PermissionDeviceFirmwareManage, func(
				w http.ResponseWriter,
				r *http.Request,
				_ userauth.Session,
			) {
				deviceID := strings.TrimSpace(r.PathValue("device_id"))
				device, err := devices.GetDevice(r.Context(), deviceID)
				if err != nil || !firmwareMaintenanceDeviceEligible(device) {
					writeJSON(w, http.StatusNotFound, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_DEVICE_NOT_FOUND",
					})
					return
				}

				items, err := artifacts.ListForDevice(device.ID)
				if err != nil && !errors.Is(err, deviceupdate.ErrArtifactNotFound) {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_ARTIFACTS_UNAVAILABLE",
					})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"device_id": device.ID,
					"profile_id": device.ProfileID,
					"current_version": device.ClientVersion,
					"artifacts": items,
				})
			}),
		)

		s.mux.HandleFunc(
			"POST /api/v1/stage-devices/{device_id}/firmware-updates",
			withPermission(auth, userauth.PermissionDeviceFirmwareManage, func(
				w http.ResponseWriter,
				r *http.Request,
				session userauth.Session,
			) {
				deviceID := strings.TrimSpace(r.PathValue("device_id"))
				var input struct {
					ArtifactID string `json:"artifact_id"`
				}
				if !decodeBoundedJSON(w, r, &input) {
					return
				}
				input.ArtifactID = strings.TrimSpace(input.ArtifactID)
				if deviceID == "" || input.ArtifactID == "" {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_INPUT_REQUIRED",
					})
					return
				}

				active, err := stageStore.ActiveOperationalSessionType(r.Context())
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_SESSION_CHECK_FAILED",
					})
					return
				}
				if active != "" {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_SESSION_ACTIVE",
						"session_type": active,
						"detail": "Firmware maintenance is unavailable while any operational Session is active.",
					})
					return
				}

				device, err := devices.GetDevice(r.Context(), deviceID)
				if err != nil || !firmwareMaintenanceDeviceEligible(device) {
					writeJSON(w, http.StatusNotFound, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_DEVICE_NOT_FOUND",
					})
					return
				}
				if err := firmwareMaintenanceStateReady(device); err != nil {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_NOT_SAFE",
						"detail": err.Error(),
					})
					return
				}

				metadata, err := artifacts.InspectForDevice(input.ArtifactID, device.ID)
				if errors.Is(err, deviceupdate.ErrArtifactNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_ARTIFACT_NOT_FOUND",
					})
					return
				}
				if errors.Is(err, deviceupdate.ErrArtifactCorrupt) {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_ARTIFACT_CORRUPT",
					})
					return
				}
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_ARTIFACT_UNAVAILABLE",
					})
					return
				}

				now := time.Now().UTC()
				manifest, err := buildStageDeviceFirmwareManifest(device, metadata, now)
				if err != nil {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_MANIFEST_REJECTED",
						"detail": err.Error(),
					})
					return
				}

				record, err := updates.Issue(r.Context(), manifest, session.User.ID)
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_PERSIST_FAILED",
						"detail": err.Error(),
					})
					return
				}

				appendAudit(r, audit, securityaudit.Event{
					EventType: "stage_device.firmware_update.issued",
					ActorUserID: session.User.ID,
					ActorUsername: session.User.Username,
					ResourceType: "stage_device_firmware_update",
					ResourceID: manifest.UpdateID,
					Result: securityaudit.ResultSuccess,
					Metadata: map[string]any{
						"device_id": manifest.DeviceID,
						"profile_id": manifest.ProfileID,
						"current_version": manifest.CurrentVersion,
						"target_version": manifest.TargetVersion,
						"source_revision": manifest.SourceRevision,
						"artifact_path": manifest.ArtifactPath,
						"expires_at": manifest.ExpiresAt.Format(time.RFC3339Nano),
						"delivery_state": "NOT_SENT",
					},
				})

				writeJSON(w, http.StatusCreated, map[string]any{
					"manifest": manifest,
					"update": record,
					"delivery_state": "NOT_SENT",
					"detail": "Manifest is persisted but not sent. Delivery requires the explicit maintenance send action.",
				})
			}),
		)

		s.mux.HandleFunc(
			"GET /api/v1/stage-devices/{device_id}/firmware-updates/{update_id}",
			withPermission(auth, userauth.PermissionDeviceFirmwareManage, func(
				w http.ResponseWriter,
				r *http.Request,
				_ userauth.Session,
			) {
				deviceID := strings.TrimSpace(r.PathValue("device_id"))
				updateID := strings.TrimSpace(r.PathValue("update_id"))
				record, err := updates.Get(r.Context(), updateID)
				if errors.Is(err, deviceupdate.ErrUpdateNotFound) ||
					(err == nil && record.Manifest.DeviceID != deviceID) {
					writeJSON(w, http.StatusNotFound, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_NOT_FOUND",
					})
					return
				}
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_UNAVAILABLE",
					})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"update": record})
			}),
		)

		s.mux.HandleFunc(
			"POST /api/v1/stage-devices/{device_id}/firmware-updates/{update_id}/send",
			withPermission(auth, userauth.PermissionDeviceFirmwareManage, func(
				w http.ResponseWriter,
				r *http.Request,
				session userauth.Session,
			) {
				deviceID := strings.TrimSpace(r.PathValue("device_id"))
				updateID := strings.TrimSpace(r.PathValue("update_id"))
				if deviceID == "" || updateID == "" {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_INPUT_REQUIRED",
					})
					return
				}

				active, err := stageStore.ActiveOperationalSessionType(r.Context())
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_SESSION_CHECK_FAILED",
					})
					return
				}
				if active != "" {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_SESSION_ACTIVE",
						"session_type": active,
					})
					return
				}

				record, err := updates.Get(r.Context(), updateID)
				if errors.Is(err, deviceupdate.ErrUpdateNotFound) ||
					(err == nil && record.Manifest.DeviceID != deviceID) {
					writeJSON(w, http.StatusNotFound, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_NOT_FOUND",
					})
					return
				}
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_UNAVAILABLE",
					})
					return
				}
				if record.State != deviceupdate.UpdateIssued {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_UPDATE_STATE_CONFLICT",
						"state": record.State,
					})
					return
				}

				device, err := devices.GetDevice(r.Context(), deviceID)
				if err != nil || !firmwareMaintenanceDeviceEligible(device) {
					writeJSON(w, http.StatusNotFound, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_DEVICE_NOT_FOUND",
					})
					return
				}
				if err := firmwareMaintenanceStateReady(device); err != nil {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_NOT_SAFE",
						"detail": err.Error(),
					})
					return
				}
				if record.Manifest.ProfileID != device.ProfileID ||
					record.Manifest.CurrentVersion != device.ClientVersion {
					writeJSON(w, http.StatusConflict, map[string]any{
						"error_code": "STAGE_DEVICE_FIRMWARE_DEVICE_CHANGED",
						"detail": "Device profile or firmware version changed after manifest issuance.",
					})
					return
				}

				sent, err := runtime.SendFirmwareMaintenance(r.Context(), updateID)
				if err != nil {
					status := http.StatusConflict
					code := "STAGE_DEVICE_FIRMWARE_SEND_REJECTED"
					switch {
					case errors.Is(err, devicechannel.ErrFirmwareMaintenanceOffline):
						code = "STAGE_DEVICE_FIRMWARE_DEVICE_OFFLINE"
					case errors.Is(err, devicechannel.ErrFirmwareMaintenanceProtocol):
						code = "STAGE_DEVICE_FIRMWARE_PROTOCOL_REQUIRED"
					case errors.Is(err, devicechannel.ErrFirmwareMaintenanceCapabilityMissing):
						code = "STAGE_DEVICE_FIRMWARE_CAPABILITY_MISSING"
					case errors.Is(err, deviceupdate.ErrUpdateExpired):
						code = "STAGE_DEVICE_FIRMWARE_UPDATE_EXPIRED"
					case errors.Is(err, deviceupdate.ErrUpdateState):
						code = "STAGE_DEVICE_FIRMWARE_UPDATE_STATE_CONFLICT"
					}
					writeJSON(w, status, map[string]any{
						"error_code": code,
						"detail": err.Error(),
					})
					return
				}

				appendAudit(r, audit, securityaudit.Event{
					EventType: "stage_device.firmware_update.sent",
					ActorUserID: session.User.ID,
					ActorUsername: session.User.Username,
					ResourceType: "stage_device_firmware_update",
					ResourceID: sent.Manifest.UpdateID,
					Result: securityaudit.ResultSuccess,
					Metadata: map[string]any{
						"device_id": sent.Manifest.DeviceID,
						"connection_generation": sent.ConnectionGeneration,
						"state": sent.State,
					},
				})
				writeJSON(w, http.StatusAccepted, map[string]any{
					"update": sent,
					"detail": "Maintenance request sent on the exact authenticated v2 connection.",
				})
			}),
		)
	}
}

func firmwareMaintenanceDeviceEligible(device deviceexperience.Device) bool {
	return device.Enabled &&
		device.ProtocolVersion == deviceexperience.ProtocolVersion2 &&
		strings.TrimSpace(device.ID) != "" &&
		strings.TrimSpace(device.ProfileID) != "" &&
		strings.TrimSpace(device.ClientVersion) != ""
}

func firmwareMaintenanceStateReady(device deviceexperience.Device) error {
	if device.Runtime == nil ||
		device.Runtime.Connection != deviceexperience.ConnectionOnline ||
		device.Runtime.Readiness != deviceexperience.ReadinessReady {
		return fmt.Errorf("device must be ONLINE and READY on its authenticated runtime")
	}

	if device.ProfileID != stagelaser.ProfileID {
		return nil
	}

	var observation stagelaser.Observation
	if err := json.Unmarshal(device.Runtime.ObservedState, &observation); err != nil {
		return fmt.Errorf("StageLaser observation is unavailable")
	}
	if err := stagelaser.ValidateObservation(observation); err != nil {
		return fmt.Errorf("StageLaser observation is invalid: %w", err)
	}
	if observation.ArmState != stagelaser.ArmDisarmed {
		return fmt.Errorf("StageLaser must be DISARMED")
	}
	if observation.LogicalState != stagelaser.StateOff {
		return fmt.Errorf("StageLaser must already be stable OFF")
	}
	if observation.StateQuality != stagelaser.StateQualityTracked &&
		observation.StateQuality != stagelaser.StateQualityConfirmed {
		return fmt.Errorf("StageLaser OFF state must be TRACKED or CONFIRMED")
	}
	if observation.ResyncRequired {
		return fmt.Errorf("StageLaser must not require state resync")
	}
	if observation.PulseInProgress {
		return fmt.Errorf("StageLaser relay pulse must not be in progress")
	}
	if observation.ActiveFlash != nil {
		return fmt.Errorf("StageLaser local Flash must not be active")
	}
	return nil
}

func buildStageDeviceFirmwareManifest(
	device deviceexperience.Device,
	metadata deviceupdate.ArtifactMetadata,
	now time.Time,
) (deviceupdate.Manifest, error) {
	updateID, err := stageid.New()
	if err != nil {
		return deviceupdate.Manifest{}, err
	}
	manifest := deviceupdate.Manifest{
		SchemaVersion: deviceupdate.SchemaVersion1,
		UpdateID: updateID,
		DeviceID: device.ID,
		ProfileID: device.ProfileID,
		CurrentVersion: device.ClientVersion,
		TargetVersion: metadata.Version,
		SourceRevision: metadata.SourceRevision,
		Qualification: metadata.Qualification,
		ArtifactPath: deviceupdate.ArtifactPath(metadata.ArtifactID),
		ArtifactSize: metadata.SizeBytes,
		ArtifactSHA256: metadata.SHA256,
		IssuedAt: now.UTC(),
		ExpiresAt: now.UTC().Add(operatorFirmwareManifestTTL),
		RollbackRequired: device.ProfileID == stagelaser.ProfileID,
	}
	if metadata.DeviceID != device.ID || metadata.ProfileID != device.ProfileID {
		return deviceupdate.Manifest{}, fmt.Errorf("artifact target does not match the selected device/profile")
	}
	if err := deviceupdate.ValidateManifest(manifest, deviceupdate.ValidationContext{
		ExpectedDeviceID: device.ID,
		ExpectedProfileID: device.ProfileID,
		Now: now.UTC(),
		RequireRollback: device.ProfileID == stagelaser.ProfileID,
	}); err != nil {
		return deviceupdate.Manifest{}, err
	}
	return manifest, nil
}
