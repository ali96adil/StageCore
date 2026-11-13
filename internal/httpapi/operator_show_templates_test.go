package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/showtemplate"
	"github.com/ali96adil/StageCore/internal/store"
)

type fakeShowTemplateService struct {
	listCalls        int
	getCalls         int
	materializeCalls int
	validateCalls    int
	importCalls      int
	exportCalls      int
	lastCreatedBy    string
	onExport         func()
	onMaterialize    func()
}

func (f *fakeShowTemplateService) List() []showtemplate.Template {
	f.listCalls++
	return []showtemplate.Template{{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: "stagecore.starter.rehearsal", Version: "1.0.0", Source: showtemplate.SourceOfficial}}
}
func (f *fakeShowTemplateService) Get(id string) (showtemplate.Template, error) {
	f.getCalls++
	if id != "stagecore.starter.rehearsal" {
		return showtemplate.Template{}, showtemplate.ErrTemplateNotFound
	}
	return showtemplate.Template{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: id, Version: "1.0.0", Source: showtemplate.SourceOfficial}, nil
}
func (f *fakeShowTemplateService) ValidateDocument(data []byte) (showtemplate.Template, showtemplate.Compatibility, error) {
	f.validateCalls++
	return showtemplate.Template{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: "imported.test", Version: "1.0.0", Source: showtemplate.SourceImported}, showtemplate.Compatibility{Compatible: true}, nil
}
func (f *fakeShowTemplateService) Materialize(_ context.Context, templateID string, request showtemplate.MaterializeRequest) (showtemplate.MaterializeResult, error) {
	f.materializeCalls++
	f.lastCreatedBy = request.CreatedBy
	if f.onMaterialize != nil {
		f.onMaterialize()
	}
	return showtemplate.MaterializeResult{TemplateID: templateID, ProjectID: "00000000-0000-7000-8000-000000000811", RevisionID: "00000000-0000-7000-8000-000000000812"}, nil
}
func (f *fakeShowTemplateService) MaterializeDocument(_ context.Context, data []byte, request showtemplate.MaterializeRequest) (showtemplate.MaterializeResult, showtemplate.Compatibility, error) {
	f.importCalls++
	f.lastCreatedBy = request.CreatedBy
	if f.onMaterialize != nil {
		f.onMaterialize()
	}
	return showtemplate.MaterializeResult{TemplateID: "imported.test", ProjectID: "00000000-0000-7000-8000-000000000813", RevisionID: "00000000-0000-7000-8000-000000000814"}, showtemplate.Compatibility{Compatible: true}, nil
}
func (f *fakeShowTemplateService) ExportProject(_ context.Context, projectID string) (showtemplate.Template, error) {
	f.exportCalls++
	if f.onExport != nil {
		f.onExport()
	}
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
	if unauthRes.Code != http.StatusUnauthorized || service.listCalls != 0 {
		t.Fatalf("unauth status=%d calls=%d", unauthRes.Code, service.listCalls)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `INSERT INTO local_users
		(user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000815', 'template-viewer', ?, 'VIEWER', 1, 1, 1)`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "template-viewer", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	viewerList := httptest.NewRequest(http.MethodGet, "/api/v1/show-templates", nil)
	viewerList.RemoteAddr = "127.0.0.1:21002"
	viewerList.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerListRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerListRes, viewerList)
	if viewerListRes.Code != http.StatusOK || service.listCalls != 1 {
		t.Fatalf("viewer list status=%d calls=%d", viewerListRes.Code, service.listCalls)
	}

	viewerExport := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project-1/template-export", nil)
	viewerExport.RemoteAddr = "127.0.0.1:21003"
	viewerExport.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerExportRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerExportRes, viewerExport)
	if viewerExportRes.Code != http.StatusOK || service.exportCalls != 1 {
		t.Fatalf("viewer export status=%d calls=%d", viewerExportRes.Code, service.exportCalls)
	}

	viewerMaterialize := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/stagecore.starter.rehearsal/materialize", strings.NewReader(`{"locale":"en"}`))
	viewerMaterialize.RemoteAddr = "127.0.0.1:21004"
	viewerMaterialize.Header.Set("Content-Type", "application/json")
	viewerMaterialize.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerMaterialize.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerMaterializeRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerMaterializeRes, viewerMaterialize)
	if viewerMaterializeRes.Code != http.StatusForbidden || service.materializeCalls != 0 {
		t.Fatalf("viewer materialize status=%d calls=%d", viewerMaterializeRes.Code, service.materializeCalls)
	}

	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/stagecore.starter.rehearsal/materialize", strings.NewReader(`{"locale":"en"}`))
	missingCSRF.RemoteAddr = "127.0.0.1:21005"
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	missingCSRFRes := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRes, missingCSRF)
	if missingCSRFRes.Code != http.StatusForbidden || service.materializeCalls != 0 {
		t.Fatalf("missing CSRF status=%d calls=%d", missingCSRFRes.Code, service.materializeCalls)
	}

	ownerMaterialize := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/stagecore.starter.rehearsal/materialize", strings.NewReader(`{"locale":"ar-IQ","project_name":"My Rehearsal"}`))
	ownerMaterialize.RemoteAddr = "127.0.0.1:21006"
	ownerMaterialize.Header.Set("Content-Type", "application/json")
	ownerMaterialize.Header.Set(csrfHeader, owner.CSRFToken)
	ownerMaterialize.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerMaterializeRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerMaterializeRes, ownerMaterialize)
	if ownerMaterializeRes.Code != http.StatusCreated || service.materializeCalls != 1 || strings.TrimSpace(service.lastCreatedBy) == "" {
		t.Fatalf("owner materialize status=%d calls=%d actor=%q body=%s", ownerMaterializeRes.Code, service.materializeCalls, service.lastCreatedBy, ownerMaterializeRes.Body.String())
	}

	document, _ := json.Marshal(showtemplate.Template{SchemaVersion: 1, MinAPIVersion: 1, MaxAPIVersion: 1, ID: "imported.test", Version: "1.0.0", Source: showtemplate.SourceImported})
	validateBody, _ := json.Marshal(map[string]any{"template": json.RawMessage(document)})
	viewerValidate := httptest.NewRequest(http.MethodPost, "/api/v1/show-templates/import/validate", strings.NewReader(string(validateBody)))
	viewerValidate.RemoteAddr = "127.0.0.1:21007"
	viewerValidate.Header.Set("Content-Type", "application/json")
	viewerValidate.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerValidate.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerValidateRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerValidateRes, viewerValidate)
	if viewerValidateRes.Code != http.StatusOK || service.validateCalls != 1 {
		t.Fatalf("viewer validate status=%d calls=%d body=%s", viewerValidateRes.Code, service.validateCalls, viewerValidateRes.Body.String())
	}
}

