package httpapi

import (
	"net/http"

	"github.com/ali96adil/StageCore/internal/storagehealth"
)

func (s *Server) handleStorageReady(w http.ResponseWriter, _ *http.Request) {
	if s.storageHealth == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "READY"})
		return
	}
	status := s.storageHealth.Status()
	payload := map[string]any{
		"storage_state": status.State,
		"data_root": storageStatusJSON(status.DataRoot),
		"vault_root": storageStatusJSON(status.VaultRoot),
	}
	if status.State == storagehealth.Critical || status.State == storagehealth.Unavailable {
		payload["status"] = "BLOCKED"
		writeJSON(w, http.StatusServiceUnavailable, payload)
		return
	}
	payload["status"] = "READY"
	writeJSON(w, http.StatusOK, payload)
}

func storageStatusJSON(status storagehealth.Status) map[string]any {
	return map[string]any{
		"state": status.State,
		"total_bytes": status.TotalBytes,
		"free_bytes": status.FreeBytes,
		"reserve_bytes": status.ReserveBytes,
		"free_percent": status.FreePercent,
		"writable": status.Writable,
		"reason": status.Reason,
	}
}
