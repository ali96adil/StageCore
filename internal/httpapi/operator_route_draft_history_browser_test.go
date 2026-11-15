package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/operatorweb"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBrowserRouteDraftHistoryKeyboardUndoAndTouchRedo(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(h.db.DB, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Route Draft History Browser", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	input, err := st.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	route, err := st.CreateRouteWithActions(store.WithRouteDraftHistoryActor(ctx, owner.Session.User.ID), domain.Route{
		RevisionID: revision.ID, Name: "Undo Route", InputID: input.ID,
		PriorityClass: domain.PriorityP2, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithOperatorWeb(),
		WithOperatorProjects(h.auth, st),
		WithOperatorConfiguration(h.auth, st),
	).Handler()
	index, err := operatorweb.Read("index.html")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := strings.Replace(string(index), `<script src="/app.js" defer></script>`, `<script src="/route-history-app.js" defer></script>`, 1)
	testScript, err := os.ReadFile(filepath.Join("testdata", "route_draft_history_browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserResult := make(chan string, 1)
	keyRequests := make(chan chan struct{}, 2)
	redoKeyRequests := make(chan chan struct{}, 1)
	touchRequests := make(chan struct {
		x, y   float64
		resume chan struct{}
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects/" + project.ID + "/configuration/lock":
			// The browser UI always checks the SHOW lock before rendering Draft
			// controls. This isolated fixture has no Runtime handlers, so model
			// the safe unlocked state without starting any output-capable runtime.
			writeJSON(w, http.StatusOK, map[string]any{"show_configuration_lock": map[string]any{"locked": false}})
		case "/":
			page := strings.Replace(indexHTML, `</body>`, `<script src="/route-draft-history-test.js" defer></script></body>`, 1)
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, page)
		case "/route-history-app.js":
			rec := httptest.NewRecorder()
			appRequest := r.Clone(r.Context())
			appRequest.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appRequest)
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/route-draft-history-config":
			writeJSON(w, http.StatusOK, map[string]string{
				"csrf": owner.CSRFToken, "project": project.ID, "project_name": project.Name, "route": route.ID,
			})
		case "/route-draft-history-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/route-draft-history-key-ready", "/route-draft-history-text-key-ready":
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
		case "/route-draft-history-redo-key-ready":
			resume := make(chan struct{})
			select {
			case redoKeyRequests <- resume:
			case <-r.Context().Done():
				return
			}
			select {
			case <-resume:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
		case "/route-draft-history-touch-ready":
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
		case "/route-draft-history-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(testScript)
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	profileDir, err := os.MkdirTemp("", "stagecore-route-history-chrome-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for attempt := 0; attempt < 20; attempt++ {
			if err := os.RemoveAll(profileDir); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Errorf("Chrome profile cleanup failed for %s", profileDir)
	}()
	cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-dev-shm-usage",
		"--remote-debugging-port=0", "--remote-allow-origins=*", "--disable-background-networking",
		"--window-size=390,844", "--user-data-dir="+profileDir, server.URL)
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "route-draft-history-browser.log"))
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
	var outcome string
	keyCommandID := 100
	for outcome == "" {
		select {
		case outcome = <-browserResult:
			if !strings.HasPrefix(outcome, `{"status":"PASS"`) {
				t.Fatal(outcome)
			}
		case resume := <-keyRequests:
			if err := dispatchChromeShortcut(cdp, keyCommandID, "z", 4); err != nil {
				t.Fatalf("send native Meta+Z shortcut to Route Undo: %v", err)
			}
			keyCommandID += 2
			close(resume)
		case resume := <-redoKeyRequests:
			if err := dispatchChromeShortcut(cdp, keyCommandID, "z", 12); err != nil {
				t.Fatalf("send native Meta+Shift+Z shortcut to Route Redo: %v", err)
			}
			keyCommandID += 2
			close(resume)
		case request := <-touchRequests:
			if err := dispatchChromeTouch(cdp, request.x, request.y); err != nil {
				t.Fatalf("send emulated touchscreen event to Route Redo: %v", err)
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
	var report struct {
		Status               string           `json:"status"`
		UndoEnabled          bool             `json:"undo_enabled"`
		KeyboardUndo         bool             `json:"keyboard_undo"`
		KeyboardRedo         bool             `json:"keyboard_redo"`
		TouchRedo            bool             `json:"touch_redo"`
		RestoredSameRoute    bool             `json:"restored_same_route"`
		Overflow             bool             `json:"overflow"`
		RouteHistoryRequests []string         `json:"route_history_requests"`
		UnexpectedMutations  []map[string]any `json:"unexpected_mutations"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil {
		t.Fatalf("decode browser result %q: %v", outcome, err)
	}
	if report.Status != "PASS" || !report.UndoEnabled || !report.KeyboardUndo || !report.KeyboardRedo || !report.TouchRedo ||
		!report.RestoredSameRoute || report.Overflow || len(report.UnexpectedMutations) != 0 {
		t.Fatalf("Route history browser result=%+v raw=%s", report, outcome)
	}
	routes, err := st.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 1 || routes[0].ID != route.ID {
		t.Fatalf("browser Redo did not restore the original Route: routes=%+v err=%v", routes, err)
	}
	snapshot, err := st.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil || snapshot != nil {
		t.Fatalf("Route history browser created a Published Snapshot: snapshot=%+v err=%v", snapshot, err)
	}
}
