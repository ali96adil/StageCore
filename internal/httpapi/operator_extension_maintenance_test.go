package httpapi

import (
	"bytes"
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

func TestOperatorExtensionMaintenanceUpdateRollbackRepairRBACAndCSRF(t *testing.T) {
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
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}

	importVersion := func(version, payload string) string {
		t.Helper()
		pkg, err := repository.ImportPackage(ctx, software.ImportParams{
			ProductID: "example.maintenance-plugin", Version: version, Platform: "linux", Architecture: "arm64",
			MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "maintenance-plugin-" + version,
			SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
			ReleaseChannel: store.SoftwareChannelRelease,
		}, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := library.Register(ctx, pkg.ID, extensionMaintenanceManifest(version), "owner"); err != nil {
			t.Fatal(err)
		}
		return pkg.ID
	}
	v1PackageID := importVersion("1.0.0", "maintenance payload v1")
	v2PackageID := importVersion("2.0.0", "maintenance payload v2")

	extensionRoot := filepath.Join(t.TempDir(), "extensions")
	installer, err := extension.NewInstaller(library, extensionRoot)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installer.InstallPlanned(ctx, v1PackageID, "owner")
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
	base := "/api/v1/extensions/installations/" + installed.InstallationID

	if _, err := h.auth.CreateUser(ctx, "maintenance-viewer", "maintenance viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "maintenance-viewer", "maintenance viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerPlan := httptest.NewRequest(http.MethodGet, base+"/update-plan?target_package_id="+v2PackageID, nil)
	viewerPlan.RemoteAddr = "127.0.0.1:19401"
	viewerPlan.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerPlanRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerPlanRes, viewerPlan)
	if viewerPlanRes.Code != http.StatusOK || !strings.Contains(viewerPlanRes.Body.String(), `"direction":"UPDATE"`) || !strings.Contains(viewerPlanRes.Body.String(), `"status":"READY"`) {
		t.Fatalf("VIEWER plan status=%d body=%s", viewerPlanRes.Code, viewerPlanRes.Body.String())
	}

	updateBody, _ := json.Marshal(map[string]string{"target_package_id": v2PackageID})
	viewerUpdate := httptest.NewRequest(http.MethodPost, base+"/update", bytes.NewReader(updateBody))
	viewerUpdate.RemoteAddr = "127.0.0.1:19402"
	viewerUpdate.Header.Set("Content-Type", "application/json")
	viewerUpdate.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerUpdate.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerUpdateRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerUpdateRes, viewerUpdate)
	if viewerUpdateRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER update status=%d want=403 body=%s", viewerUpdateRes.Code, viewerUpdateRes.Body.String())
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	missingCSRF := httptest.NewRequest(http.MethodPost, base+"/update", bytes.NewReader(updateBody))
	missingCSRF.RemoteAddr = "127.0.0.1:19403"
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	missingCSRFRes := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFRes, missingCSRF)
	if missingCSRFRes.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF update status=%d want=403 body=%s", missingCSRFRes.Code, missingCSRFRes.Body.String())
	}

	ownerMutation := func(path string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:19404"
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set(csrfHeader, owner.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	updateRes := ownerMutation(base+"/update", updateBody)
	if updateRes.Code != http.StatusOK || !strings.Contains(updateRes.Body.String(), `"direction":"UPDATE"`) || !strings.Contains(updateRes.Body.String(), `"to_version":"2.0.0"`) {
		t.Fatalf("OWNER update status=%d body=%s", updateRes.Code, updateRes.Body.String())
	}
	updated, err := installer.Get(ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PackageID != v2PackageID || updated.InstallationID != installed.InstallationID {
		t.Fatalf("updated installation=%+v", updated)
	}

	rollbackBody, _ := json.Marshal(map[string]string{"target_package_id": v1PackageID})
	rollbackRes := ownerMutation(base+"/update", rollbackBody)
	if rollbackRes.Code != http.StatusOK || !strings.Contains(rollbackRes.Body.String(), `"direction":"ROLLBACK"`) || !strings.Contains(rollbackRes.Body.String(), `"to_version":"1.0.0"`) {
		t.Fatalf("OWNER rollback status=%d body=%s", rollbackRes.Code, rollbackRes.Body.String())
	}
	rolledBack, err := installer.Get(ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}

	payloadPath := filepath.Join(extensionRoot, "installed", filepath.FromSlash(rolledBack.PayloadRelativePath))
	if err := os.Chmod(payloadPath, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payloadPath, []byte("tampered payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(payloadPath, 0o440); err != nil {
		t.Fatal(err)
	}

	repairRes := ownerMutation(base+"/repair", nil)
	if repairRes.Code != http.StatusOK || !strings.Contains(repairRes.Body.String(), `"payload_repaired":true`) {
		t.Fatalf("OWNER repair status=%d body=%s", repairRes.Code, repairRes.Body.String())
	}
	if _, err := installer.Get(ctx, installed.InstallationID); err != nil {
		t.Fatalf("repaired installation verification failed: %v", err)
	}
}

func extensionMaintenanceManifest(version string) []byte {
	return []byte(`{
		"schema_version":1,
		"extension_id":"example.maintenance-plugin",
		"version":"` + version + `",
		"kind":"PLUGIN",
		"source":"LOCAL",
		"name":{"en":"Maintenance Plugin","ar-IQ":"إضافة صيانة"},
		"summary":{"en":"Exercises safe maintenance.","ar-IQ":"تختبر الصيانة الآمنة."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},
		"permissions":[],
		"capabilities":[]
	}`)
}
