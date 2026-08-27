package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Server) handleVaultObject(w http.ResponseWriter, r *http.Request) {
	if !secureDeviceRequest(r) {
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
		return
	}
	if !s.requireRuntimeSession(w, r) {
		return
	}

	contentHash := strings.ToLower(strings.TrimSpace(r.PathValue("content_hash")))
	file, object, err := s.vault.OpenObject(r.Context(), contentHash)
	if errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "VAULT_OBJECT_NOT_FOUND"})
		return
	}
	if err != nil {
		status := http.StatusConflict
		if len(contentHash) != 64 {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]any{"error_code": "VAULT_OBJECT_UNAVAILABLE"})
		return
	}
	defer file.Close()

	content := interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	}(file)
	jobID := ""
	if s.bulk != nil {
		jobID, err = s.bulk.Begin(r.Context(), bulk.KindMediaSync, object.SizeBytes)
		if errors.Is(err, bulk.ErrShowBlocked) {
			writeJSON(w, http.StatusLocked, map[string]any{"error_code": "BULK_TRANSFER_BLOCKED_SHOW"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "BULK_TRANSFER_UNAVAILABLE"})
			return
		}
		content = bulk.NewGuardedReadSeeker(r.Context(), s.bulk, jobID, file)
		defer func() {
			if r.Context().Err() != nil {
				s.bulk.Cancel(jobID, r.Context().Err().Error())
				return
			}
			s.bulk.Complete(jobID)
		}()
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", `"sha256:`+object.ContentHash+`"`)
	w.Header().Set("X-Content-SHA256", object.ContentHash)
	w.Header().Set("X-Content-Length", int64String(object.SizeBytes))
	http.ServeContent(w, r, object.ContentHash, object.CreatedAt, content)
}

func (s *Server) requireRuntimeSession(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "StageCoreSession "
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) || strings.TrimSpace(strings.TrimPrefix(authorization, prefix)) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": companionauth.CodeSessionInvalid})
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	if _, err := s.companionAuth.ValidateRuntimeSession(r.Context(), token); err != nil {
		writeCompanionAuthError(w, err)
		return false
	}
	return true
}

func int64String(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
