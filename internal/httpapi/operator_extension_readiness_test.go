package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOperatorExtensionReadinessViewerAndIntegrityBoundary(t *testing.T) {
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
		ProductID: "readiness.http-plugin", Version: "1.0.0", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "readiness-http-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("readiness HTTP payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
		"schema_version":1,
		"extension_id":"readiness.http-plugin",
		"version":"1.0.0",
		"kind":"PLUGIN",
		"source":"LOCAL",
		"name":{"en":"Readiness HTTP Plugin","ar-IQ":"إضافة اختبار جاهزية HTTP"},
		"summary":{"en":"Tests readiness HTTP boundaries.","ar-IQ":"تختبر حدود HTTP لتقييم الجاهزية."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]}
	}`)
	if _, err := library.Register(ctx, pkg.ID, manifest, "owner"); err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(t.TempDir(), "extensions")
	installer, err := extension.NewInstaller(library, installRoot)
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
	assessor, err := extension.NewReadinessAssessor(installer, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionReadiness(h.auth, assessor)).Handler()

	if _, err := h.auth.CreateUser(ctx, "readiness-viewer", "readiness viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "readiness-viewer", "readiness viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/installations/"+installed.InstallationID+"/readiness", nil)
	get.RemoteAddr = "127.0.0.1:19401"
	get.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, get)
	if getRes.Code != http.StatusOK || !strings.Contains(getRes.Body.String(), `"status":"READY_FOR_ACTIVATION"`) || !strings.Contains(getRes.Body.String(), `"code":"ACTIVATION_NOT_IMPLEMENTED"`) {
		t.Fatalf("VIEWER readiness status=%d body=%s", getRes.Code, getRes.Body.String())
	}
	if strings.Contains(getRes.Body.String(), `"running"`) || strings.Contains(getRes.Body.String(), `"enabled"`) {
		t.Fatalf("readiness response invented runtime state: %s", getRes.Body.String())
	}

	installedPath := filepath.Join(installRoot, "installed", filepath.FromSlash(installed.PayloadRelativePath))
	if err := os.Chmod(installedPath, 0o640); err != nil {
		t.Fatal(err)
	}
	tampered := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/installations/"+installed.InstallationID+"/readiness", nil)
	tampered.RemoteAddr = "127.0.0.1:19402"
	tampered.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	tamperedRes := httptest.NewRecorder()
	handler.ServeHTTP(tamperedRes, tampered)
	if tamperedRes.Code != http.StatusConflict || !strings.Contains(tamperedRes.Body.String(), "EXTENSION_READINESS_INTEGRITY_FAILED") {
		t.Fatalf("tampered readiness status=%d body=%s", tamperedRes.Code, tamperedRes.Body.String())
	}
}
