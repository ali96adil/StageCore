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
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
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
	if _, err := st.CreateSession(ctx, published.ID, domain.SessionShow, "Browser SHOW workspace"); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(st, registry)
	handler := New(WithOperatorWeb(), WithOperatorProjects(h.auth, st), WithOperatorRuntime(h.auth, st, runtime)).Handler()
	testScript, err := os.ReadFile(filepath.Join("testdata", "show_workspace_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<!doctype html><html><head><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/workspace-profile.css"></head><body>
<div id="loginView"></div><div id="loginMessage"></div><header class="topbar"><span id="hubBadge"></span><div id="userArea"><span id="userLabel"></span><button id="logoutButton" type="button"></button></div></header>
<div id="appView" class="app-layout"><aside id="sidebar" class="sidebar"><button id="projectsNav" class="nav-button" type="button">Projects</button><div id="workspaceNav" class="workspace-nav"><p id="workspaceProjectName" class="workspace-name"></p>
<button class="nav-button" data-page="dashboard" type="button">Dashboard</button><button class="nav-button" data-page="configuration" type="button">Configuration</button><button class="nav-button" data-page="cues" type="button">Cues</button><button class="nav-button" data-page="runtime" type="button">Runtime</button><button class="nav-button" data-page="preflight" type="button">Preflight</button><button class="nav-button" data-page="sessions" type="button">Sessions</button><button class="nav-button" data-page="notes" type="button">Notes</button></div></aside>
<main class="workspace"><div id="globalMessage" class="message hidden"></div><section id="content"></section></main></div>
<form id="loginForm"><input id="username"><input id="password" type="password"></form><dialog id="cueDialog"><form id="cueForm"><div id="actionsEditor"></div></form></dialog>
<script src="/show-workspace-app.js"></script><script src="/workspace-profile.js"></script><script src="/show-workspace-test.js"></script></body></html>`)
		case "/show-workspace-app.js":
			rec := httptest.NewRecorder()
			appReq := r.Clone(r.Context())
			appReq.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appReq)
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/show-workspace-config":
			writeJSON(w, http.StatusOK, map[string]string{"csrf": owner.CSRFToken, "project": project.ID})
		case "/show-workspace-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/show-workspace-test.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Write(testScript)
		default:
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
		"--disable-background-networking", "--user-data-dir="+profileDir, server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "show-browser.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer browserLog.Close()
	cmd.Stdout, cmd.Stderr = browserLog, browserLog
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	defer func() {
		cancel()
		if !exited {
			<-done
		}
	}()
	select {
	case outcome := <-browserResult:
		if outcome != "PASS" {
			t.Fatal(outcome)
		}
	case err := <-done:
		exited = true
		output, _ := os.ReadFile(browserLog.Name())
		t.Fatalf("browser exited before reporting: %v\n%s", err, output)
	case <-ctx.Done():
		output, _ := os.ReadFile(browserLog.Name())
		t.Fatalf("browser did not finish: %s", output)
	}
}
