package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/timingintelligence"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestBrowserShowWorkspacePinsRuntime(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	type browserLogin struct {
		token, csrf, userID, username string
	}
	roleLogins := make(map[string]browserLogin)
	roleLogins[userauth.RoleOwner] = browserLogin{
		token: owner.Token, csrf: owner.CSRFToken, userID: owner.Session.User.ID, username: owner.Session.User.Username,
	}
	for _, role := range []string{userauth.RoleTechnician, userauth.RoleOperator, userauth.RoleViewer} {
		username := "workspace-" + strings.ToLower(role)
		if _, err := h.auth.CreateUser(ctx, username, h.password, role); err != nil {
			t.Fatalf("create isolated %s browser user: %v", role, err)
		}
		login, err := h.auth.Login(ctx, username, h.password, "127.0.0.1")
		if err != nil {
			t.Fatalf("login isolated %s browser user: %v", role, err)
		}
		roleLogins[role] = browserLogin{
			token: login.Token, csrf: login.CSRFToken, userID: login.Session.User.ID, username: username,
		}
	}
	st := store.New(h.db.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Browser SHOW workspace", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"Opening Cue", "Final Cue"} {
		if _, err := st.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: fmt.Sprint(index + 1), Name: name, OrderIndex: index + 1,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		}, []domain.Action{{
			ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
			Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Seed an active SHOW row directly in the temporary test database so this
	// UI-routing test does not depend on operational admission or touch outputs.
	showSession, err := st.CreateSession(ctx, published.ID, domain.SessionShow, "Browser SHOW workspace")
	if err != nil {
		t.Fatal(err)
	}
	var stopSafetyMu sync.Mutex
	var stopSafetySessions []string
	runtime := runtimecontrol.New(st, registry, runtimecontrol.WithSessionStopSafety(func(_ context.Context, got domain.Session, _ contracts.CommandEnvelope) error {
		if (got.Type != domain.SessionShow && got.Type != domain.SessionRehearsal) || got.ProjectID != project.ID {
			return fmt.Errorf("simulated SHOW stop safety received wrong Project/Session: %+v", got)
		}
		if got.Type == domain.SessionShow {
			stopSafetyMu.Lock()
			stopSafetySessions = append(stopSafetySessions, got.ID)
			stopSafetyMu.Unlock()
		}
		return nil
	}))
	handler := New(WithOperatorWeb(), WithOperatorProjects(h.auth, st), WithOperatorRuntime(h.auth, st, runtime),
		WithOperatorCuePublish(h.auth, st, publish.New(st, registry)),
		WithOperatorTimingIntelligence(h.auth, timingintelligence.New(st, clock.Real{}))).Handler()
	testScript, err := os.ReadFile(filepath.Join("testdata", "show_workspace_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	type viewportRequest struct {
		width, height int
		resume        chan struct{}
	}
	viewportRequests := make(chan viewportRequest, 1)
	keyRequests := make(chan chan struct{}, 1)
	touchRequests := make(chan struct {
		x, y   float64
		resume chan struct{}
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/workspace-profile.css"></head><body>
<div id="loginView"></div><div id="loginMessage"></div><header class="topbar"><span id="hubBadge"></span><div id="userArea"><span id="userLabel"></span><button id="logoutButton" type="button"></button></div></header>
<div id="appView" class="app-layout"><aside id="sidebar" class="sidebar"><button id="projectsNav" class="nav-button" type="button">Projects</button><div id="workspaceNav" class="workspace-nav"><p id="workspaceProjectName" class="workspace-name"></p>
<button class="nav-button" data-page="dashboard" type="button">Dashboard</button><button class="nav-button" data-page="configuration" type="button">Configuration</button><button class="nav-button" data-page="cues" type="button">Cues</button><button class="nav-button" data-page="runtime" type="button">Runtime</button><button class="nav-button" data-page="preflight" type="button">Preflight</button><button class="nav-button" data-page="sessions" type="button">Sessions</button><button class="nav-button" data-page="notes" type="button">Notes</button></div></aside>
<main class="workspace"><div id="globalMessage" class="message hidden"></div><section id="content"></section></main></div>
<form id="loginForm"><input id="username"><input id="password" type="password"></form><dialog id="cueDialog"><form id="cueForm"><div id="actionsEditor"></div></form></dialog>
<script src="/show-workspace-app.js"></script><script src="/timing-intelligence.js"></script><script src="/workspace-profile.js"></script><script src="/show-workspace-test.js"></script></body></html>`)
		case "/show-workspace-app.js":
			rec := httptest.NewRecorder()
			appReq := r.Clone(r.Context())
			appReq.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appReq)
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/show-workspace-config":
			writeJSON(w, http.StatusOK, map[string]string{"csrf": owner.CSRFToken, "project": project.ID})
		case "/show-workspace-switch-role":
			if r.Method != http.MethodPost {
				http.Error(w, "POST required", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Role string `json:"role"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil {
				http.Error(w, "invalid role request", http.StatusBadRequest)
				return
			}
			login, ok := roleLogins[request.Role]
			if !ok {
				http.Error(w, "unknown test role", http.StatusBadRequest)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: login.token, HttpOnly: true, Path: "/"})
			writeJSON(w, http.StatusOK, map[string]any{
				"csrf": login.csrf,
				"user": map[string]string{"user_id": login.userID, "username": login.username, "role": request.Role},
			})
		case "/show-workspace-seed-show":
			if r.Method != http.MethodPost {
				http.Error(w, "POST required", http.StatusMethodNotAllowed)
				return
			}
			seeded, err := st.CreateSession(r.Context(), published.ID, domain.SessionShow, "Browser role SHOW")
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, map[string]string{"session_id": seeded.ID})
		case "/show-workspace-seed-preview":
			if r.Method != http.MethodPost {
				http.Error(w, "POST required", http.StatusMethodNotAllowed)
				return
			}
			seeded, err := st.CreateSessionWithPurpose(r.Context(), published.ID, domain.SessionRehearsal,
				"Browser Preview approval", domain.RehearsalPreview)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, map[string]string{"session_id": seeded.ID})
		case "/show-workspace-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/show-workspace-viewport-ready":
			resume := make(chan struct{})
			select {
			case viewportRequests <- viewportRequest{width: 390, height: 844, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/show-workspace-resize-ready":
			var request struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil ||
				(request.Width != 390 && request.Width != 768 && request.Width != 1280) || request.Height <= 0 || request.Height > 1200 {
				http.Error(w, "invalid responsive test viewport", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case viewportRequests <- viewportRequest{width: request.Width, height: request.Height, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/show-workspace-key-ready":
			resume := make(chan struct{})
			select {
			case keyRequests <- resume:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/show-workspace-touch-ready":
			var request struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request); err != nil ||
				request.X < 0 || request.X > 390 || request.Y < 0 || request.Y > 844 {
				http.Error(w, "invalid touch coordinates", http.StatusBadRequest)
				return
			}
			resume := make(chan struct{})
			select {
			case touchRequests <- struct {
				x, y   float64
				resume chan struct{}
			}{x: request.X, y: request.Y, resume: resume}:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/show-workspace-test.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Write(testScript)
		default:
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/preflight") {
				writeJSON(w, http.StatusOK, map[string]any{
					"status": "PASS", "project_id": project.ID, "runtime_snapshot_id": published.ID,
					"snapshot_version": published.SnapshotVersion, "evaluated_at": time.Now().UTC(),
					"checks": []any{}, "roles": []any{}, "media": []any{},
					"storage": map[string]any{"status": "PASS", "state": "HEALTHY", "writable": true},
				})
				return
			}
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	profileDir, err := os.MkdirTemp("", "stagecore-show-chrome-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		var removeErr error
		for attempt := 0; attempt < 20; attempt++ {
			removeErr = os.RemoveAll(profileDir)
			if removeErr == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Errorf("Chrome profile cleanup failed: %v", removeErr)
	}()
	cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-dev-shm-usage",
		"--remote-debugging-port=0", "--remote-allow-origins=*", "--disable-background-networking",
		"--window-size=390,844", "--user-data-dir="+profileDir, server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "show-browser.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer browserLog.Close()
	cmd.Stdout, cmd.Stderr = browserLog, browserLog
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cdp, err := connectChromePage(ctx, profileDir, server.URL)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("connect to isolated Chrome DevTools: %v", err)
	}
	defer cdp.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	defer func() {
		cancel()
		if !exited {
			<-done
		}
	}()
	keyCommandID := 100
	viewportCommandID := 90
	outcome := ""
	for outcome == "" {
		select {
		case request := <-viewportRequests:
			if err := chromeDevToolsCommand(cdp, viewportCommandID, "Emulation.setDeviceMetricsOverride", map[string]any{
				"width": request.width, "height": request.height, "deviceScaleFactor": 1, "mobile": true,
			}); err != nil {
				t.Fatalf("set isolated Chrome mobile viewport: %v", err)
			}
			viewportCommandID++
			close(request.resume)
		case outcome = <-browserResult:
		case resume := <-keyRequests:
			if err := dispatchChromeKey(cdp, keyCommandID, "Enter", false); err != nil {
				t.Fatalf("send native Enter key to workspace navigation: %v", err)
			}
			keyCommandID += 2
			close(resume)
		case request := <-touchRequests:
			if err := dispatchChromeTouch(cdp, request.x, request.y); err != nil {
				t.Fatalf("send emulated touchscreen event to workspace navigation: %v", err)
			}
			close(request.resume)
		case err := <-done:
			exited = true
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser exited before reporting: %v\n%s", err, output)
		case <-ctx.Done():
			output, _ := os.ReadFile(browserLog.Name())
			t.Fatalf("browser did not finish: %s", output)
		}
	}
	var browserOutcome struct {
		Status         string   `json:"status"`
		SeededSessions []string `json:"seeded_sessions"`
		Failure        string   `json:"failure,omitempty"`
	}
	if err := json.Unmarshal([]byte(outcome), &browserOutcome); err != nil {
		t.Fatalf("decode browser role result %q: %v", outcome, err)
	}
	if browserOutcome.Status != "PASS" {
		t.Fatalf("browser role acceptance failed: %s", browserOutcome.Failure)
	}
	expectedSessions := append([]string{showSession.ID}, browserOutcome.SeededSessions...)
	stopSafetyMu.Lock()
	safetySessions := append([]string(nil), stopSafetySessions...)
	stopSafetyMu.Unlock()
	if len(safetySessions) != len(expectedSessions) {
		t.Fatalf("normal SHOW stop safety seam calls=%v, want one for each seeded Session %v", safetySessions, expectedSessions)
	}
	for _, expectedID := range expectedSessions {
		found := false
		for _, actualID := range safetySessions {
			if actualID == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("normal SHOW stop safety did not run for Session %s: calls=%v", expectedID, safetySessions)
		}
		stopped, err := st.GetSession(ctx, expectedID)
		if err != nil || stopped.Status != domain.SessionCompleted {
			t.Fatalf("SHOW stop endpoint did not complete isolated Session %s: session=%+v err=%v", expectedID, stopped, err)
		}
	}
	active, err := st.ActiveSessionForProject(ctx, project.ID)
	if err != nil || active != nil {
		t.Fatalf("SHOW stop endpoint left an active Project Session: active=%+v err=%v", active, err)
	}
}
