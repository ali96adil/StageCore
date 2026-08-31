package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOperatorExtensionLibraryRBACAndTrustBoundary(t *testing.T) {
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
	}, strings.NewReader("operator extension payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorExtensionLibrary(h.auth, library, nil)).Handler()

	unauthenticatedReq := httptest.NewRequest(http.MethodGet, "/api/v1/extensions", nil)
	unauthenticatedReq.RemoteAddr = "127.0.0.1:19100"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticatedReq)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status=%d want=401", unauthenticatedRes.Code)
	}

	if _, err := h.auth.CreateUser(ctx, "extension-viewer", "extension viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "extension-viewer", "extension viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerList := httptest.NewRequest(http.MethodGet, "/api/v1/extensions", nil)
	viewerList.RemoteAddr = "127.0.0.1:19101"
	viewerList.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerListRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerListRes, viewerList)
	if viewerListRes.Code != http.StatusOK {
		t.Fatalf("VIEWER list status=%d body=%s", viewerListRes.Code, viewerListRes.Body.String())
	}

	localManifest := extensionTestManifest("LOCAL")
	viewerRegisterBody, _ := json.Marshal(map[string]any{"package_id": pkg.ID, "manifest": json.RawMessage(localManifest)})
	viewerRegister := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/register", bytes.NewReader(viewerRegisterBody))
	viewerRegister.RemoteAddr = "127.0.0.1:19102"
	viewerRegister.Header.Set("Content-Type", "application/json")
	viewerRegister.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerRegister.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerRegisterRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRegisterRes, viewerRegister)
	if viewerRegisterRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER register status=%d want=403", viewerRegisterRes.Code)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	requestOwner := func(manifest []byte) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{"package_id": pkg.ID, "manifest": json.RawMessage(manifest)})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extensions/register", bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:19103"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, owner.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	officialRes := requestOwner(extensionTestManifest("OFFICIAL"))
	if officialRes.Code != http.StatusForbidden || !strings.Contains(officialRes.Body.String(), "EXTENSION_OFFICIAL_SOURCE_FORBIDDEN") {
		t.Fatalf("self-asserted OFFICIAL status=%d body=%s", officialRes.Code, officialRes.Body.String())
	}

	registerRes := requestOwner(localManifest)
	if registerRes.Code != http.StatusCreated {
		t.Fatalf("OWNER register status=%d body=%s", registerRes.Code, registerRes.Body.String())
	}
	var registered extension.Package
	if err := json.Unmarshal(registerRes.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.PackageID != pkg.ID || registered.Manifest.Name.ArIQ == "" || registered.Manifest.Source != extension.SourceLocal {
		t.Fatalf("registered=%+v", registered)
	}

	ownerList := httptest.NewRequest(http.MethodGet, "/api/v1/extensions?extension_id=example.osc-plugin", nil)
	ownerList.RemoteAddr = "127.0.0.1:19104"
	ownerList.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	ownerListRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerListRes, ownerList)
	if ownerListRes.Code != http.StatusOK || !strings.Contains(ownerListRes.Body.String(), `"ar-IQ"`) || !strings.Contains(ownerListRes.Body.String(), pkg.ID) {
		t.Fatalf("OWNER list status=%d body=%s", ownerListRes.Code, ownerListRes.Body.String())
	}
}

func extensionTestManifest(source string) []byte {
	return []byte(`{
		"schema_version":1,
		"extension_id":"example.osc-plugin",
		"version":"1.2.3",
		"kind":"PLUGIN",
		"source":"` + source + `",
		"name":{"en":"Example OSC Plugin","ar-IQ":"إضافة OSC تجريبية"},
		"summary":{"en":"Sends OSC messages.","ar-IQ":"ترسل رسائل OSC إلى أجهزة المسرح."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},
		"permissions":["network.udp.send"],
		"capabilities":["osc.send"]
	}`)
}