func TestShowTemplateDraftRollsBackWhenBrowserSessionEnds(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	projects := store.New(h.db.DB, clock.Real{})
	service, err := showtemplate.NewService(projects, showtemplate.BuiltinCatalog())
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorShowTemplates(h.auth, service)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/show-templates/stagecore.starter.rehearsal/materialize",
		[]byte(`{"project_name":"Revoked draft"}`), login.Token, login.CSRFToken)
	req.Body = &firmwareUploadRevokeReader{ReadCloser: req.Body, revoke: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("revoked template response=%d body=%s", res.Code, res.Body)
	}
	drafts, err := projects.ListProjects(ctx)
	if err != nil || len(drafts) != 0 {
		t.Fatalf("revoked template left projects=%+v err=%v", drafts, err)
	}
}

func TestShowTemplateExportWithholdsProjectDataAfterLogout(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeShowTemplateService{onExport: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	handler := New(WithOperatorShowTemplates(h.auth, service)).Handler()
	req := authenticatedMutationRequest(t, http.MethodGet,
		"/api/v1/projects/00000000-0000-7000-8000-000000000811/template-export",
		nil, login.Token, "")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || service.exportCalls != 1 ||
		strings.Contains(res.Body.String(), "exported.test") {
		t.Fatalf("revoked export status=%d calls=%d body=%s", res.Code, service.exportCalls, res.Body)
	}
}

func TestShowTemplateMaterializationSuppressesCommittedResultAfterLogout(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeShowTemplateService{onMaterialize: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	handler := New(WithOperatorShowTemplates(h.auth, service)).Handler()
	request := authenticatedMutationRequest(
		t,
		http.MethodPost,
		"/api/v1/show-templates/stagecore.starter.rehearsal/materialize",
		[]byte(`{"project_name":"Committed but withheld"}`),
		login.Token,
		login.CSRFToken,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if service.materializeCalls != 1 || response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"result"`) {
		t.Fatalf("revoked materialization exposed result: calls=%d status=%d body=%s", service.materializeCalls, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked materialization lost commit uncertainty: %s", response.Body.String())
	}
}
