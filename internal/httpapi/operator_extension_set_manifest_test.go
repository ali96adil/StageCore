package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestOperatorExtensionSetManifestRBACPlanAndRestore(t *testing.T) {
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
	installer, err := extension.NewInstaller(library, filepath.Join(t.TempDir(), "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	// The set-manifest routes must be registered transitively by the production
	// installer option; this test intentionally does not add a separate option.
	handler := New(WithOperatorExtensionInstaller(h.auth, installer, nil)).Handler()

	unauth := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/set-manifest", nil)
	unauth.RemoteAddr = "127.0.0.1:19600"
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated export status=%d want=401 body=%s", unauthRes.Code, unauthRes.Body.String())
	}

	if _, err := h.auth.CreateUser(ctx, "set-viewer", "set viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "set-viewer", "set viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerExport := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/set-manifest", nil)
	viewerExport.RemoteAddr = "127.0.0.1:19601"
	viewerExport.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerExportRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerExportRes, viewerExport)
	if viewerExportRes.Code != http.StatusOK || viewerExportRes.Header().Get("Content-Disposition") == "" {
		t.Fatalf("VIEWER export status=%d headers=%v body=%s", viewerExportRes.Code, viewerExportRes.Header(), viewerExportRes.Body.String())
	}
	var exported extension.ExtensionSetManifest
	if err := json.Unmarshal(viewerExportRes.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Format != extension.ExtensionSetFormatV1 || exported.SchemaVersion != extension.ExtensionSetSchemaVersion || len(exported.Extensions) != 0 {
		t.Fatalf("exported=%+v", exported)
	}

	emptyRaw, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	viewerRestore := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/set-manifest/restore", bytes.NewReader(emptyRaw))
	viewerRestore.RemoteAddr = "127.0.0.1:19602"
	viewerRestore.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerRestore.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerRestoreRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRestoreRes, viewerRestore)
	if viewerRestoreRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER restore status=%d want=403 body=%s", viewerRestoreRes.Code, viewerRestoreRes.Body.String())
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerRequest := func(path string, raw []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.RemoteAddr = "127.0.0.1:19603"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, owner.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	planRes := ownerRequest("/api/v1/extensions/set-manifest/restore-plan", emptyRaw)
	if planRes.Code != http.StatusOK || !strings.Contains(planRes.Body.String(), `"status":"NOOP"`) {
		t.Fatalf("OWNER empty plan status=%d body=%s", planRes.Code, planRes.Body.String())
	}
	restoreRes := ownerRequest("/api/v1/extensions/set-manifest/restore", emptyRaw)
	if restoreRes.Code != http.StatusOK || !strings.Contains(restoreRes.Body.String(), `"runtime_intent_restored":false`) || !strings.Contains(restoreRes.Body.String(), `"permission_reviews_restored":false`) {
		t.Fatalf("OWNER empty restore status=%d body=%s", restoreRes.Code, restoreRes.Body.String())
	}

	hash := strings.Repeat("a", 64)
	blockedRaw, err := json.Marshal(extension.ExtensionSetManifest{
		Format: extension.ExtensionSetFormatV1,
		SchemaVersion: extension.ExtensionSetSchemaVersion,
		Extensions: []extension.ExtensionSetEntry{{
			ExtensionID: "example.missing-addon",
			Version: "1.0.0",
			Kind: extension.KindAddon,
			Source: extension.SourceLocal,
			ManifestSHA256: hash,
			PayloadSHA256: hash,
			PayloadSizeBytes: 1,
			Platform: "linux",
			Architecture: "arm64",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedPlanRes := ownerRequest("/api/v1/extensions/set-manifest/restore-plan", blockedRaw)
	if blockedPlanRes.Code != http.StatusOK || !strings.Contains(blockedPlanRes.Body.String(), `"status":"BLOCKED"`) || !strings.Contains(blockedPlanRes.Body.String(), "EXACT_PACKAGE_NOT_AVAILABLE") {
		t.Fatalf("blocked plan status=%d body=%s", blockedPlanRes.Code, blockedPlanRes.Body.String())
	}
	blockedRestoreRes := ownerRequest("/api/v1/extensions/set-manifest/restore", blockedRaw)
	if blockedRestoreRes.Code != http.StatusConflict || !strings.Contains(blockedRestoreRes.Body.String(), "EXTENSION_SET_RESTORE_BLOCKED") || !strings.Contains(blockedRestoreRes.Body.String(), "EXACT_PACKAGE_NOT_AVAILABLE") {
		t.Fatalf("blocked restore status=%d body=%s", blockedRestoreRes.Code, blockedRestoreRes.Body.String())
	}

	invalidRes := ownerRequest("/api/v1/extensions/set-manifest/restore-plan", []byte(`{"format":"wrong","schema_version":1,"extensions":[]}`))
	if invalidRes.Code != http.StatusBadRequest || !strings.Contains(invalidRes.Body.String(), "EXTENSION_SET_INVALID") {
		t.Fatalf("invalid plan status=%d body=%s", invalidRes.Code, invalidRes.Body.String())
	}

	tooLarge := []byte(strings.Repeat("x", extension.MaxExtensionSetManifestSize+2))
	tooLargeRes := ownerRequest("/api/v1/extensions/set-manifest/restore-plan", tooLarge)
	if tooLargeRes.Code != http.StatusRequestEntityTooLarge || !strings.Contains(tooLargeRes.Body.String(), "EXTENSION_SET_TOO_LARGE") {
		t.Fatalf("oversized plan status=%d body=%s", tooLargeRes.Code, tooLargeRes.Body.String())
	}
}
