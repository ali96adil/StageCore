package httpaction

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type testSecretResolver struct {
	value string
	err   error
	calls atomic.Int32
	ref   string
}

func (r *testSecretResolver) Resolve(_ context.Context, reference string) (string, error) {
	r.calls.Add(1)
	r.ref = reference
	if r.err != nil {
		return "", r.err
	}
	return r.value, nil
}

func TestHTTPActionUsesOnlyDeclaredSecretReferenceAtExecutionBoundary(t *testing.T) {
	const secretValue = "not-in-project-configuration"
	resolver := &testSecretResolver{value: secretValue}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer "+secretValue {
			t.Fatalf("authorization=%q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	target, _ := json.Marshal(map[string]any{
		"url": server.URL,
		"secret_ref": "secret:device-api",
		"secret_header": "Authorization",
		"secret_prefix": "Bearer ",
	})
	if strings.Contains(string(target), secretValue) {
		t.Fatal("target configuration contains plaintext secret")
	}
	result := NewWithSecretResolver(resolver).Execute(context.Background(), capability.Request{
		ExecutionID: "http-secret", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "HTTP-DEVICE", LogicalType: "http", Configuration: target},
		Parameters: json.RawMessage(`{"method":"POST"}`), TimeoutMS: 1000,
	})
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckAccepted || result.ErrorCode != "" {
		t.Fatalf("result=%+v", result)
	}
	if resolver.calls.Load() != 1 || resolver.ref != "secret:device-api" || calls.Load() != 1 {
		t.Fatalf("resolver calls=%d ref=%q HTTP calls=%d", resolver.calls.Load(), resolver.ref, calls.Load())
	}
}

func TestHTTPActionDoesNotSendWhenSecretResolutionFails(t *testing.T) {
	resolver := &testSecretResolver{err: errors.New("missing")}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	target, _ := json.Marshal(map[string]any{
		"url": server.URL,
		"secret_ref": "secret:missing",
		"secret_header": "X-Api-Key",
	})
	result := NewWithSecretResolver(resolver).Execute(context.Background(), capability.Request{
		ExecutionID: "http-secret-missing", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "HTTP-DEVICE", LogicalType: "http", Configuration: target},
		Parameters: json.RawMessage(`{"method":"GET"}`), TimeoutMS: 1000,
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "HTTP_SECRET_UNAVAILABLE" || result.AckLevel != contracts.AckNone {
		t.Fatalf("result=%+v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP request sent despite unresolved secret: %d", calls.Load())
	}
}
