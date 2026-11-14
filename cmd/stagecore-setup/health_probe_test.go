package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPerformHealthProbeReturnsOnlyHealthFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health/ready" {
			_, _ = w.Write([]byte(`{"status":"READY","storage_state":"HEALTHY","private":"omit"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"LIVE","private":"omit"}`))
	}))
	defer server.Close()

	for _, probe := range []struct {
		path, wantStatus, wantStorage string
	}{
		{"/health/live", "LIVE", ""},
		{"/health/ready", "READY", "HEALTHY"},
	} {
		result, err := performHealthProbe(context.Background(), server.URL+probe.path, time.Second, server.Client())
		if err != nil {
			t.Fatalf("probe %s: %v", probe.path, err)
		}
		if result.HTTPStatus != http.StatusOK || result.Status != probe.wantStatus || result.StorageState != probe.wantStorage {
			t.Fatalf("probe %s result=%+v", probe.path, result)
		}
	}
}

func TestPerformHealthProbeRejectsRedirectAndWrongHealthState(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/health/ready", http.StatusFound)
	}))
	defer redirect.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if _, err := performHealthProbe(context.Background(), redirect.URL+"/health/live", time.Second, client); err == nil {
		t.Fatal("redirect to another health endpoint was accepted")
	}

	wrongState := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"LIVE"}`))
	}))
	defer wrongState.Close()
	if _, err := performHealthProbe(context.Background(), wrongState.URL+"/health/ready", time.Second, wrongState.Client()); err == nil {
		t.Fatal("wrong health state was accepted")
	}
}

func TestPerformHealthProbeRejectsNonLoopbackEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"http://example.com/health/live",
		"http://192.0.2.1:17840/health/live",
		"http://127.0.0.1:17840/admin",
	} {
		if _, err := performHealthProbe(context.Background(), endpoint, time.Second, nil); err == nil {
			t.Fatalf("unsafe endpoint %q was accepted", endpoint)
		}
	}
}
