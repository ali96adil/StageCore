package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/showcapsule"
)

type fakeShowCapsuleService struct {
	previewCalls              int
	exportCalls               int
	planCalls                 int
	materializeCalls          int
	lastProjectID             string
	lastPath                  string
	lastExportRoot            string
	lastMode                  showcapsule.ExportMode
	lastImportedBy            string
	previewErr                error
	onPreview                 func()
	planErr                   error
	onExport                  func()
	afterExportAuthorize      func()
	onPlan                    func()
	afterMaterializeAuthorize func()
}

func (f *fakeShowCapsuleService) BuildManifest(_ context.Context, projectID string, options showcapsule.BuildOptions) (showcapsule.Manifest, error) {
	f.previewCalls++
	f.lastProjectID = projectID
	f.lastMode = options.Mode
	if f.onPreview != nil {
		f.onPreview()
	}
	return showcapsule.Manifest{ExportMode: options.Mode}, f.previewErr
}

func (f *fakeShowCapsuleService) Export(ctx context.Context, projectID, root string, options showcapsule.BuildOptions) (showcapsule.ExportResult, error) {
	f.exportCalls++
	f.lastProjectID = projectID
	f.lastExportRoot = root
	f.lastMode = options.Mode
	if f.onExport != nil {
		f.onExport()
	}
	if options.AuthorizePublish != nil {
		if err := options.AuthorizePublish(ctx); err != nil {
			return showcapsule.ExportResult{}, err
		}
	}
	if f.afterExportAuthorize != nil {
		f.afterExportAuthorize()
	}
	return showcapsule.ExportResult{CapsuleID: "00000000-0000-7000-8000-000000000611", Manifest: showcapsule.Manifest{ExportMode: options.Mode}}, nil
}

func (f *fakeShowCapsuleService) PlanImport(_ context.Context, path string) (showcapsule.ImportPlan, error) {
	f.planCalls++
	if f.onPlan != nil {
		f.onPlan()
	}
	f.lastPath = path
	return showcapsule.ImportPlan{CapsuleID: filepath.Base(path), MaterializationReady: true}, f.planErr
}

func (f *fakeShowCapsuleService) Materialize(ctx context.Context, path string, options showcapsule.MaterializeOptions) (showcapsule.MaterializeResult, error) {
	f.materializeCalls++
	f.lastPath = path
	f.lastImportedBy = options.ImportedBy
	if options.AuthorizeWrite != nil {
		if err := options.AuthorizeWrite(ctx); err != nil {
			return showcapsule.MaterializeResult{}, err
		}
	}
	if f.afterMaterializeAuthorize != nil {
		f.afterMaterializeAuthorize()
	}
	return showcapsule.MaterializeResult{CapsuleID: filepath.Base(path), ProjectID: "00000000-0000-7000-8000-000000000612"}, nil
}

