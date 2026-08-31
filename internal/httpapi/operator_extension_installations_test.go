package httpapi

import (
	"context"
	"encoding/json"
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

func TestOperatorExtensionInstallerRBACAndIntegrity(t *testing.T) {
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
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("operator install payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Register(ctx, pkg.ID, extensionTestManifest("LOCAL"), "owner"); err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(t.TempDir(), "extensions")
	installer, err := extension.NewInstaller(library, installRoot)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionInstaller(h.auth, installer, nil)).Handler()

	if _, err := h.auth.CreateUser(ctx, "install-viewer", "install viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "install-viewer", "install viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerList := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/installations", nil)
	viewerList.RemoteAddr = "127.0.0.1:19201"
	viewerList.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerListRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerListRes, viewerList)
	if viewerListRes.Code != http.StatusOK {
		t.Fatalf("VIEWER list status=%d body=%s", viewerListRes.Code, viewerListRes.Body.String())
	}

	viewerInstall := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/packages/"+pkg.ID+"/install", nil)
	viewerInstall.RemoteAddr = "127.0.0.1:19202"
	viewerInstall.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerInstall.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerInstallRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerInstallRes, viewerInstall)
	if viewerInstallRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER install status=%d want=403 body=%s", viewerInstallRes.Code, viewerInstallRes.Body.String())
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerInstall := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/packages/"+pkg.ID+"/install", nil)
	ownerInstall.RemoteAddr = "127.0.0.1:19203"
	ownerInstall.Header.Set(csrfHeader, owner.CSRFToken)
	ownerInstall.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerInstallRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerInstallRes, ownerInstall)
	if ownerInstallRes.Code != http.StatusCreated {
		t.Fatalf("OWNER install status=%d body=%s", ownerInstallRes.Code, ownerInstallRes.Body.String())
	}
	var installed extension.Installation
	if err := json.Unmarshal(ownerInstallRes.Body.Bytes(), &installed); err != nil {
		t.Fatal(err)
	}
	if installed.PackageID != pkg.ID || installed.LifecycleState != store.ExtensionInstallationInstalled || installed.ExtensionID != "example.osc-plugin" {
		t.Fatalf("installed=%+v", installed)
	}
	if strings.Contains(ownerInstallRes.Body.String(), `"enabled"`) || strings.Contains(ownerInstallRes.Body.String(), `"running"`) {
		t.Fatalf("installation response invented runtime state: %s", ownerInstallRes.Body.String())
	}

	ownerList := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/installations?extension_id=example.osc-plugin", nil)
	ownerList.RemoteAddr = "127.0.0.1:19204"
	ownerList.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerListRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerListRes, ownerList)
	if ownerListRes.Code != http.StatusOK || !strings.Contains(ownerListRes.Body.String(), installed.InstallationID) {
		t.Fatalf("OWNER list status=%d body=%s", ownerListRes.Code, ownerListRes.Body.String())
	}

	installedPath := filepath.Join(installRoot, "installed", filepath.FromSlash(installed.PayloadRelativePath))
	if err := os.Chmod(installedPath, 0o640); err != nil {
		t.Fatal(err)
	}
	integrityReq := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/installations/"+installed.InstallationID, nil)
	integrityReq.RemoteAddr = "127.0.0.1:19205"
	integrityReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	integrityRes := httptest.NewRecorder()
	handler.ServeHTTP(integrityRes, integrityReq)
	if integrityRes.Code != http.StatusConflict || !strings.Contains(integrityRes.Body.String(), "EXTENSION_INSTALL_INTEGRITY_FAILED") {
		t.Fatalf("tampered install status=%d body=%s", integrityRes.Code, integrityRes.Body.String())
	}
}
