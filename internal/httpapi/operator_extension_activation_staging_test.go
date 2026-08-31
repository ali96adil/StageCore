package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOperatorExtensionActivationStagingRBACReadinessAndNoExecution(t *testing.T) {
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
		ProductID: "staging.http-plugin", Version: "1.0.0", Platform: "linux", Architecture: runtime.GOARCH,
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "staging-http-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader(minimalHTTPPluginELF(t)))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := extension.Manifest{
		SchemaVersion: extension.ManifestSchemaVersion,
		ExtensionID: "staging.http-plugin",
		Version: "1.0.0",
		Kind: extension.KindPlugin,
		Source: extension.SourceLocal,
		Name: extension.LocalizedText{EN: "Staging HTTP Plugin", ArIQ: "إضافة اختبار تجهيز HTTP"},
		Summary: extension.LocalizedText{EN: "Tests activation staging HTTP boundaries.", ArIQ: "تختبر حدود HTTP لتجهيز التفعيل."},
		Compatibility: extension.Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{runtime.GOARCH}},
		Permissions: []string{"network.udp.send"},
		Capabilities: []string{"test.execute"},
		Runtime: &extension.RuntimeContract{
			Protocol: extension.RuntimeProtocolPluginV1,
			Artifact: extension.RuntimeArtifactNativeExecutable,
			CapabilityPermissions: map[string][]string{"test.execute": {"network.udp.send"}},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Register(ctx, pkg.ID, raw, "owner"); err != nil {
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
	stager, err := extension.NewActivationStager(installer, reviewer, assessor)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionActivationStaging(h.auth, stager, nil)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	pendingReq := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/installations/"+installed.InstallationID+"/activation-staging-check", nil)
	pendingReq.RemoteAddr = "127.0.0.1:19601"
	pendingReq.Header.Set(csrfHeader, owner.CSRFToken)
	pendingReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	pendingRes := httptest.NewRecorder()
	handler.ServeHTTP(pendingRes, pendingReq)
	if pendingRes.Code != http.StatusConflict || !strings.Contains(pendingRes.Body.String(), "EXTENSION_ACTIVATION_NOT_READY") || !strings.Contains(pendingRes.Body.String(), "PERMISSION_REVIEW_PENDING") {
		t.Fatalf("pending staging status=%d body=%s", pendingRes.Code, pendingRes.Body.String())
	}

	if _, err := h.auth.CreateUser(ctx, "staging-viewer", "staging viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "staging-viewer", "staging viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/installations/"+installed.InstallationID+"/activation-staging-check", nil)
	viewerReq.RemoteAddr = "127.0.0.1:19602"
	viewerReq.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER staging status=%d body=%s", viewerRes.Code, viewerRes.Body.String())
	}

	if _, err := reviewer.Decide(ctx, installed.InstallationID, "network.udp.send", extension.PermissionDecisionApproved, "owner"); err != nil {
		t.Fatal(err)
	}
	checkReq := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/installations/"+installed.InstallationID+"/activation-staging-check", nil)
	checkReq.RemoteAddr = "127.0.0.1:19603"
	checkReq.Header.Set(csrfHeader, owner.CSRFToken)
	checkReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	checkRes := httptest.NewRecorder()
	handler.ServeHTTP(checkRes, checkReq)
	body := checkRes.Body.String()
	if checkRes.Code != http.StatusOK || !strings.Contains(body, `"status":"STAGING_VERIFIED"`) || !strings.Contains(body, `"execution_authorized":false`) || !strings.Contains(body, extension.ActivationExecutionIsolationRequired) {
		t.Fatalf("OWNER staging status=%d body=%s", checkRes.Code, body)
	}
	if strings.Contains(body, `"enabled"`) || strings.Contains(body, `"running"`) || strings.Contains(body, `"plugin_ready"`) {
		t.Fatalf("activation staging invented execution state: %s", body)
	}
	stageEntries, err := os.ReadDir(filepath.Join(installRoot, "runtime", "staging-check"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stageEntries) != 0 {
		t.Fatalf("HTTP activation staging left files: %v", stageEntries)
	}
}
