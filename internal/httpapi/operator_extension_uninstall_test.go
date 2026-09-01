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
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOperatorExtensionUninstallRequiresManagePermissionCSRFAndDisabledRuntime(t *testing.T) {
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
	softwarePackage, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("operator uninstall payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Register(ctx, softwarePackage.ID, extensionTestManifest("LOCAL"), "owner"); err != nil {
		t.Fatal(err)
	}
	installer, err := extension.NewInstaller(library, filepath.Join(t.TempDir(), "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installer.InstallPlanned(ctx, softwarePackage.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := extension.NewPermissionReviewer(installer)
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := extension.NewReadinessAssessor(installer, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	isolator, err := extension.NewRuntimeIsolator(installer, reviewer, readiness, "")
	if err != nil {
		t.Fatal(err)
	}
	probe, err := extension.NewRuntimeProbe(installer, isolator)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := extension.NewRuntimeSupervisor(installer, isolator, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer supervisor.Close()

	handler := New(WithOperatorExtensionRuntimeLifecycle(h.auth, supervisor, nil)).Handler()
	endpoint := "/api/v1/extensions/installations/" + installed.InstallationID

	if _, err := h.auth.CreateUser(ctx, "uninstall-viewer", "uninstall viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "uninstall-viewer", "uninstall viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerReq := httptest.NewRequest(http.MethodDelete, endpoint, nil)
	viewerReq.RemoteAddr = "127.0.0.1:19301"
	viewerReq.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER uninstall status=%d want=403 body=%s", viewerRes.Code, viewerRes.Body.String())
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	missingCSRF := httptest.NewRequest(http.MethodDelete, endpoint, nil)
	missingCSRF.RemoteAddr = "127.0.0.1:19302"
	missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	missingCSRFRes := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRes, missingCSRF)
	if missingCSRFRes.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF uninstall status=%d want=403 body=%s", missingCSRFRes.Code, missingCSRFRes.Body.String())
	}

	ownerReq := httptest.NewRequest(http.MethodDelete, endpoint, nil)
	ownerReq.RemoteAddr = "127.0.0.1:19303"
	ownerReq.Header.Set(csrfHeader, owner.CSRFToken)
	ownerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerRes, ownerReq)
	if ownerRes.Code != http.StatusOK {
		t.Fatalf("OWNER uninstall status=%d body=%s", ownerRes.Code, ownerRes.Body.String())
	}
	if !strings.Contains(ownerRes.Body.String(), `"payload_removed":true`) || !strings.Contains(ownerRes.Body.String(), installed.InstallationID) {
		t.Fatalf("OWNER uninstall body=%s", ownerRes.Body.String())
	}

	notFoundReq := httptest.NewRequest(http.MethodDelete, endpoint, nil)
	notFoundReq.RemoteAddr = "127.0.0.1:19304"
	notFoundReq.Header.Set(csrfHeader, owner.CSRFToken)
	notFoundReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	notFoundRes := httptest.NewRecorder()
	handler.ServeHTTP(notFoundRes, notFoundReq)
	if notFoundRes.Code != http.StatusNotFound || !strings.Contains(notFoundRes.Body.String(), "EXTENSION_INSTALLATION_NOT_FOUND") {
		t.Fatalf("second uninstall status=%d body=%s", notFoundRes.Code, notFoundRes.Body.String())
	}
}