func TestShowCapsuleAPIAuthenticationRBACCSRFAndPathBounds(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	service := &fakeShowCapsuleService{}
	root := t.TempDir()
	const projectID = "00000000-0000-7000-8000-000000000610"
	const capsuleID = "00000000-0000-7000-8000-000000000611"
	if err := os.MkdirAll(filepath.Join(root, "imports", capsuleID), 0o750); err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorShowCapsules(h.auth, service, root)).Handler()

	unauth := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/show-capsule/preview", nil)
	unauth.RemoteAddr = "127.0.0.1:19101"
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusUnauthorized || service.previewCalls != 0 {
		t.Fatalf("unauth preview status=%d calls=%d", unauthRes.Code, service.previewCalls)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000613', 'capsule-viewer', ?, 'VIEWER', 1, 1, 1)
	`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "capsule-viewer", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	viewerPreview := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/show-capsule/preview?mode=SELF_CONTAINED", nil)
	viewerPreview.RemoteAddr = "127.0.0.1:19102"
	viewerPreview.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerPreviewRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerPreviewRes, viewerPreview)
	if viewerPreviewRes.Code != http.StatusOK || service.previewCalls != 1 || service.lastMode != showcapsule.ExportSelfContained {
		t.Fatalf("viewer preview status=%d calls=%d mode=%q body=%s", viewerPreviewRes.Code, service.previewCalls, service.lastMode, viewerPreviewRes.Body.String())
	}

	invalidMode := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/show-capsule/preview?mode=FULL", nil)
	invalidMode.RemoteAddr = "127.0.0.1:19103"
	invalidMode.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	invalidModeRes := httptest.NewRecorder()
	handler.ServeHTTP(invalidModeRes, invalidMode)
	if invalidModeRes.Code != http.StatusBadRequest || service.previewCalls != 1 {
		t.Fatalf("invalid mode status=%d calls=%d", invalidModeRes.Code, service.previewCalls)
	}

	viewerExport := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/show-capsules", strings.NewReader(`{"mode":"MANIFEST_ONLY"}`))
	viewerExport.RemoteAddr = "127.0.0.1:19104"
	viewerExport.Header.Set("Content-Type", "application/json")
	viewerExport.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerExport.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerExportRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerExportRes, viewerExport)
	if viewerExportRes.Code != http.StatusForbidden || service.exportCalls != 0 {
		t.Fatalf("viewer export status=%d calls=%d", viewerExportRes.Code, service.exportCalls)
	}

	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/show-capsules", strings.NewReader(`{"mode":"MANIFEST_ONLY"}`))
	missingCSRF.RemoteAddr = "127.0.0.1:19105"
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	missingCSRFRes := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRes, missingCSRF)
	if missingCSRFRes.Code != http.StatusForbidden || service.exportCalls != 0 {
		t.Fatalf("missing CSRF export status=%d calls=%d", missingCSRFRes.Code, service.exportCalls)
	}

	ownerExport := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/show-capsules", strings.NewReader(`{"mode":"SELF_CONTAINED"}`))
	ownerExport.RemoteAddr = "127.0.0.1:19106"
	ownerExport.Header.Set("Content-Type", "application/json")
	ownerExport.Header.Set(csrfHeader, owner.CSRFToken)
	ownerExport.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerExportRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerExportRes, ownerExport)
	if ownerExportRes.Code != http.StatusCreated || service.exportCalls != 1 || service.lastExportRoot != filepath.Join(root, "exports") {
		t.Fatalf("owner export status=%d calls=%d root=%q body=%s", ownerExportRes.Code, service.exportCalls, service.lastExportRoot, ownerExportRes.Body.String())
	}

	viewerPlan := httptest.NewRequest(http.MethodGet, "/api/v1/show-capsules/imports/"+capsuleID+"/plan", nil)
	viewerPlan.RemoteAddr = "127.0.0.1:19107"
	viewerPlan.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerPlanRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerPlanRes, viewerPlan)
	if viewerPlanRes.Code != http.StatusOK || service.planCalls != 1 {
		t.Fatalf("viewer plan status=%d calls=%d", viewerPlanRes.Code, service.planCalls)
	}

	invalidPath := httptest.NewRequest(http.MethodGet, "/api/v1/show-capsules/imports/not-a-capsule/plan", nil)
	invalidPath.RemoteAddr = "127.0.0.1:19108"
	invalidPath.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	invalidPathRes := httptest.NewRecorder()
	handler.ServeHTTP(invalidPathRes, invalidPath)
	if invalidPathRes.Code != http.StatusBadRequest || service.planCalls != 1 {
		t.Fatalf("invalid capsule path status=%d calls=%d", invalidPathRes.Code, service.planCalls)
	}

	viewerMaterialize := httptest.NewRequest(http.MethodPost, "/api/v1/show-capsules/imports/"+capsuleID+"/materialize", nil)
	viewerMaterialize.RemoteAddr = "127.0.0.1:19109"
	viewerMaterialize.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerMaterialize.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerMaterializeRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerMaterializeRes, viewerMaterialize)
	if viewerMaterializeRes.Code != http.StatusForbidden || service.materializeCalls != 0 {
		t.Fatalf("viewer materialize status=%d calls=%d", viewerMaterializeRes.Code, service.materializeCalls)
	}

	ownerMaterialize := httptest.NewRequest(http.MethodPost, "/api/v1/show-capsules/imports/"+capsuleID+"/materialize", nil)
	ownerMaterialize.RemoteAddr = "127.0.0.1:19110"
	ownerMaterialize.Header.Set(csrfHeader, owner.CSRFToken)
	ownerMaterialize.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerMaterializeRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerMaterializeRes, ownerMaterialize)
	if ownerMaterializeRes.Code != http.StatusCreated || service.materializeCalls != 1 || service.lastImportedBy != "owner" {
		t.Fatalf("owner materialize status=%d calls=%d actor=%q body=%s", ownerMaterializeRes.Code, service.materializeCalls, service.lastImportedBy, ownerMaterializeRes.Body.String())
	}
}

func TestShowCapsuleExportRechecksSessionAtPublication(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeShowCapsuleService{onExport: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	handler := New(WithOperatorShowCapsules(h.auth, service, t.TempDir())).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/00000000-0000-7000-8000-000000000610/show-capsules",
		[]byte(`{"mode":"SELF_CONTAINED"}`), login.Token, login.CSRFToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || service.exportCalls != 1 || strings.Contains(rec.Body.String(), "capsule_id") {
		t.Fatalf("revoked export got HTTP %d, %d calls, %s", rec.Code, service.exportCalls, rec.Body.String())
	}
}

func TestShowCapsulePreviewSuppressesErrorsAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeShowCapsuleService{
		previewErr: errors.New("capsule metadata lookup failed"),
		onPreview: func() {
			if err := h.auth.Logout(ctx, login.Token); err != nil {
				t.Errorf("revoke Owner during capsule preview: %v", err)
			}
		},
	}
	handler := New(WithOperatorShowCapsules(h.auth, service, t.TempDir())).Handler()
	request := authenticatedReadRequest(http.MethodGet,
		"/api/v1/projects/00000000-0000-7000-8000-000000000610/show-capsule/preview", login.Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "SESSION_INVALID") ||
		strings.Contains(response.Body.String(), "PROJECTS_UNAVAILABLE") || service.previewCalls != 1 {
		t.Fatalf("revoked Owner received capsule preview error: status=%d calls=%d body=%s", response.Code, service.previewCalls, response.Body.String())
	}
}

func TestShowCapsulePlanSuppressesErrorsAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	const capsuleID = "00000000-0000-7000-8000-000000000611"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "imports", capsuleID), 0o750); err != nil {
		t.Fatal(err)
	}
	service := &fakeShowCapsuleService{
		planErr: errors.New("capsule readiness lookup failed"),
		onPlan: func() {
			if err := h.auth.Logout(ctx, login.Token); err != nil {
				t.Errorf("revoke Owner during capsule plan: %v", err)
			}
		},
	}
	handler := New(WithOperatorShowCapsules(h.auth, service, root)).Handler()
	request := authenticatedReadRequest(http.MethodGet,
		"/api/v1/show-capsules/imports/"+capsuleID+"/plan", login.Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "SESSION_INVALID") ||
		strings.Contains(response.Body.String(), "CAPSULE_VERIFICATION_FAILED") || service.planCalls != 1 {
		t.Fatalf("revoked Owner received capsule plan error: status=%d calls=%d body=%s", response.Code, service.planCalls, response.Body.String())
	}
}

func TestShowCapsuleExportSuppressesCommittedResultAfterFinalRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeShowCapsuleService{afterExportAuthorize: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	handler := New(WithOperatorShowCapsules(h.auth, service, t.TempDir())).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/00000000-0000-7000-8000-000000000610/show-capsules",
		[]byte(`{"mode":"SELF_CONTAINED"}`), login.Token, login.CSRFToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || service.exportCalls != 1 || strings.Contains(rec.Body.String(), "capsule_id") ||
		!strings.Contains(rec.Body.String(), `"effect_may_have_been_committed":true`) || !strings.Contains(rec.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked caller received exported capsule result: HTTP %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestShowCapsuleMaterializeSuppressesCommittedResultAfterFinalRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	const capsuleID = "00000000-0000-7000-8000-000000000611"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "imports", capsuleID), 0o750); err != nil {
		t.Fatal(err)
	}
	service := &fakeShowCapsuleService{afterMaterializeAuthorize: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	handler := New(WithOperatorShowCapsules(h.auth, service, root)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/show-capsules/imports/"+capsuleID+"/materialize", nil, login.Token, login.CSRFToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || service.materializeCalls != 1 || strings.Contains(rec.Body.String(), "project_id") ||
		!strings.Contains(rec.Body.String(), `"effect_may_have_been_committed":true`) || !strings.Contains(rec.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked caller received materialized capsule result: HTTP %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLegacyShowCapsulePlanWithholdsResultsAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	const capsuleID = "00000000-0000-7000-8000-000000000611"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "imports", capsuleID), 0o750); err != nil {
		t.Fatal(err)
	}
	service := &fakeShowCapsuleService{onPlan: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	handler := New(WithOperatorShowCapsules(h.auth, service, root)).Handler()
	req := authenticatedMutationRequest(t, http.MethodGet, "/api/v1/show-capsules/imports/"+capsuleID+"/plan", nil, login.Token, "")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || service.planCalls != 1 || strings.Contains(res.Body.String(), capsuleID) {
		t.Fatalf("revoked plan response=%d body=%s calls=%d", res.Code, res.Body, service.planCalls)
	}
}
