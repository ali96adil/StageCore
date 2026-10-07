package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/deviceupdate"
	"github.com/ali96adil/StageCore/internal/domain"
)

type stageDeviceSessionValidator interface {
	ValidateRuntimeSession(context.Context, string) (domain.CompanionRuntimeSession, error)
}

// WithStageDeviceFirmwareArtifacts installs the device-only firmware artifact
// download route. The caller must install this option only on the pinned TLS
// device gateway, never on the Operator/browser server.
func WithStageDeviceFirmwareArtifacts(
	auth stageDeviceSessionValidator,
	artifacts *deviceupdate.ArtifactRegistry,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || artifacts == nil {
			return
		}
		s.mux.HandleFunc(
			"GET /api/v1/stage-device-firmware/artifacts/{artifact_id}/firmware.bin",
			func(w http.ResponseWriter, r *http.Request) {
				handleStageDeviceFirmwareArtifact(w, r, auth, artifacts)
			},
		)
	}
}

func handleStageDeviceFirmwareArtifact(
	w http.ResponseWriter,
	r *http.Request,
	auth stageDeviceSessionValidator,
	artifacts *deviceupdate.ArtifactRegistry,
) {
	if !secureDeviceRequest(r) {
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{
			"error_code": "SECURE_TRANSPORT_REQUIRED",
		})
		return
	}

	const prefix = "StageCoreSession "
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) ||
		strings.TrimSpace(strings.TrimPrefix(authorization, prefix)) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error_code": companionauth.CodeSessionInvalid,
		})
		return
	}

	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	session, err := auth.ValidateRuntimeSession(r.Context(), token)
	if err != nil {
		writeCompanionAuthError(w, err)
		return
	}

	artifactID := strings.TrimSpace(r.PathValue("artifact_id"))
	file, metadata, err := artifacts.OpenForDevice(artifactID, session.CompanionID)
	if errors.Is(err, deviceupdate.ErrArtifactNotFound) {
		// Cross-device binding failures intentionally look identical to missing
		// artifacts so one trusted device cannot enumerate another device's
		// firmware inventory.
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
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "firmware.bin"))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", `"sha256:`+metadata.SHA256+`"`)
	w.Header().Set("X-Content-SHA256", metadata.SHA256)
	w.Header().Set("X-Content-Length", int64String(metadata.SizeBytes))
	w.Header().Set("X-StageCore-Artifact-ID", metadata.ArtifactID)
	w.Header().Set("X-StageCore-Source-Revision", metadata.SourceRevision)
	http.ServeContent(w, r, "firmware.bin", metadata.CreatedAt, file)
}
