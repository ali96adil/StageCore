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

type revokingExtensionMaintenanceSupervisor struct {
	extensionMaintenanceSupervisor
	update func(context.Context, string, string, string) (extension.UpdateResult, error)
	repair func(context.Context, string, string) (extension.RepairResult, error)
}

func (s revokingExtensionMaintenanceSupervisor) UpdateInstallation(ctx context.Context, installationID, targetPackageID, actor string) (extension.UpdateResult, error) {
	return s.update(ctx, installationID, targetPackageID, actor)
}

func (s revokingExtensionMaintenanceSupervisor) RepairInstallation(ctx context.Context, installationID, actor string) (extension.RepairResult, error) {
	return s.repair(ctx, installationID, actor)
}

func TestExtensionMaintenanceSuppressesCommittedUpdateAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	applied := false
	supervisor := revokingExtensionMaintenanceSupervisor{update: func(_ context.Context, installationID, targetPackageID, actor string) (extension.UpdateResult, error) {
		applied = true // Model an update that completed before the HTTP response.
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			return extension.UpdateResult{}, err
		}
		return extension.UpdateResult{
			InstallationID: installationID, ExtensionID: "test.extension", Direction: extension.UpdateDirectionUpdate,
			FromPackageID: "package-old", FromVersion: "1.0.0", ToPackageID: targetPackageID, ToVersion: "2.0.0",
		}, nil
	}}
	server := New()
	registerOperatorExtensionMaintenanceRoutes(server.mux, h.auth, supervisor, nil)
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/extensions/installations/installation-1/update",
		[]byte(`{"target_package_id":"package-new"}`), owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if !applied {
		t.Fatal("test update did not complete before Owner session revocation")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || body["error_code"] != "EXTENSION_MAINTENANCE_AUTHORITY_REVOKED" ||
		body["effect_may_have_been_applied"] != true || body["retryable"] != false {
		t.Fatalf("revoked Owner received success or retryable update response: HTTP %d %+v", response.Code, body)
	}
	if _, leaked := body["installation_id"]; leaked {
		t.Fatalf("revoked Owner received committed extension update record: %+v", body)
	}
}

func TestExtensionMaintenanceDoesNotDispatchUpdateAfterRevocationWhileReadingBody(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	dispatched := false
	supervisor := revokingExtensionMaintenanceSupervisor{update: func(context.Context, string, string, string) (extension.UpdateResult, error) {
		dispatched = true
		return extension.UpdateResult{InstallationID: "installation-1"}, nil
	}}
	server := New()
	registerOperatorExtensionMaintenanceRoutes(server.mux, h.auth, supervisor, nil)
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/extensions/installations/installation-1/update",
		[]byte(`{"target_package_id":"package-new"}`), owner.Token, owner.CSRFToken)
	request.Body = &firmwareUploadRevokeReader{ReadCloser: request.Body, revoke: func() {
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			t.Errorf("revoke Owner while request body is read: %v", err)
		}
	}}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if dispatched {
		t.Fatal("extension update was dispatched after its Owner session was revoked while reading the request body")
	}
	if response.Code != http.StatusForbidden {
		t.Fatalf("revoked Owner update response=%d want=%d body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["effect_may_have_been_applied"] != false || body["retryable"] != false || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("pre-dispatch denial response overstated or cached the result: headers=%v body=%+v", response.Header(), body)
	}
}

func TestExtensionMaintenanceSuppressesCommittedRepairAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	repaired := false
	supervisor := revokingExtensionMaintenanceSupervisor{repair: func(_ context.Context, installationID, actor string) (extension.RepairResult, error) {
		repaired = true
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			return extension.RepairResult{}, err
		}
		return extension.RepairResult{InstallationID: installationID, PackageID: "package-1", ExtensionID: "test.extension", Version: "1.0.0", PayloadRepaired: true}, nil
	}}
	server := New()
	registerOperatorExtensionMaintenanceRoutes(server.mux, h.auth, supervisor, nil)
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/extensions/installations/installation-1/repair", nil, owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if !repaired {
		t.Fatal("test repair did not complete before Owner session revocation")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || body["error_code"] != "EXTENSION_MAINTENANCE_AUTHORITY_REVOKED" ||
		body["effect_may_have_been_applied"] != true || body["retryable"] != false {
		t.Fatalf("revoked Owner received success or retryable repair response: HTTP %d %+v", response.Code, body)
	}
	if _, leaked := body["package_id"]; leaked {
		t.Fatalf("revoked Owner received committed extension repair record: %+v", body)
	}
}

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
	runtimeRevoked := false
	runtimeHandler := New(withOperatorExtensionRuntimeLifecycle(h.auth, supervisor, nil, extensionRuntimeLifecycleHooks{
		BeforeReadResponse: func() {
			runtimeRevoked = true
			if err := h.auth.Logout(ctx, owner.Token); err != nil {
				t.Errorf("revoke Owner before runtime status response: %v", err)
			}
		},
	})).Handler()
	runtimeRequest := httptest.NewRequest(http.MethodGet, base+"/runtime", nil)
	runtimeRequest.RemoteAddr = "127.0.0.1:19405"
	runtimeRequest.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	runtimeResponse := httptest.NewRecorder()
	runtimeHandler.ServeHTTP(runtimeResponse, runtimeRequest)
	if !runtimeRevoked {
		t.Fatal("test did not revoke Owner after extension runtime status read")
	}
	if runtimeResponse.Code != http.StatusForbidden || strings.Contains(runtimeResponse.Body.String(), installed.InstallationID) || runtimeResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoked Owner received runtime status: status=%d headers=%v body=%s", runtimeResponse.Code, runtimeResponse.Header(), runtimeResponse.Body.String())
	}
	owner, err = h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
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
