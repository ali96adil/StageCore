package server

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-06-hub-deployment/internal/config"
	"github.com/ali96adil/StageCore/prototypes/spk-06-hub-deployment/internal/storage"
)

type Server struct {
	cfg    config.Config
	layout storage.Layout
	hubID  string
	mux    *http.ServeMux
}

func New(cfg config.Config, layout storage.Layout, hubID string) *Server {
	s := &Server{cfg: cfg, layout: layout, hubID: hubID, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "LIVE"})
	})
	s.mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := s.layout.Writable(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "NOT_READY", "reason": "data_root_unwritable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "READY", "hub_id": s.hubID, "instance_name": s.cfg.InstanceName,
			"goos": runtime.GOOS, "goarch": runtime.GOARCH,
		})
	})
	s.mux.HandleFunc("GET /runtime/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "OK", "at": time.Now().UTC()})
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
