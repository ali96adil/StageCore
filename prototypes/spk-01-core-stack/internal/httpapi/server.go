package httpapi

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-01-core-stack/internal/core"
)

//go:embed web/*
var webFiles embed.FS

type Server struct {
	core *core.Service
	bus  *core.Bus
}

func New(c *core.Service, bus *core.Bus) http.Handler {
	s := &Server{core: c, bus: bus}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("POST /api/cues", s.addCue)
	mux.HandleFunc("POST /api/publish", s.publish)
	mux.HandleFunc("POST /api/go", s.goCue)
	mux.HandleFunc("GET /api/events", s.events)
	root, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(root)))
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]any{"status":"ok","time":time.Now().UTC()}) }
func (s *Server) state(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, s.core.State()) }

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string `json:"name"` }
	if err := decode(r, &in); err != nil { fail(w, http.StatusBadRequest, err); return }
	p, err := s.core.CreateProject(in.Name)
	if err != nil { fail(w, http.StatusBadRequest, err); return }
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) addCue(w http.ResponseWriter, r *http.Request) {
	var in struct { ProjectID string `json:"project_id"`; Number string `json:"number"`; Name string `json:"name"`; Message string `json:"message"` }
	if err := decode(r, &in); err != nil { fail(w, http.StatusBadRequest, err); return }
	cue, err := s.core.AddCue(in.ProjectID, in.Number, in.Name, in.Message)
	if err != nil { fail(w, statusFor(err), err); return }
	writeJSON(w, http.StatusCreated, cue)
}

func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	var in struct{ ProjectID string `json:"project_id"` }
	if err := decode(r, &in); err != nil { fail(w, http.StatusBadRequest, err); return }
	snap, err := s.core.Publish(in.ProjectID)
	if err != nil { fail(w, statusFor(err), err); return }
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) goCue(w http.ResponseWriter, r *http.Request) {
	var in struct{ ProjectID string `json:"project_id"` }
	if err := decode(r, &in); err != nil { fail(w, http.StatusBadRequest, err); return }
	exec, err := s.core.Go(in.ProjectID)
	if err != nil { fail(w, statusFor(err), err); return }
	writeJSON(w, http.StatusOK, exec)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok { fail(w, http.StatusInternalServerError, errors.New("streaming unsupported")); return }
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, cancel := s.bus.Subscribe(64)
	defer cancel()
	fmt.Fprint(w, ": stagecore events\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done(): return
		case evt, ok := <-ch:
			if !ok { return }
			b, _ := json.Marshal(evt)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func statusFor(err error) int {
	if errors.Is(err, core.ErrProjectNotFound) { return http.StatusNotFound }
	if errors.Is(err, core.ErrNoPublished) || errors.Is(err, core.ErrNoNextCue) { return http.StatusConflict }
	return http.StatusBadRequest
}

func decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) { w.Header().Set("Content-Type","application/json"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(v) }
func fail(w http.ResponseWriter, status int, err error) { writeJSON(w, status, map[string]any{"error":err.Error()}) }
