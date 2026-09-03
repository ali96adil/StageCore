package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/showtemplate"
)

type fakeShowTemplateService struct {
	listCalls        int
	getCalls         int
	materializeCalls int
	validateCalls    int
	importCalls      int
	exportCalls      int
	lastCreatedBy    string
}

func (f *fakeShowTemplateService) List() []showtemplate.Template {
	f.listCalls++
	return []showtemplate.Template{{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: "stagecore.starter.rehearsal", Version: "1.0.0", Source: showtemplate.SourceOfficial}}
}
func (f *fakeShowTemplateService) Get(id string) (showtemplate.Template, error) {
	f.getCalls++
	if id != "stagecore.starter.rehearsal" { return showtemplate.Template{}, showtemplate.ErrTemplateNotFound }
	return showtemplate.Template{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: id, Version: "1.0.0", Source: showtemplate.SourceOfficial}, nil
}
func (f *fakeShowTemplateService) ValidateDocument(data []byte) (showtemplate.Template, showtemplate.Compatibility, error) {
	f.validateCalls++
	return showtemplate.Template{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: "imported.test", Version: "1.0.0", Source: showtemplate.SourceImported}, showtemplate.Compatibility{Compatible: true}, nil
}
func (f *fakeShowTemplateService) Materialize(_ context.Context, templateID string, request showtemplate.MaterializeRequest) (showtemplate.MaterializeResult, error) {
	f.materializeCalls++
	f.lastCreatedBy = request.CreatedBy
	return showtemplate.MaterializeResult{TemplateID: templateID, ProjectID: "00000000-0000-7000-8000-000000000811", RevisionID: "00000000-0000-7000-8000-000000000812"}, nil
}
func (f *fakeShowTemplateService) MaterializeDocument(_ context.Context, data []byte, request showtemplate.MaterializeRequest) (showtemplate.MaterializeResult, showtemplate.Compatibility, error) {
	f.importCalls++
	f.lastCreatedBy = request.CreatedBy
	return showtemplate.MaterializeResult{TemplateID: "imported.test", ProjectID: "00000000-0000-7000-8000-000000000813", RevisionID: "00000000-0000-7000-8000-000000000814"}, showtemplate.Compatibility{Compatible: true}, nil
}
func (f *fakeShowTemplateService) ExportProject(_ context.Context, projectID string) (showtemplate.Template, error) {
	f.exportCalls++
	return showtemplate.Template{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: "exported.test", Version: "1.0.0", Source: showtemplate.SourceExported}, nil
}

func TestShowTemplateAPIAuthenticationRBACAndCSRF(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	service := &fakeShowTemplateService{}
	handler := New(WithOperatorShowTemplates(h.auth, service)).Handler()

	unauth := httptest.NewRequest(http.MethodGet, "/api/v1/show-templates", nil)
	unauth.RemoteAddr = "127.0.0.1:21001"
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusUnauthorized || service.listCalls != 0 { t.Fatalf("unauth status=%d calls=%d", unauthRes.Code, service.listCalls) }

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil { t.Fatal(err) }
	if _, err := h.db.DB.ExecContext(ctx, `INSERT INTO local_users
		(user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000815', 'template-viewer', ?, 'VIEWER', 1, 1, 1)`, passwordHash); err != nil { t.Fatal(err) }
	viewer, err := h.auth.Login(ctx, "template-viewer", h.password, "127.0.0.2")
	if err != nil { t.Fatal(err) }

	viewerList := httptest.NewRequest(http.MethodGet, "/api/v1/show-templates", nil)
	viewerList.RemoteAddr = "127.0.0.1:21002"
	viewerList.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerListRes := httptest.NewRecorder(); handler.ServeHTTP(viewerListRes, viewerList)
	if viewerListRes.Code != http.StatusOK || service.listCalls != 1 { t.Fatalf("viewer list status=%d calls=%d", viewerListRes.Code, service.listCalls) }

	viewerExport := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project-1/template-export", nil)
	viewerExport.RemoteAddr = "127.0.0.1:21003"; viewerExport.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerExportRes := httptest.NewRecorder(); handler.ServeHTTP(viewerExportRes, viewerExport)
	if viewerExportRes.Code != http.StatusOK || service.exportCalls != 1 { t.Fatalf("viewer export status=%d calls=%d", viewerExportRes.Code, service.exportCalls) }

	viewerMaterialize := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/stagecore.starter.rehearsal/materialize", strings.NewReader(`{"locale":"en"}`))
	viewerMaterialize.RemoteAddr = "127.0.0.1:21004"; viewerMaterialize.Header.Set("Content-Type", "application/json"); viewerMaterialize.Header.Set(csrfHeader, viewer.CSRFToken); viewerMaterialize.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerMaterializeRes := httptest.NewRecorder(); handler.ServeHTTP(viewerMaterializeRes, viewerMaterialize)
	if viewerMaterializeRes.Code != http.StatusForbidden || service.materializeCalls != 0 { t.Fatalf("viewer materialize status=%d calls=%d", viewerMaterializeRes.Code, service.materializeCalls) }

	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/stagecore.starter.rehearsal/materialize", strings.NewReader(`{"locale":"en"}`))
	missingCSRF.RemoteAddr = "127.0.0.1:21005"; missingCSRF.Header.Set("Content-Type", "application/json"); missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	missingCSRFRes := httptest.NewRecorder(); handler.ServeHTTP(missingCSRFRes, missingCSRF)
	if missingCSRFRes.Code != http.StatusForbidden || service.materializeCalls != 0 { t.Fatalf("missing CSRF status=%d calls=%d", missingCSRFRes.Code, service.materializeCalls) }

	ownerMaterialize := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/stagecore.starter.rehearsal/materialize", strings.NewReader(`{"locale":"ar-IQ","project_name":"My Rehearsal"}`))
	ownerMaterialize.RemoteAddr = "127.0.0.1:21006"; ownerMaterialize.Header.Set("Content-Type", "application/json"); ownerMaterialize.Header.Set(csrfHeader, owner.CSRFToken); ownerMaterialize.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerMaterializeRes := httptest.NewRecorder(); handler.ServeHTTP(ownerMaterializeRes, ownerMaterialize)
	if ownerMaterializeRes.Code != http.StatusCreated || service.materializeCalls != 1 || strings.TrimSpace(service.lastCreatedBy) == "" { t.Fatalf("owner materialize status=%d calls=%d actor=%q body=%s", ownerMaterializeRes.Code, service.materializeCalls, service.lastCreatedBy, ownerMaterializeRes.Body.String()) }

	document, _ := json.Marshal(showtemplate.Template{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: "imported.test", Version: "1.0.0", Source: showtemplate.SourceImported})
	validateBody, _ := json.Marshal(map[string]any{"template": json.RawMessage(document)})
	viewerValidate := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/import/validate", strings.NewReader(string(validateBody)))
	viewerValidate.RemoteAddr = "127.0.0.1:21007"; viewerValidate.Header.Set("Content-Type", "application/json"); viewerValidate.Header.Set(csrfHeader, viewer.CSRFToken); viewerValidate.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerValidateRes := httptest.NewRecorder(); handler.ServeHTTP(viewerValidateRes, viewerValidate)
	if viewerValidateRes.Code != http.StatusOK || service.validateCalls != 1 { t.Fatalf("viewer validate status=%d calls=%d body=%s", viewerValidateRes.Code, service.validateCalls, viewerValidateRes.Body.String()) }
}
