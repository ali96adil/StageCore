package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOperatorExtensionLibrarySuppressesResultsAfterOwnerRevocation(t *testing.T) {
	for _, route := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "register", method: http.MethodPost, path: "/api/v1/extensions/register"},
		{name: "list", method: http.MethodGet, path: "/api/v1/extensions"},
		{name: "get", method: http.MethodGet, path: "/api/v1/extensions/packages/"},
	} {
		t.Run(route.name, func(t *testing.T) {
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
			}, bytes.NewReader([]byte("extension payload")))
			if err != nil {
				t.Fatal(err)
			}
			library, err := extension.NewLibrary(stageStore, repository)
			if err != nil {
				t.Fatal(err)
			}
			if route.name != "register" {
				if _, err := library.Register(ctx, pkg.ID, extensionTestManifest("LOCAL"), "owner"); err != nil {
					t.Fatal(err)
				}
			}
			owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			handler := New(withOperatorExtensionLibrary(h.auth, library, nil, extensionLibraryHooks{
				BeforeResponse: func() {
					if err := h.auth.Logout(ctx, owner.Token); err != nil {
						t.Errorf("revoke Owner session: %v", err)
					}
				},
			})).Handler()

			var request *http.Request
			if route.method == http.MethodPost {
				body, err := json.Marshal(map[string]any{
					"package_id": pkg.ID,
					"manifest":   json.RawMessage(extensionTestManifest("LOCAL")),
				})
				if err != nil {
					t.Fatal(err)
				}
				request = authenticatedMutationRequest(t, route.method, route.path, body, owner.Token, owner.CSRFToken)
			} else {
				path := route.path
				if route.name == "get" {
					path += pkg.ID
				}
				request = httptest.NewRequest(route.method, path, nil)
				request.RemoteAddr = "127.0.0.1:19110"
				request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("revoked Owner received library result: HTTP %d body=%s", response.Code, response.Body.String())
			}
			if route.method == http.MethodPost {
				var body map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body["effect_may_have_been_committed"] != true || body["retryable"] != false {
					t.Fatalf("registration uncertainty was not reported safely: %+v", body)
				}
				items, err := library.List(ctx, "")
				if err != nil || len(items) != 1 {
					t.Fatalf("completed registration was lost: items=%+v err=%v", items, err)
				}
			}
		})
	}
}
