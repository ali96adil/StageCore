package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestProjectCameraViewerRevokesIdleStream(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "camera project"})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "other project"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	user, err := h.auth.CreateUser(ctx, "camera viewer", h.password, userauth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	login := func() string {
		t.Helper()
		credentials, err := h.auth.Login(ctx, user.Username, h.password, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		return credentials.Token
	}
	grant := func(project string, permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, user.ID, project, permissions); err != nil {
			t.Fatal(err)
		}
	}

	token := strings.Repeat("v", 32)
	tokenFile := filepath.Join(t.TempDir(), "viewer.token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var upstreamCalls atomic.Int32
	disconnected := make(chan struct{}, 2)
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.URL.String() != "/api/v0/stream" || r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("Cookie") != "" {
			t.Error("unexpected relay request scope")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { disconnected <- struct{}{} }()
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=relay")
		fmt.Fprint(w, "--relay\r\nContent-Type: image/jpeg\r\nContent-Length: 4\r\n\r\n\xff\xd8\xff\xd9\r\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer relay.Close()
	if _, err := NewProjectCameraViewer(first.ID, "192.168.1.2:9081", tokenFile); err == nil {
		t.Fatal("accepted a non-loopback relay")
	}
	viewer, err := NewProjectCameraViewer(first.ID, strings.TrimPrefix(relay.URL, "http://"), tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	hub := httptest.NewServer(New(WithOperatorCameraViewer(h.auth, access, viewer)).Handler())
	defer hub.Close()
	client := &http.Client{Timeout: 6 * time.Second}
	do := func(method, project, browserToken, suffix string, expected int) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, hub.URL+"/api/v1/projects/"+project+"/camera-relay/stream"+suffix, nil)
		if err != nil {
			t.Fatal(err)
		}
		if browserToken != "" {
			req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: browserToken})
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		if resp.StatusCode != expected {
			t.Fatalf("%s got %d, want %d", method, resp.StatusCode, expected)
		}
		return resp
	}

	browser := login()
	do(http.MethodGet, first.ID, "", "", 401)
	grant(first.ID, projectaccess.ProjectView)
	do(http.MethodGet, first.ID, browser, "", 404)
	grant(second.ID, projectaccess.ProjectView, projectaccess.MediaRead)
	do(http.MethodGet, second.ID, browser, "", 404)
	grant(first.ID, projectaccess.ProjectView, projectaccess.MediaRead)
	do(http.MethodHead, first.ID, browser, "", 204)
	if upstreamCalls.Load() != 0 {
		t.Fatal("unauthorized request or HEAD contacted the relay")
	}

	waitForClose := func(resp *http.Response) {
		t.Helper()
		if _, err := io.ReadAll(resp.Body); err != nil {
			t.Fatalf("stream did not close promptly: %v", err)
		}
		select {
		case <-disconnected:
		case <-time.After(2 * time.Second):
			t.Fatal("revoked stream left a relay viewer open")
		}
	}
	firstStream := do(http.MethodGet, first.ID, browser, "?flash=on", 200)
	grant(first.ID, projectaccess.ProjectView)
	waitForClose(firstStream)

	grant(first.ID, projectaccess.ProjectView, projectaccess.MediaRead)
	secondStream := do(http.MethodGet, first.ID, browser, "", 200)
	otherBrowser := login()
	if err := h.auth.Logout(ctx, browser); err != nil {
		t.Fatal(err)
	}
	waitForClose(secondStream)
	do(http.MethodHead, first.ID, otherBrowser, "", 204)
}

func TestProjectCameraViewerRechecksBeforeFirstFrame(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "viewer.token")
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("v", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	var allowed atomic.Bool
	allowed.Store(true)
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		allowed.Store(false)
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=relay")
		fmt.Fprint(w, "--relay\r\nContent-Type: image/jpeg\r\nContent-Length: 4\r\n\r\n\xff\xd8\xff\xd9\r\n--relay--\r\n")
	}))
	defer relay.Close()
	viewer, err := NewProjectCameraViewer("project", strings.TrimPrefix(relay.URL, "http://"), tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viewer.stream(w, r, func(context.Context) error {
			if !allowed.Load() {
				return projectaccess.ErrDenied
			}
			return nil
		})
	}))
	defer hub.Close()
	client := hub.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Get(hub.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusNotFound ||
		strings.Contains(string(body), "\xff\xd8") ||
		strings.HasPrefix(resp.Header.Get("Content-Type"), "multipart/") {
		t.Fatalf("revoked viewer got status %d, content type %q, %d bytes, err=%v",
			resp.StatusCode, resp.Header.Get("Content-Type"), len(body), err)
	}
}
