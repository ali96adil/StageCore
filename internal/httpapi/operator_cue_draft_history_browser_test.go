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

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/operatorweb"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBrowserCueDraftHistoryKeyboardUndoAndTouchRedo(t *testing.T) {
	chrome := os.Getenv("STAGECORE_TEST_CHROME")
	if chrome == "" {
		t.Skip("set STAGECORE_TEST_CHROME to enable the real-browser gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	h := newAuthHarness(t)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	projects := store.New(h.db.DB, clock.Real{})
	project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{
		Name: "Cue Draft History Browser", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := projects.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Baseline Cue", OrderIndex: 0,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := projects.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2A", Name: "Linked child", OrderIndex: 1,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM-LIGHT", CapabilityKey: "lighting.set",
		Parameters: json.RawMessage(`{"channel":1,"value":60}`), TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy: json.RawMessage(`{}`), PriorityClass: domain.PriorityP2, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	linkedID := linked.ID
	created, err := projects.CreateCueWithActions(store.WithCueDraftHistoryActor(ctx, owner.Session.User.ID), domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Undo Mixed Linked Cue", OrderIndex: 2,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{"linked_cue_ids":["` + linked.ID + `"],"linked_cue_mode":"TOGETHER"}`),
	}, []domain.Action{
		{OrderIndex: 0, ExecutionMode: "PARALLEL", TargetRef: "SIM-VIDEO", CapabilityKey: "osc.send",
			Parameters: json.RawMessage(`{"address":"/cue/fire"}`), TimeoutPolicy: json.RawMessage(`{}`),
			ErrorPolicy: json.RawMessage(`{}`), PriorityClass: domain.PriorityP1, Enabled: true},
		{OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM-AUDIO", CapabilityKey: "midi.send",
			Parameters: json.RawMessage(`{"note":60}`), TimeoutPolicy: json.RawMessage(`{"start_delay_ms":250}`),
			ErrorPolicy: json.RawMessage(`{}`), PriorityClass: domain.PriorityP2, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	handler := New(
		WithOperatorWeb(),
		WithOperatorProjects(h.auth, projects),
		WithOperatorConfigurationDraft(h.auth, projects),
		WithOperatorCuePublish(h.auth, projects, publish.New(projects, registry)),
		WithProjectRouteQualification(access),
	).Handler()
	index, err := operatorweb.Read("index.html")
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := strings.Replace(string(index), `<script src="/app.js" defer></script>`, `<script src="/draft-history-app.js" defer></script>`, 1)
	testScript, err := os.ReadFile(filepath.Join("testdata", "cue_draft_history_browser.js"))
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
		case "/":
			page := strings.Replace(indexHTML, `</body>`, `<script src="/cue-draft-history-test.js" defer></script></body>`, 1)
			http.SetCookie(w, &http.Cookie{Name: browserSessionCookie, Value: owner.Token, HttpOnly: true, Path: "/"})
			setOperatorWebSecurityHeaders(w)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, page)
		case "/draft-history-app.js":
			rec := httptest.NewRecorder()
			appRequest := r.Clone(r.Context())
			appRequest.URL.Path = "/app.js"
			handler.ServeHTTP(rec, appRequest)
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = io.WriteString(w, strings.TrimSuffix(strings.TrimSpace(rec.Body.String()), "boot();"))
		case "/cue-draft-history-config":
			writeJSON(w, http.StatusOK, map[string]string{
				"csrf": owner.CSRFToken, "project": project.ID, "project_name": project.Name,
				"baseline_cue": baseline.ID, "linked_cue": linkedID, "created_cue": created.ID,
			})
		case "/cue-draft-history-result":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
			browserResult <- string(body)
			w.WriteHeader(http.StatusNoContent)
		case "/cue-draft-history-key-ready", "/cue-draft-history-input-key-ready":
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
		case "/cue-draft-history-redo-key-ready":
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
		case "/cue-draft-history-touch-ready":
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
		case "/cue-draft-history-test.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(testScript)
		default:
			handler.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	profileDir, err := os.MkdirTemp("", "stagecore-cue-history-chrome-")
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
	browserLog, err := os.Create(filepath.Join(t.TempDir(), "cue-draft-history-browser.log"))
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
				t.Fatalf("send native Meta+Z shortcut to Undo button: %v", err)
			}
			keyCommandID += 2
			close(resume)
		case resume := <-redoKeyRequests:
			if err := dispatchChromeShortcut(cdp, keyCommandID, "z", 12); err != nil {
				t.Fatalf("send native Meta+Shift+Z shortcut to Redo button: %v", err)
			}
			keyCommandID += 2
			close(resume)
		case request := <-touchRequests:
			if err := dispatchChromeTouch(cdp, request.x, request.y); err != nil {
				t.Fatalf("send emulated touchscreen event to Redo button: %v", err)
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
		Status            string   `json:"status"`
		UndoEnabled       bool     `json:"undo_enabled"`
		RedoAfterUndo     bool     `json:"redo_after_undo"`
		KeyboardUndo      bool     `json:"keyboard_undo"`
		KeyboardRedo      bool     `json:"keyboard_redo"`
		TouchRedo         bool     `json:"touch_redo"`
		RestoredSameCue   bool     `json:"restored_same_cue"`
		LinkedChildUndo   bool     `json:"linked_child_after_undo"`
		LinkedChildRedo   bool     `json:"linked_child_after_keyboard_redo"`
		Overflow          bool     `json:"overflow"`
		HistoryRequests   []string `json:"history_requests"`
		ExecutionRequests []string `json:"execution_requests"`
	}
	if err := json.Unmarshal([]byte(outcome), &report); err != nil {
		t.Fatalf("decode browser result %q: %v", outcome, err)
	}
	if report.Status != "PASS" || !report.UndoEnabled || !report.RedoAfterUndo ||
		!report.KeyboardUndo || !report.KeyboardRedo || !report.TouchRedo || !report.RestoredSameCue ||
		!report.LinkedChildUndo || !report.LinkedChildRedo || report.Overflow ||
		len(report.HistoryRequests) < 2 || len(report.ExecutionRequests) != 0 {
		t.Fatalf("browser Draft history acceptance failed: %+v", report)
	}
	cues, err := projects.ListCues(ctx, revision.ID)
	if err != nil || len(cues) != 3 || cues[0].ID != baseline.ID || cues[1].ID != linked.ID || cues[2].ID != created.ID ||
		len(cues[1].Actions) != 1 || cues[1].Actions[0].ID != linked.Actions[0].ID ||
		cues[1].Actions[0].CapabilityKey != "lighting.set" || len(cues[2].Actions) != 2 ||
		cues[2].Actions[0].ID != created.Actions[0].ID || cues[2].Actions[1].ID != created.Actions[1].ID ||
		cues[2].Actions[0].ExecutionMode != "PARALLEL" || cues[2].Actions[0].CapabilityKey != "osc.send" ||
		cues[2].Actions[1].ExecutionMode != "SEQUENTIAL" || cues[2].Actions[1].CapabilityKey != "midi.send" ||
		string(cues[2].Actions[1].TimeoutPolicy) != `{"start_delay_ms":250}` ||
		string(cues[2].ExecutionPolicy) != string(created.ExecutionPolicy) {
		t.Fatalf("Redo did not restore the original Cue and Action identity: cues=%+v err=%v", cues, err)
	}
	active, err := projects.ActiveSessionForProject(ctx, project.ID)
	if err != nil || active != nil {
		t.Fatalf("Draft history browser flow started a runtime Session: session=%+v err=%v", active, err)
	}
	snapshot, err := projects.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil || snapshot != nil {
		t.Fatalf("Draft history browser flow published a Snapshot: snapshot=%+v err=%v", snapshot, err)
	}
}
