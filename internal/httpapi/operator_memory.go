package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type noteView struct {
	NoteID     string            `json:"note_id"`
	ProjectID  string            `json:"project_id"`
	SessionID  *string           `json:"session_id,omitempty"`
	CueID      *string           `json:"cue_id,omitempty"`
	Category   string            `json:"category"`
	Body       string            `json:"body"`
	Status     domain.NoteStatus `json:"status"`
	CreatedBy  string            `json:"created_by"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
}

type createNoteRequest struct {
	SessionID *string `json:"session_id"`
	CueID     *string `json:"cue_id"`
	Category  string  `json:"category"`
	Body      string  `json:"body"`
}

type updateNoteRequest struct {
	Category string `json:"category"`
	Body     string `json:"body"`
}

func WithOperatorMemory(auth *userauth.Service, projectStore *store.Store, memory *sessionmemory.Service) Option {
	return func(s *Server) {
		if auth == nil || projectStore == nil || memory == nil {
			return
		}

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/sessions", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			limit := 100
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				value, err := strconv.Atoi(raw)
				if err != nil || value <= 0 || value > 500 {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_LIMIT"})
					return
				}
				limit = value
			}
			sessions, err := memory.List(r.Context(), r.PathValue("project_id"), limit)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
		}))

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/sessions/{session_id}", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			detail, err := memory.Detail(r.Context(), r.PathValue("project_id"), r.PathValue("session_id"))
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, detail)
		}))

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/notes", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			filter := store.NoteFilter{
				Status: domain.NoteStatus(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))),
				Category: strings.TrimSpace(r.URL.Query().Get("category")),
				SessionID: strings.TrimSpace(r.URL.Query().Get("session_id")),
				CueID: strings.TrimSpace(r.URL.Query().Get("cue_id")),
			}
			notes, err := projectStore.ListNotes(r.Context(), r.PathValue("project_id"), filter)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			views := make([]noteView, 0, len(notes))
			for _, note := range notes {
				views = append(views, viewNote(note))
			}
			writeJSON(w, http.StatusOK, map[string]any{"notes": views})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/notes", withPermission(auth, userauth.PermissionNoteWrite, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			var body createNoteRequest
			if !decodeBoundedJSON(w, r, &body) {
				return
			}
			note, err := projectStore.CreateNote(r.Context(), r.PathValue("project_id"), store.CreateNoteParams{
				SessionID: body.SessionID, CueID: body.CueID, Category: body.Category,
				Body: body.Body, CreatedBy: session.User.Username,
			})
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"note": viewNote(note)})
		}))

		s.mux.HandleFunc("PATCH /api/v1/projects/{project_id}/notes/{note_id}", withPermission(auth, userauth.PermissionNoteWrite, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			var body updateNoteRequest
			if !decodeBoundedJSON(w, r, &body) {
				return
			}
			note, err := projectStore.UpdateNote(r.Context(), r.PathValue("project_id"), r.PathValue("note_id"), body.Body, body.Category)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"note": viewNote(note)})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/notes/{note_id}/resolve", withPermission(auth, userauth.PermissionNoteWrite, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			note, err := projectStore.SetNoteStatus(r.Context(), r.PathValue("project_id"), r.PathValue("note_id"), domain.NoteResolved)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"note": viewNote(note)})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/notes/{note_id}/reopen", withPermission(auth, userauth.PermissionNoteWrite, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			note, err := projectStore.SetNoteStatus(r.Context(), r.PathValue("project_id"), r.PathValue("note_id"), domain.NoteOpen)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"note": viewNote(note)})
		}))
	}
}

func viewNote(note domain.Note) noteView {
	return noteView{
		NoteID: note.ID, ProjectID: note.ProjectID, SessionID: note.SessionID, CueID: note.CueID,
		Category: note.Category, Body: note.Body, Status: note.Status, CreatedBy: note.CreatedBy,
		CreatedAt: note.CreatedAt, UpdatedAt: note.UpdatedAt, ResolvedAt: note.ResolvedAt,
	}
}
