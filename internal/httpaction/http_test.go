package httpaction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestHTTPActionTruthfulResultAndNoRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/cue" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request=%s %s headers=%v", r.Method, r.URL.Path, r.Header)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	target, _ := json.Marshal(map[string]any{"url": server.URL})
	params, _ := json.Marshal(map[string]any{"method": "POST", "path": "/cue", "headers": map[string]string{"Content-Type": "application/json"}, "body": `{"go":true}`})
	result := New().Execute(context.Background(), capability.Request{
		ExecutionID: "exec-http-1", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "HTTP-DEVICE", LogicalType: "http", Configuration: target},
		Parameters: params, TimeoutMS: 1000,
	})
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckAccepted || result.ErrorCode != "" || calls.Load() != 1 {
		t.Fatalf("result=%+v calls=%d", result, calls.Load())
	}
}

func TestHTTPActionTimeoutDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-r.Context().Done()
	}))
	defer server.Close()
	target, _ := json.Marshal(map[string]any{"url": server.URL})
	result := New().Execute(context.Background(), capability.Request{
		ExecutionID: "exec-http-timeout", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "HTTP-SLOW", LogicalType: "http", Configuration: target},
		Parameters: json.RawMessage(`{"method":"GET"}`), TimeoutMS: 30,
	})
	if result.Result != domain.ExecutionTimedOut || result.ErrorCode != "HTTP_TIMEOUT" || calls.Load() != 1 {
		t.Fatalf("result=%+v calls=%d", result, calls.Load())
	}
}

func TestHTTPActionBlocksCredentialBearingConfiguration(t *testing.T) {
	executor := NewWithClient(&http.Client{Timeout: time.Second})
	target, _ := json.Marshal(map[string]any{"url": "http://127.0.0.1/device"})
	params := json.RawMessage(`{"method":"POST","headers":{"Authorization":"Bearer raw-secret"}}`)
	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "exec-http-secret", Capability: CapabilityKey,
		Target: &capability.Target{Ref: "HTTP-DEVICE", LogicalType: "http", Configuration: target}, Parameters: params,
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SECRET_STORE_REQUIRED" || result.AckLevel != contracts.AckNone {
		t.Fatalf("secret-bearing result=%+v", result)
	}
}
