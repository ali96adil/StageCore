package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/extension"
)

type staticExtensionRuntimeProbe struct {
	result extension.RuntimeProbeResult
}

func (p staticExtensionRuntimeProbe) Probe(context.Context, string) (extension.RuntimeProbeResult, error) {
	return p.result, nil
}

func TestExtensionRuntimeProbeSuppressesResultAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withOperatorExtensionRuntimeProbe(h.auth, staticExtensionRuntimeProbe{
		result: extension.RuntimeProbeResult{
			InstallationID: "installation-private", ExtensionID: "private.runtime", Version: "1.2.3",
			Status: "READY", ProcessStarted: true, ProcessStopped: true,
		},
	}, nil, extensionRuntimeProbeHooks{BeforeResponse: func() {
		revoked = true
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			t.Errorf("revoke Owner before runtime probe response: %v", err)
		}
	}})).Handler()
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/extensions/installations/installation-private/runtime-probe", nil, owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatal("test did not revoke Owner immediately before runtime probe response")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || body["error_code"] != capability.DispatchAuthorityRevokedCode ||
		body["probe_may_have_run"] != true || body["effect_may_have_been_committed"] != true || body["retryable"] != false {
		t.Fatalf("revoked Owner received runtime probe result or retryable response: HTTP %d %+v", response.Code, body)
	}
	if _, leaked := body["installation_id"]; leaked {
		t.Fatalf("revoked Owner received runtime probe details: %+v", body)
	}
}
