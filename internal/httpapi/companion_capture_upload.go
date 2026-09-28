package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	stagevault "github.com/ali96adil/StageCore/internal/vault"
)

const companionCaptureUploadCredentialHeader = "X-StageCore-Upload-Credential"

func WithCompanionCaptureUpload(
	auth *companionauth.Service,
	stageStore *store.Store,
	v *stagevault.Vault,
	manager *bulk.Manager,
) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil || v == nil || manager == nil {
			return
		}
		s.mux.HandleFunc(
			"PUT /api/v1/companion/capture-uploads/{ticket_id}",
			func(w http.ResponseWriter, r *http.Request) {
				handleCompanionCaptureUpload(w, r, auth, stageStore, v, manager)
			},
		)
	}
}

func handleCompanionCaptureUpload(
	w http.ResponseWriter,
	r *http.Request,
	auth *companionauth.Service,
	stageStore *store.Store,
	v *stagevault.Vault,
	manager *bulk.Manager,
) {
	if !secureDeviceRequest(r) {
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
		return
	}

	session, ok := companionCaptureUploadRuntimeSession(w, r, auth)
	if !ok {
		return
	}

	uploadCredential := strings.TrimSpace(r.Header.Get(companionCaptureUploadCredentialHeader))
	if uploadCredential == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": "CAPTURE_UPLOAD_TICKET_INVALID"})
		return
	}
	ticket, err := stageStore.AuthorizeCompanionUploadTicket(r.Context(), uploadCredential)
	if err != nil {
		writeCompanionCaptureUploadTicketError(w, err)
		return
	}
	if strings.TrimSpace(r.PathValue("ticket_id")) != ticket.ID ||
		ticket.RuntimeSessionID != session.ID ||
		ticket.CompanionID != session.CompanionID ||
		ticket.Purpose != store.CompanionUploadExecutionEnvironmentCapture {
		writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "CAPTURE_UPLOAD_SCOPE_MISMATCH"})
		return
	}

	if r.ContentLength < 0 {
		writeJSON(w, http.StatusLengthRequired, map[string]any{"error_code": "CAPTURE_UPLOAD_CONTENT_LENGTH_REQUIRED"})
		return
	}
	if r.ContentLength != ticket.ExpectedSizeBytes {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error_code": "CAPTURE_UPLOAD_LENGTH_MISMATCH",
			"expected_size_bytes": ticket.ExpectedSizeBytes,
		})
		return
	}

	jobID, err := manager.Begin(r.Context(), bulk.KindCaptureUpload, ticket.ExpectedSizeBytes)
	if errors.Is(err, bulk.ErrShowBlocked) {
		writeJSON(w, http.StatusLocked, map[string]any{"error_code": "BULK_TRANSFER_BLOCKED_SHOW"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "BULK_TRANSFER_UNAVAILABLE"})
		return
	}

	reader := bulk.NewGuardedReader(r.Context(), manager, jobID, r.Body)
	object, err := v.ImportVerifiedObject(
		r.Context(),
		ticket.ExpectedContentHash,
		ticket.ExpectedSizeBytes,
		reader,
	)
	if err != nil {
		if r.Context().Err() != nil {
			manager.Cancel(jobID, r.Context().Err().Error())
		} else {
			manager.Fail(jobID, "verified capture upload rejected")
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error_code": "CAPTURE_UPLOAD_CONTENT_REJECTED"})
		return
	}

	completed, err := stageStore.CompleteCompanionUploadTicket(
		r.Context(), ticket.ID, object.ContentHash, object.SizeBytes,
	)
	if err != nil {
		manager.Fail(jobID, "capture upload authority changed before completion")
		_, _ = v.RemoveObjectIfUnreferenced(r.Context(), object.ContentHash)
		if errors.Is(err, domain.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CAPTURE_UPLOAD_COMPLETION_REJECTED"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CAPTURE_UPLOAD_COMPLETION_UNAVAILABLE"})
		return
	}

	manager.Complete(jobID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket_id": completed.ID,
		"status": completed.Status,
		"content_hash": object.ContentHash,
		"size_bytes": object.SizeBytes,
	})
}

func companionCaptureUploadRuntimeSession(
	w http.ResponseWriter,
	r *http.Request,
	auth *companionauth.Service,
) (domain.CompanionRuntimeSession, bool) {
	const prefix = "StageCoreSession "
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) ||
		strings.TrimSpace(strings.TrimPrefix(authorization, prefix)) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": companionauth.CodeSessionInvalid})
		return domain.CompanionRuntimeSession{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	session, err := auth.ValidateRuntimeSession(r.Context(), token)
	if err != nil {
		writeCompanionAuthError(w, err)
		return domain.CompanionRuntimeSession{}, false
	}
	return session, true
}

func writeCompanionCaptureUploadTicketError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": "CAPTURE_UPLOAD_TICKET_INVALID"})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CAPTURE_UPLOAD_TICKET_NOT_ACTIVE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CAPTURE_UPLOAD_TICKET_UNAVAILABLE"})
	}
}
