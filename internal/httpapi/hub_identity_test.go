package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
)

func TestHubIdentityEndpointExposesOnlyPublicDiscoveryMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	security, err := hubsecurity.Open(ctx, handle.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := security.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}

	server := New(WithHubIdentity(security))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/hub/identity", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["hub_id"] != identity.HubID || payload["fingerprint"] != identity.Fingerprint || payload["display_name"] != identity.DisplayName {
		t.Fatalf("unexpected identity payload: %#v", payload)
	}
	for _, forbidden := range []string{"private_key", "password", "session_token", "setup_code", "pairing_code"} {
		if _, exists := payload[forbidden]; exists {
			t.Fatalf("public identity response leaked %q", forbidden)
		}
	}
}
