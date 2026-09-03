package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/showtemplate"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type ShowTemplateService interface {
	List() []showtemplate.Template
	Get(string) (showtemplate.Template, error)
	ValidateDocument([]byte) (showtemplate.Template, showtemplate.Compatibility, error)
	Materialize(context.Context, string, showtemplate.MaterializeRequest) (showtemplate.MaterializeResult, error)
	MaterializeDocument(context.Context, []byte, showtemplate.MaterializeRequest) (showtemplate.MaterializeResult, showtemplate.Compatibility, error)
	ExportProject(context.Context, string) (showtemplate.Template, error)
}

type templateMaterializeRequest struct {
	ProjectName        string         `json:"project_name"`
	ProjectDescription string         `json:"project_description"`
	Locale             string         `json:"locale"`
	Values             map[string]any `json:"values"`
}

type templateDocumentRequest struct {
	Template json.RawMessage `json:"template"`
}

type templateDocumentMaterializeRequest struct {
	Template           json.RawMessage `json:"template"`
	ProjectName        string          `json:"project_name"`
	ProjectDescription string          `json:"project_description"`
	Locale             string          `json:"locale"`
	Values             map[string]any  `json:"values"`
}

func WithOperatorShowTemplates(auth *userauth.Service, service ShowTemplateService) Option {
	return func(s *Server) {
		if auth == nil || service == nil { return }

		s.mux.HandleFunc("GET /api/v1/show-templates", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, _ *http.Request, _ userauth.Session) {
			writeJSON(w, http.StatusOK, map[string]any{"schema_version": showtemplate.SchemaVersion, "api_version": showtemplate.CurrentAPIVersion, "templates": service.List()})
		}))
		s.mux.HandleFunc("GET /api/v1/show-templates/{template_id}", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			value, err := service.Get(strings.TrimSpace(r.PathValue("template_id")))
			if errors.Is(err, showtemplate.ErrTemplateNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "SHOW_TEMPLATE_NOT_FOUND"}); return
			}
			if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "SHOW_TEMPLATE_UNAVAILABLE"}); return }
			writeJSON(w, http.StatusOK, map[string]any{"template": value, "compatibility": showtemplate.CompatibilityFor(value)})
		}))
		s.mux.HandleFunc("POST /api/v1/show-templates/{template_id}/materialize", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			var body templateMaterializeRequest
			if !decodeBoundedJSON(w, r, &body) { return }
			if !validTemplateMaterializeBody(body.ProjectName, body.ProjectDescription, body.Locale, body.Values) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "SHOW_TEMPLATE_VALUES_INVALID"}); return
			}
			result, err := service.Materialize(r.Context(), strings.TrimSpace(r.PathValue("template_id")), showtemplate.MaterializeRequest{ProjectName: body.ProjectName, ProjectDescription: body.ProjectDescription, Locale: body.Locale, Values: body.Values, CreatedBy: session.User.ID})
			if err != nil { writeShowTemplateError(w, err); return }
			writeJSON(w, http.StatusCreated, map[string]any{"result": result})
		}))
		s.mux.HandleFunc("POST /api/v1/show-templates/import/validate", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			var body templateDocumentRequest
			if !decodeBoundedJSON(w, r, &body) { return }
			if len(body.Template) == 0 || len(body.Template) > 2<<20 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "SHOW_TEMPLATE_DOCUMENT_INVALID"}); return
			}
			value, compatibility, err := service.ValidateDocument(body.Template)
			if err != nil { writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "SHOW_TEMPLATE_DOCUMENT_INVALID", "message": err.Error(), "compatibility": compatibility}); return }
			writeJSON(w, http.StatusOK, map[string]any{"template": value, "compatibility": compatibility})
		}))
		s.mux.HandleFunc("POST /api/v1/show-templates/import/materialize", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			var body templateDocumentMaterializeRequest
			if !decodeBoundedJSON(w, r, &body) { return }
			if len(body.Template) == 0 || len(body.Template) > 2<<20 || !validTemplateMaterializeBody(body.ProjectName, body.ProjectDescription, body.Locale, body.Values) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "SHOW_TEMPLATE_DOCUMENT_INVALID"}); return
			}
			result, compatibility, err := service.MaterializeDocument(r.Context(), body.Template, showtemplate.MaterializeRequest{ProjectName: body.ProjectName, ProjectDescription: body.ProjectDescription, Locale: body.Locale, Values: body.Values, CreatedBy: session.User.ID})
			if err != nil { writeShowTemplateError(w, err); return }
			writeJSON(w, http.StatusCreated, map[string]any{"result": result, "compatibility": compatibility})
		}))
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/template-export", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			value, err := service.ExportProject(r.Context(), strings.TrimSpace(r.PathValue("project_id")))
			if err != nil { writeShowTemplateError(w, err); return }
			writeJSON(w, http.StatusOK, map[string]any{"template": value, "compatibility": showtemplate.CompatibilityFor(value)})
		}))
	}
}

func validTemplateMaterializeBody(name, description, locale string, values map[string]any) bool {
	if len(strings.TrimSpace(name)) > 160 || len(description) > 500 || len(values) > 64 { return false }
	locale = strings.TrimSpace(locale)
	if locale != "" && locale != "en" && locale != "ar-IQ" && locale != "ar" { return false }
	return true
}

func writeShowTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, showtemplate.ErrTemplateNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "SHOW_TEMPLATE_NOT_FOUND"})
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "SHOW_TEMPLATE_VALUES_INVALID", "message": err.Error()})
	case errors.Is(err, domain.ErrShowConfigurationLocked):
		writeJSON(w, http.StatusLocked, map[string]any{"error_code": "SHOW_CONFIGURATION_LOCKED"})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "SHOW_TEMPLATE_CONFLICT", "message": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_NOT_FOUND"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "SHOW_TEMPLATE_UNAVAILABLE"})
	}
}
