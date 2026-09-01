package httpapi

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestOperatorOfflineBundleAPIUsesPluginManageCSRFAndTrustedCatalog(t *testing.T) {
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
	importer, err := extension.NewOfflineBundleImporter(library)
	if err != nil {
		t.Fatal(err)
	}
	catalogRoot := t.TempDir()
	catalog, err := extension.NewTrustedCatalog(importer, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionOfflineBundles(h.auth, importer, catalog, nil)).Handler()

	if _, err := h.auth.CreateUser(ctx, "bundle-viewer", "bundle viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "bundle-viewer", "bundle viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	localBundle := buildHTTPAPIOfflineBundle(t, "LOCAL", []byte("local bundle payload"))
	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/import-bundle", bytes.NewReader(localBundle))
	viewerReq.RemoteAddr = "127.0.0.1:19300"
	viewerReq.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER import status=%d want=403 body=%s", viewerRes.Code, viewerRes.Body.String())
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	noCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/import-bundle", bytes.NewReader(localBundle))
	noCSRF.RemoteAddr = "127.0.0.1:19301"
	noCSRF.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	noCSRFRes := httptest.NewRecorder()
	handler.ServeHTTP(noCSRFRes, noCSRF)
	if noCSRFRes.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d want=403 body=%s", noCSRFRes.Code, noCSRFRes.Body.String())
	}

	ownerImport := func(bundle []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/import-bundle", bytes.NewReader(bundle))
		req.RemoteAddr = "127.0.0.1:19302"
		req.Header.Set(csrfHeader, owner.CSRFToken)
		req.Header.Set("Content-Type", "application/vnd.stagecore.extension-bundle")
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	localRes := ownerImport(localBundle)
	if localRes.Code != http.StatusCreated || !strings.Contains(localRes.Body.String(), `"source":"LOCAL"`) || !strings.Contains(localRes.Body.String(), `"trusted_official":false`) {
		t.Fatalf("OWNER local import status=%d body=%s", localRes.Code, localRes.Body.String())
	}

	officialUpload := ownerImport(buildHTTPAPIOfflineBundle(t, "OFFICIAL", []byte("untrusted official payload")))
	if officialUpload.Code != http.StatusForbidden || !strings.Contains(officialUpload.Body.String(), "EXTENSION_BUNDLE_SOURCE_FORBIDDEN") {
		t.Fatalf("self-asserted OFFICIAL status=%d body=%s", officialUpload.Code, officialUpload.Body.String())
	}

	officialCatalogBundle := buildHTTPAPIOfflineBundle(t, "OFFICIAL", []byte("trusted catalog payload"))
	if err := os.WriteFile(filepath.Join(catalogRoot, "official.scext"), officialCatalogBundle, 0o640); err != nil {
		t.Fatal(err)
	}
	syncReq := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/catalog/sync", nil)
	syncReq.RemoteAddr = "127.0.0.1:19303"
	syncReq.Header.Set(csrfHeader, owner.CSRFToken)
	syncReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	syncRes := httptest.NewRecorder()
	handler.ServeHTTP(syncRes, syncReq)
	if syncRes.Code != http.StatusOK || !strings.Contains(syncRes.Body.String(), `"extension_id":"example.osc-plugin"`) {
		t.Fatalf("catalog sync status=%d body=%s", syncRes.Code, syncRes.Body.String())
	}
}

func buildHTTPAPIOfflineBundle(t *testing.T, source string, payload []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(payload)
	metadata := extension.OfflineBundleMetadata{
		Format: extension.OfflineBundleFormatV1,
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease, PayloadSHA256: hex.EncodeToString(sum[:]), PayloadSizeBytes: int64(len(payload)),
	}
	metadataRaw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	manifest := extensionTestManifest(source)
	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)
	write := func(name string, content []byte) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	write(extension.OfflineBundleMetadataName, metadataRaw)
	write(extension.OfflineBundleManifestName, manifest)
	write(extension.OfflineBundlePayloadName, payload)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
