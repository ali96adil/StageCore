package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectRemoteOperatorHTTP(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := redirectRemoteOperatorHTTP(next, "stagecore-abcdef123456.local", "0.0.0.0:7842")

	remote := httptest.NewRequest(http.MethodGet, "http://192.168.3.135:7840/operator?x=1", nil)
	remote.RemoteAddr = "192.168.3.20:50000"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, remote)
	if recorder.Code != http.StatusPermanentRedirect {
		t.Fatalf("remote status=%d", recorder.Code)
	}
	if got := recorder.Header().Get("Location"); got != "https://stagecore-abcdef123456.local:7842/operator?x=1" {
		t.Fatalf("redirect location=%q", got)
	}

	health := httptest.NewRequest(http.MethodGet, "http://192.168.3.135:7840/health/ready", nil)
	health.RemoteAddr = "192.168.3.20:50000"
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, health)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("remote health status=%d", recorder.Code)
	}

	local := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7840/operator", nil)
	local.RemoteAddr = "127.0.0.1:50000"
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, local)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("loopback status=%d", recorder.Code)
	}
}
