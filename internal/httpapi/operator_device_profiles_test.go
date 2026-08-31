package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceprofile"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOperatorDeviceProfileCatalogIsAuthenticatedAndMaterializesTypedTarget(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	catalog := deviceprofile.BuiltinCatalog()
	handler := New(WithOperatorDeviceProfiles(h.auth, catalog)).Handler()

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/device-profiles", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated catalog status=%d want=401", unauthorized.Code)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerRequest := func(method, path string, body any) *httptest.ResponseRecorder {
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &payload)
		req.RemoteAddr = "127.0.0.1:19001"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if method != http.MethodGet {
			req.Header.Set(csrfHeader, owner.CSRFToken)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	list := ownerRequest(http.MethodGet, "/api/v1/device-profiles", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		SchemaVersion int                     `json:"schema_version"`
		Profiles      []deviceprofile.Profile `json:"profiles"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.SchemaVersion != deviceprofile.CatalogSchemaVersion || len(listed.Profiles) != 1 {
		t.Fatalf("catalog=%#v", listed)
	}
	if listed.Profiles[0].Name.EN == "" || listed.Profiles[0].Name.ArIQ == "" {
		t.Fatalf("profile localization missing: %#v", listed.Profiles[0].Name)
	}

	materialized := ownerRequest(http.MethodPost, "/api/v1/device-profiles/stagecore.generic.osc-udp/materialize", map[string]any{
		"values": map[string]any{"host": "projector.local", "port": 9010},
	})
	if materialized.Code != http.StatusOK {
		t.Fatalf("materialize status=%d body=%s", materialized.Code, materialized.Body.String())
	}
	var result struct {
		Target deviceprofile.MaterializedTarget `json:"target"`
	}
	if err := json.Unmarshal(materialized.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Target.LogicalType != "GENERIC" {
		t.Fatalf("logical type=%q", result.Target.LogicalType)
	}
	var cfg struct {
		OSC struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"osc"`
	}
	if err := json.Unmarshal(result.Target.Configuration, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.OSC.Host != "projector.local" || cfg.OSC.Port != 9010 {
		t.Fatalf("materialized config=%#v", cfg)
	}

	match := ownerRequest(http.MethodPost, "/api/v1/device-profiles/match", map[string]any{
		"attributes": map[string]string{"protocol": "osc"},
	})
	if match.Code != http.StatusOK {
		t.Fatalf("match status=%d body=%s", match.Code, match.Body.String())
	}
	var matchBody struct {
		Matches []deviceprofile.MatchCandidate `json:"matches"`
	}
	if err := json.Unmarshal(match.Body.Bytes(), &matchBody); err != nil {
		t.Fatal(err)
	}
	if len(matchBody.Matches) != 0 {
		t.Fatalf("generic manual profile must not auto-match: %#v", matchBody.Matches)
	}

	if _, err := h.auth.CreateUser(ctx, "profile-viewer", "profile viewer password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "profile-viewer", "profile viewer password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerList := httptest.NewRequest(http.MethodGet, "/api/v1/device-profiles", nil)
	viewerList.RemoteAddr = "127.0.0.1:19002"
	viewerList.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerListRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerListRes, viewerList)
	if viewerListRes.Code != http.StatusOK {
		t.Fatalf("VIEWER catalog status=%d want=200", viewerListRes.Code)
	}

	viewerMaterialize := httptest.NewRequest(http.MethodPost, "/api/v1/device-profiles/stagecore.generic.osc-udp/materialize", bytes.NewBufferString(`{"values":{"host":"projector.local"}}`))
	viewerMaterialize.RemoteAddr = "127.0.0.1:19003"
	viewerMaterialize.Header.Set("Content-Type", "application/json")
	viewerMaterialize.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerMaterialize.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerMaterializeRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerMaterializeRes, viewerMaterialize)
	if viewerMaterializeRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER materialize status=%d want=403", viewerMaterializeRes.Code)
	}
}
