package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOperatorExtensionPermissionReviewRBACAndGrantSeparation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	v, err := vault.Open(t.TempDir(), stageStore)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(v, stageStore, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "review.http-plugin", Version: "1.0.0", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "review-http-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("permission review HTTP payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
		"schema_version":1,
		"extension_id":"review.http-plugin",
		"version":"1.0.0",
		"kind":"PLUGIN",
		"source":"LOCAL",
		"name":{"en":"Review HTTP Plugin","ar-IQ":"إضافة اختبار مراجعة HTTP"},
		"summary":{"en":"Tests permission review HTTP boundaries.","ar-IQ":"تختبر حدود HTTP لمراجعة الصلاحيات."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},
		"permissions":["network.udp.send"]
	}`)
	if _, err := library.Register(ctx, pkg.ID, manifest, "owner"); err != nil {
		t.Fatal(err)
	}
	installer, err := extension.NewInstaller(library, filepath.Join(t.TempDir(), "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installer.InstallPlanned(ctx, pkg.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := extension.NewPermissionReviewer(installer)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionPermissionReview(h.auth, reviewer, nil)).Handler()

	if _, err := h.auth.CreateUser(ctx, "review-viewer", "review viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "review-viewer", "review viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerGet := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/installations/"+installed.InstallationID+"/permission-review", nil)
	viewerGet.RemoteAddr = "127.0.0.1:19301"
	viewerGet.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerGetRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerGetRes, viewerGet)
	if viewerGetRes.Code != http.StatusOK || !strings.Contains(viewerGetRes.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("VIEWER review status=%d body=%s", viewerGetRes.Code, viewerGetRes.Body.String())
	}

	viewerPut := httptest.NewRequest(http.MethodPut, "/api/v1/extensions/installations/"+installed.InstallationID+"/permissions/network.udp.send", strings.NewReader(`{"decision":"APPROVED"}`))
	viewerPut.RemoteAddr = "127.0.0.1:19302"
	viewerPut.Header.Set("Content-Type", "application/json")
	viewerPut.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerPut.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerPutRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerPutRes, viewerPut)
	if viewerPutRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER decision status=%d body=%s", viewerPutRes.Code, viewerPutRes.Body.String())
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerPut := httptest.NewRequest(http.MethodPut, "/api/v1/extensions/installations/"+installed.InstallationID+"/permissions/network.udp.send", strings.NewReader(`{"decision":"APPROVED"}`))
	ownerPut.RemoteAddr = "127.0.0.1:19303"
	ownerPut.Header.Set("Content-Type", "application/json")
	ownerPut.Header.Set(csrfHeader, owner.CSRFToken)
	ownerPut.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerPutRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerPutRes, ownerPut)
	if ownerPutRes.Code != http.StatusOK || !strings.Contains(ownerPutRes.Body.String(), `"status":"APPROVED"`) {
		t.Fatalf("OWNER decision status=%d body=%s", ownerPutRes.Code, ownerPutRes.Body.String())
	}

	permissionService, err := pluginpermissions.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	granted, err := permissionService.Granted(ctx, "review.http-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 0 {
		t.Fatalf("HTTP permission review leaked runtime grants: %v", granted)
	}

	invalidPut := httptest.NewRequest(http.MethodPut, "/api/v1/extensions/installations/"+installed.InstallationID+"/permissions/filesystem.write", strings.NewReader(`{"decision":"APPROVED"}`))
	invalidPut.RemoteAddr = "127.0.0.1:19304"
	invalidPut.Header.Set("Content-Type", "application/json")
	invalidPut.Header.Set(csrfHeader, owner.CSRFToken)
	invalidPut.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	invalidPutRes := httptest.NewRecorder()
	handler.ServeHTTP(invalidPutRes, invalidPut)
	if invalidPutRes.Code != http.StatusBadRequest || !strings.Contains(invalidPutRes.Body.String(), "EXTENSION_PERMISSION_NOT_REQUESTED") {
		t.Fatalf("unrequested permission status=%d body=%s", invalidPutRes.Code, invalidPutRes.Body.String())
	}
}
