package camerarelay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProtectedRelayScopesAndRevokesActiveStream(t *testing.T) {
	dir := t.TempDir()
	viewPath, controlPath := filepath.Join(dir, "view"), filepath.Join(dir, "control")
	writeToken := func(path, token string) {
		t.Helper()
		next := path + ".new"
		if err := os.WriteFile(next, []byte(token+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(next, path); err != nil {
			t.Fatal(err)
		}
	}
	viewer, controller := strings.Repeat("v", 48), strings.Repeat("c", 48)
	writeToken(viewPath, viewer)
	writeToken(controlPath, controller)
	authorize, err := FileAccess(viewPath, controlPath)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := New(Config{
		SourceURL: "http://127.0.0.1/stream", FlashControlURL: "http://127.0.0.1/api/v0/flash",
		Authorize: authorize,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := relay.Handler()
	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{"GET", "/api/v0/health", "", http.StatusUnauthorized},
		{"GET", "/api/v0/stream?token=" + viewer, "", http.StatusUnauthorized},
		{"GET", "/api/v0/stream?flash=on", viewer, http.StatusUnauthorized},
		{"POST", "/api/v0/flash?state=off", viewer, http.StatusUnauthorized},
		{"POST", "/api/v0/flash?state=on", controller, http.StatusConflict},
		{"GET", "/api/v0/health", viewer, http.StatusServiceUnavailable},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.status || res.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("%s %s status=%d want=%d", tc.method, tc.path, res.Code, tc.status)
		}
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	req, err := http.NewRequest("GET", server.URL+"/api/v0/stream?flash=on", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+controller)
	client := server.Client()
	client.Timeout = 4 * time.Second
	connected := make(chan struct{})
	publishDone := make(chan struct{})
	go func() {
		defer close(publishDone)
		for i := 0; i < 200; i++ {
			relay.mu.Lock()
			active := relay.flashRequestingViewersLocked() == 1 && relay.flashDesired
			relay.mu.Unlock()
			if active {
				close(connected)
				relay.publish(testJPEG(1))
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	resp, err := client.Do(req)
	<-publishDone
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status=%d", resp.StatusCode)
	}
	select {
	case <-connected:
	default:
		t.Fatal("authorized viewer did not request flash")
	}
	writeToken(controlPath, strings.Repeat("n", 48))
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("stream did not close after rotation: %v", err)
	}
	relay.mu.Lock()
	defer relay.mu.Unlock()
	if len(relay.subscribers) != 0 || relay.flashDesired {
		t.Fatal("revoked stream kept its viewer slot or requested flash")
	}
}
