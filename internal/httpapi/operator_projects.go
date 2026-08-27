package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type projectView struct {
	ID                string                  `json:"project_id"`
	Name              string                  `json:"name"`
	Description       string                  `json:"description"`
	LifecycleState    domain.ProjectLifecycle `json:"lifecycle_state"`
	CurrentRevisionID string                  `json:"current_revision_id"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

type revisionView struct {
	ID             string                `json:"revision_id"`
	RevisionNumber int64                 `json:"revision_number"`
	Status         domain.RevisionStatus `json:"status"`
	CreatedAt      time.Time             `json:"created_at"`
}

type snapshotView struct {
	ID              string                       `json:"runtime_snapshot_id"`
	RevisionID      string                       `json:"revision_id"`
	SnapshotVersion int64                        `json:"snapshot_version"`
	Status          domain.RuntimeSnapshotStatus `json:"status"`
	CreatedAt       time.Time                    `json:"created_at"`
}

type sessionView struct {
	ID                string               `json:"session_id"`
	RuntimeSnapshotID string               `json:"runtime_snapshot_id"`
	Type              domain.SessionType   `json:"type"`
	Status            domain.SessionStatus `json:"status"`
	StartedAt         time.Time            `json:"started_at"`
}

type cueSummaryView struct {
	ID           string `json:"cue_id"`
	DisplayLabel string `json:"display_label"`
	Name         string `json:"name"`
	OrderIndex   int    `json:"order_index"`
}

type dashboardView struct {
	Project             projectView      `json:"project"`
	DraftRevision       revisionView     `json:"draft_revision"`
	PublishedSnapshot   *snapshotView    `json:"published_snapshot"`
	PublicationState    string           `json:"publication_state"`
	UnpublishedChanges  bool             `json:"unpublished_changes"`
	Mode                string           `json:"mode"`
	ActiveSession       *sessionView     `json:"active_session"`
	CurrentCue          *cueSummaryView  `json:"current_cue"`
	NextCue             *cueSummaryView  `json:"next_cue"`
	RuntimeErrorCount   int64            `json:"runtime_error_count"`
	RuntimeWarningCount int64            `json:"runtime_warning_count"`
	Readiness           dashboardReadiness `json:"readiness"`
}

type dashboardReadiness struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func WithOperatorProjects(auth *userauth.Service, projectStore *store.Store) Option {
	return func(s *Server) {
		if auth == nil || projectStore == nil {
			return
		}
		registerOperatorProjectRoutes(s.mux, auth, projectStore)
	}
}

func registerOperatorProjectRoutes(mux *http.ServeMux, auth *userauth.Service, projectStore *store.Store) {
	mux.HandleFunc("GET /api/v1/projects", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projects, err := projectStore.ListProjects(r.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECTS_UNAVAILABLE"})
			return
		}
		items := make([]projectView, 0, len(projects))
		for _, project := range projects {
			items = append(items, makeProjectView(project))
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": items})
	}))

	mux.HandleFunc("POST /api/v1/projects", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body createProjectRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		project, revision, err := projectStore.CreateProject(r.Context(), store.CreateProjectParams{
			Name: strings.TrimSpace(body.Name), Description: body.Description,
			CreatedBy: session.User.ID, ChangeNote: "Created from Operator Web",
		})
		if err != nil {
			status := http.StatusBadRequest
			code := "PROJECT_INVALID"
			if !errors.Is(err, domain.ErrInvalidInput) {
				status = http.StatusServiceUnavailable
				code = "PROJECT_CREATE_FAILED"
			}
			writeJSON(w, status, map[string]any{"error_code": code})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"project": makeProjectView(project),
			"draft_revision": makeRevisionView(revision),
		})
	}))

	mux.HandleFunc("GET /api/v1/projects/{project_id}", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, revision, ok := loadProjectAndRevision(w, r, projectStore)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"project": makeProjectView(project),
			"draft_revision": makeRevisionView(revision),
		})
	}))

	mux.HandleFunc("GET /api/v1/projects/{project_id}/dashboard", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, revision, ok := loadProjectAndRevision(w, r, projectStore)
		if !ok {
			return
		}
		dashboard, err := buildDashboard(r, projectStore, project, revision)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "DASHBOARD_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, dashboard)
	}))
}

func loadProjectAndRevision(w http.ResponseWriter, r *http.Request, projectStore *store.Store) (domain.Project, domain.ProjectRevision, bool) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	project, err := projectStore.GetProject(r.Context(), projectID)
	if errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_NOT_FOUND"})
		return domain.Project{}, domain.ProjectRevision{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECTS_UNAVAILABLE"})
		return domain.Project{}, domain.ProjectRevision{}, false
	}
	revision, err := projectStore.GetRevision(r.Context(), project.CurrentRevisionID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "REVISION_UNAVAILABLE"})
		return domain.Project{}, domain.ProjectRevision{}, false
	}
	return project, revision, true
}

func buildDashboard(r *http.Request, projectStore *store.Store, project domain.Project, revision domain.ProjectRevision) (dashboardView, error) {
	snapshot, err := projectStore.LatestPublishedRuntimeSnapshotForProject(r.Context(), project.ID)
	if err != nil {
		return dashboardView{}, err
	}
	activeSession, err := projectStore.ActiveSessionForProject(r.Context(), project.ID)
	if err != nil {
		return dashboardView{}, err
	}

	view := dashboardView{
		Project: makeProjectView(project), DraftRevision: makeRevisionView(revision),
		PublicationState: "NOT_PUBLISHED", UnpublishedChanges: true, Mode: "EDIT",
		Readiness: dashboardReadiness{Status: "NOT_EVALUATED", Note: "Full endpoint/media/storage preflight is delivered in M6 S3."},
	}
	if snapshot != nil {
		value := makeSnapshotView(*snapshot)
		view.PublishedSnapshot = &value
		view.PublicationState = "PUBLISHED"
		view.UnpublishedChanges = snapshot.RevisionID != revision.ID
	}
	if activeSession == nil {
		return view, nil
	}

	sessionValue := makeSessionView(*activeSession)
	view.ActiveSession = &sessionValue
	switch activeSession.Type {
	case domain.SessionShow:
		view.Mode = "SHOW"
	case domain.SessionRehearsal:
		view.Mode = "REHEARSAL"
	default:
		view.Mode = "EDIT"
	}
	view.RuntimeErrorCount, err = projectStore.SessionRuntimeErrorCount(r.Context(), activeSession.ID)
	if err != nil {
		return dashboardView{}, err
	}

	activeSnapshot, err := projectStore.GetRuntimeSnapshot(r.Context(), activeSession.RuntimeSnapshotID)
	if err != nil {
		return dashboardView{}, err
	}
	cues, err := projectStore.ListCues(r.Context(), activeSnapshot.RevisionID)
	if err != nil {
		return dashboardView{}, err
	}
	current, next := currentAndNextCue(cues, activeSession.CurrentCueID)
	view.CurrentCue = current
	view.NextCue = next
	return view, nil
}

func currentAndNextCue(cues []domain.Cue, currentID *string) (*cueSummaryView, *cueSummaryView) {
	if currentID == nil {
		for _, cue := range cues {
			if cue.Enabled {
				value := makeCueSummary(cue)
				return nil, &value
			}
		}
		return nil, nil
	}
	var current *cueSummaryView
	foundCurrent := false
	for _, cue := range cues {
		if cue.ID == *currentID {
			value := makeCueSummary(cue)
			current = &value
			foundCurrent = true
			continue
		}
		if foundCurrent && cue.Enabled {
			value := makeCueSummary(cue)
			return current, &value
		}
	}
	return current, nil
}

func makeProjectView(project domain.Project) projectView {
	return projectView{
		ID: project.ID, Name: project.Name, Description: project.Description,
		LifecycleState: project.LifecycleState, CurrentRevisionID: project.CurrentRevisionID,
		CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt,
	}
}

func makeRevisionView(revision domain.ProjectRevision) revisionView {
	return revisionView{ID: revision.ID, RevisionNumber: revision.RevisionNumber, Status: revision.Status, CreatedAt: revision.CreatedAt}
}

func makeSnapshotView(snapshot domain.RuntimeSnapshot) snapshotView {
	return snapshotView{ID: snapshot.ID, RevisionID: snapshot.RevisionID, SnapshotVersion: snapshot.SnapshotVersion, Status: snapshot.Status, CreatedAt: snapshot.CreatedAt}
}

func makeSessionView(session domain.Session) sessionView {
	return sessionView{ID: session.ID, RuntimeSnapshotID: session.RuntimeSnapshotID, Type: session.Type, Status: session.Status, StartedAt: session.StartedAt}
}

func makeCueSummary(cue domain.Cue) cueSummaryView {
	return cueSummaryView{ID: cue.ID, DisplayLabel: cue.DisplayLabel, Name: cue.Name, OrderIndex: cue.OrderIndex}
}
