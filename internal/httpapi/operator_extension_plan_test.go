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

func TestOperatorExtensionInstallPlanIsReadableAndInstallReturnsPlanBlocker(t *testing.T) {
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
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin-plan",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("operator dependency plan payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(
		string(extensionTestManifest("LOCAL")),
		`"capabilities":["osc.send"]`,
		`"capabilities":["osc.send"],"dependencies":[{"extension_id":"missing.helper","min_version":"1.0.0"}]`,
		1,
	)
	if _, err := library.Register(ctx, pkg.ID, []byte(manifest), "owner"); err != nil {
		t.Fatal(err)
	}
	installer, err := extension.NewInstaller(library, filepath.Join(t.TempDir(), "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionInstaller(h.auth, installer, nil)).Handler()

	if _, err := h.auth.CreateUser(ctx, "plan-viewer", "plan viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "plan-viewer", "plan viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	planReq := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/packages/"+pkg.ID+"/install-plan", nil)
	planReq.RemoteAddr = "127.0.0.1:19301"
	planReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	planRes := httptest.NewRecorder()
	handler.ServeHTTP(planRes, planReq)
	if planRes.Code != http.StatusOK || !strings.Contains(planRes.Body.String(), `"status":"BLOCKED"`) || !strings.Contains(planRes.Body.String(), `"code":"DEPENDENCY_UNAVAILABLE"`) {
		t.Fatalf("VIEWER plan status=%d body=%s", planRes.Code, planRes.Body.String())
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	installReq := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/packages/"+pkg.ID+"/install", nil)
	installReq.RemoteAddr = "127.0.0.1:19302"
	installReq.Header.Set(csrfHeader, owner.CSRFToken)
	installReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	installRes := httptest.NewRecorder()
	handler.ServeHTTP(installRes, installReq)
	if installRes.Code != http.StatusConflict || !strings.Contains(installRes.Body.String(), `"error_code":"EXTENSION_DEPENDENCY_PLAN_BLOCKED"`) || !strings.Contains(installRes.Body.String(), `"plan"`) || !strings.Contains(installRes.Body.String(), `"DEPENDENCY_UNAVAILABLE"`) {
		t.Fatalf("OWNER blocked install status=%d body=%s", installRes.Code, installRes.Body.String())
	}
}
